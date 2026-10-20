package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"smartdisk/internal/config"
	"smartdisk/internal/model"
	"smartdisk/internal/smart"
)

// Collector gathers disk inventory and SMART data.
type Collector struct {
	cfg config.Config
	run Runner

	hibernate *hibernateStore
	powerLog  *powerLogStore

	// scanMu serializes expensive inventory scans. Readers only hold mu long
	// enough to copy the currently published snapshot, so a slow smartctl call
	// never blocks an already-cached overview.
	scanMu sync.Mutex
	mu     sync.RWMutex

	cached             []model.Disk
	cachedAt           time.Time
	hasCached          bool
	overview           overviewCache
	powerRevision      uint64
	powerStateRevision map[string]uint64
}

// overviewCache is the serialized /api/disks payload built once when the
// disk cache is published. The hot list path reuses it instead of
// deep-copying every disk's SMART tables on each request.
type overviewCache = []byte

// buildOverviewJSON converts a canonical disk slice into the serialized
// /api/disks payload. It matches httpapi.writeOverview's output shape,
// including the mock flag used by the UI in fixture mode.
func (c *Collector) buildOverviewJSON(disks []model.Disk) ([]byte, error) {
	sums := make([]model.Summary, 0, len(disks))
	for i := range disks {
		sums = append(sums, disks[i].ToSummary())
	}
	ov := model.Overview{
		Disks:     sums,
		Count:     len(sums),
		Generated: newestCollectedAt(disks),
		Mock:      c.cfg.MockDir != "",
	}
	return json.Marshal(ov)
}

// newestCollectedAt mirrors httpapi.collectedAt: report when the data was
// actually gathered instead of the page-open time.
func newestCollectedAt(disks []model.Disk) int64 {
	var newest int64
	for i := range disks {
		if disks[i].CollectedAt > newest {
			newest = disks[i].CollectedAt
		}
	}
	if newest == 0 {
		return time.Now().Unix()
	}
	return newest
}

// New builds a Collector. If cfg.MockDir is set it uses a MockRunner.
func New(cfg config.Config) *Collector {
	var r Runner
	if cfg.MockDir != "" {
		r = MockRunner{Dir: cfg.MockDir}
	} else {
		r = ExecRunner{}
	}
	return &Collector{
		cfg:                cfg,
		run:                r,
		hibernate:          loadHibernateStore(filepath.Join(cfg.DataDir, "hibernate.json")),
		powerLog:           loadPowerLogStore(filepath.Join(cfg.DataDir, "power-events.json")),
		powerStateRevision: make(map[string]uint64),
	}
}

// PowerEvents returns the power-control event log, newest first.
func (c *Collector) PowerEvents(limit int) []model.PowerEvent {
	return c.powerLog.list(limit)
}

// ClearPowerEvents wipes the persisted power-control log.
func (c *Collector) ClearPowerEvents() error {
	return c.powerLog.clear()
}

// lsblkNode is one row of `lsblk -J -O`.
type lsblkNode struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Type        string      `json:"type"`
	Tran        string      `json:"tran"`
	Size        json.Number `json:"size"`
	Model       string      `json:"model"`
	Serial      string      `json:"serial"`
	Rota        *bool       `json:"rota"`
	FsUsed      json.Number `json:"fsused"` // bytes used on this (partition) node
	FsUsePct    string      `json:"fsuse%"` // e.g. "42%"
	FsType      string      `json:"fstype"`
	Label       string      `json:"label"`
	UUID        string      `json:"uuid"`
	MountPoint  string      `json:"mountpoint"`
	MountPoints []string    `json:"mountpoints"`
	Children    []lsblkNode `json:"children"`
}

type lsblkOutput struct {
	BlockDevices []lsblkNode `json:"blockdevices"`
}

// inventory is the parsed lsblk result: physical disks plus every node (for
// building the storage topology including md/lvm virtual devices).
type inventory struct {
	disks []lsblkNode // physical disks (type=disk, not virtual)
	roots []lsblkNode // all top-level nodes (for topology tree)
}

// List returns the disk inventory. On a cold cache it reads base device data
// and SMART for awake devices, while deferring SMART reads for sleeping or
// indeterminate rotational disks. A full read requires Refresh or RefreshDisk.
func (c *Collector) List(ctx context.Context) ([]model.Disk, error) {
	if disks, ok := c.snapshot(); ok {
		return disks, nil
	}
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	if disks, ok := c.snapshot(); ok {
		return disks, nil
	}
	return c.collectAndPublish(ctx)
}

// Refresh forces a fresh collection, bypassing and replacing the cache.
// Sleeping rotational disks are probed (never read) and stay deferred, so a
// refresh reports the true power state without waking them.
func (c *Collector) Refresh(ctx context.Context) ([]model.Disk, error) {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	return c.collectAndPublish(ctx)
}

func cloneDisks(disks []model.Disk) []model.Disk {
	out := make([]model.Disk, len(disks))
	for i := range disks {
		out[i] = cloneDisk(disks[i])
	}
	return out
}

func cloneDisk(d model.Disk) model.Disk {
	out := d
	out.Attrs = append([]model.SmartAttr(nil), d.Attrs...)
	out.HealthNotes = append([]string(nil), d.HealthNotes...)
	out.SelfTests = append([]model.SelfTestEntry(nil), d.SelfTests...)
	out.ATAErrorLog = append([]model.ATAErrorEntry(nil), d.ATAErrorLog...)
	out.DevStats = append([]model.DevStatEntry(nil), d.DevStats...)
	out.Partitions = append([]model.Partition(nil), d.Partitions...)
	out.Roles = append([]string(nil), d.Roles...)
	out.VolumePaths = append([]string(nil), d.VolumePaths...)
	out.Chain = append([]model.ChainHop(nil), d.Chain...)
	out.SATAPhy = append([]model.SATAPhyEvent(nil), d.SATAPhy...)
	if d.NVMe != nil {
		v := *d.NVMe
		out.NVMe = &v
	}
	if d.Bridge != nil {
		v := *d.Bridge
		out.Bridge = &v
	}
	if d.Link != nil {
		v := *d.Link
		out.Link = &v
	}
	if d.TempHistory != nil {
		v := *d.TempHistory
		out.TempHistory = &v
	}
	if d.BtrfsErrs != nil {
		v := *d.BtrfsErrs
		out.BtrfsErrs = &v
	}
	return out
}

func (c *Collector) snapshot() ([]model.Disk, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.hasCached {
		return nil, false
	}
	return cloneDisks(c.cached), true
}

// snapshotOverview returns a fresh copy of the serialized overview.
func (c *Collector) snapshotOverview() ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.overview) == 0 {
		return nil, false
	}
	out := make([]byte, len(c.overview))
	copy(out, c.overview)
	return out, true
}

// OverviewJSON serves the serialized /api/disks payload from the cache when
// available. Falls back to a normal List + serialize on a cold cache.
func (c *Collector) OverviewJSON(ctx context.Context) ([]byte, bool) {
	if data, ok := c.snapshotOverview(); ok {
		return data, true
	}
	disks, err := c.List(ctx)
	if err != nil {
		return nil, false
	}
	data, err := c.buildOverviewJSON(disks)
	if err != nil {
		return nil, false
	}
	return data, true
}

// SetNote persists a user note for one disk. Empty note clears it.
func (c *Collector) SetNote(ctx context.Context, id, note string) (*model.Disk, error) {
	d, err := c.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, nil
	}
	if err := c.hibernate.setNote(d, note); err != nil {
		return nil, err
	}
	d.UserNote = note
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasCached {
		return d, nil
	}
	for i := range c.cached {
		if c.cached[i].ID == id || stableKey(&c.cached[i]) == stableKey(d) {
			c.cached[i].UserNote = note
			break
		}
	}
	if data, err := c.buildOverviewJSON(c.cached); err == nil {
		c.overview = overviewCache(data)
	}
	return d, nil
}

// updateCachedPowerState patches the cached disk's power-related fields after
// a power op, so /api/disks reflects the new state immediately without a
// full re-collection (which the anti-wake cache is designed to avoid).
func (c *Collector) updateCachedPowerState(id string, d *model.Disk) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasCached {
		return
	}
	for i := range c.cached {
		if c.cached[i].ID == id {
			c.cached = cloneDisks(c.cached)
			copyPowerState(&c.cached[i], d)
			if data, err := c.buildOverviewJSON(c.cached); err == nil {
				c.overview = overviewCache(data)
			}
			c.powerRevision++
			if c.powerStateRevision == nil {
				c.powerStateRevision = make(map[string]uint64)
			}
			c.powerStateRevision[id] = c.powerRevision
			return
		}
	}
}

func copyPowerState(dst, src *model.Disk) {
	dst.PowerState = src.PowerState
	dst.Hibernated = src.Hibernated
	dst.OfflineAt = src.OfflineAt
	dst.CanSleep = src.CanSleep
	dst.CanOffline = src.CanOffline
	dst.CanWake = src.CanWake
	dst.NoSleepReason = src.NoSleepReason
	dst.NoOfflineReason = src.NoOfflineReason
}

func (c *Collector) collectAndPublish(ctx context.Context) ([]model.Disk, error) {
	c.mu.RLock()
	startPowerRevision := c.powerRevision
	c.mu.RUnlock()

	disks, err := c.collect(ctx)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.powerRevision != startPowerRevision {
		current := make(map[string]model.Disk, len(c.cached))
		for i := range c.cached {
			current[c.cached[i].ID] = c.cached[i]
		}
		for i := range disks {
			if c.powerStateRevision[disks[i].ID] > startPowerRevision {
				if latest, ok := current[disks[i].ID]; ok {
					copyPowerState(&disks[i], &latest)
				}
			}
		}
	}
	c.cached = cloneDisks(disks)
	c.cachedAt = time.Now()
	c.hasCached = true
	for id, revision := range c.powerStateRevision {
		if revision <= startPowerRevision {
			delete(c.powerStateRevision, id)
		}
	}
	// Serialize the canonical overview before releasing the write lock so the
	// hot path can serve repeat requests without another full clone.
	if data, err := c.buildOverviewJSON(c.cached); err == nil {
		c.overview = overviewCache(data)
	}
	return cloneDisks(c.cached), nil
}

// collect builds a fresh inventory. Rotational disks are always power-probed
// first (smartctl -n standby does not spin up a sleeping drive); only awake
// disks get a full SMART read, keeping a refresh wake-free.
func (c *Collector) collect(ctx context.Context) ([]model.Disk, error) {
	inv, err := c.enumDisks(ctx)
	if err != nil {
		return nil, err
	}
	lsusb := c.loadLsusb(ctx)

	now := time.Now().Unix()
	disks := make([]model.Disk, len(inv.disks))
	workerCount := min(2, len(inv.disks))
	var workers sync.WaitGroup
	jobs := make(chan int)
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				if err := ctx.Err(); err != nil {
					return
				}
				d := c.baseDisk(inv.disks[i])
				if d.Rotational {
					if d.Interface == model.IfUSB {
						d.Bridge = c.resolveBridge(ctx, &d, lsusb)
					}
					state := c.probePowerState(ctx, &d)
					if state != "active" && state != "idle" {
						d.SmartDeferred = true
						c.fillLink(ctx, &d)
					} else {
						c.fillSmartWithPowerProbe(ctx, &d, lsusb, false)
					}
				} else {
					c.fillSmart(ctx, &d, lsusb)
				}
				d.CollectedAt = now
				disks[i] = d
			}
		}()
	}
	for i := range inv.disks {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Build storage topology and annotate each disk with its role + chain.
	c.annotateTopology(disks, inv.roots)

	// btrfs pool error counters (depends on chain/volume paths). btrfs calls
	// are external commands, so run them through the same bounded pool as
	// SMART reads instead of serializing the whole inventory.
	var btrfsWorkers sync.WaitGroup
	btrfsJobs := make(chan int)
	btrfsCount := 0
	for i := range disks {
		if !disks[i].SmartDeferred {
			btrfsCount++
		}
	}
	if btrfsCount > 0 {
		workerCount := min(2, btrfsCount)
		for range workerCount {
			btrfsWorkers.Add(1)
			go func() {
				defer btrfsWorkers.Done()
				for i := range btrfsJobs {
					if ctx.Err() != nil {
						return
					}
					c.fillBtrfsErrors(ctx, &disks[i])
				}
			}()
		}
		for i := range disks {
			if !disks[i].SmartDeferred {
				select {
				case btrfsJobs <- i:
				case <-ctx.Done():
					close(btrfsJobs)
					btrfsWorkers.Wait()
					return nil, ctx.Err()
				}
			}
		}
		close(btrfsJobs)
		btrfsWorkers.Wait()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	// Grade after all data (incl. btrfs) is gathered.
	for i := range disks {
		smart.Grade(&disks[i])
	}
	// Attach the persisted user note (survives reboots via serial key).
	for i := range disks {
		disks[i].UserNote = c.hibernate.noteFor(&disks[i])
	}

	// Probe kernel device state once per disk. The hibernate reconciliation
	// below consults it twice for offline records; one parallel pass avoids
	// serial cat spawns per disk and removes the duplicate probe.
	offlineStates := make([]bool, len(disks))
	var stateWorkers sync.WaitGroup
	stateJobs := make(chan int)
	stateWorkerCount := min(2, len(disks))
	for range stateWorkerCount {
		stateWorkers.Add(1)
		go func() {
			defer stateWorkers.Done()
			for i := range stateJobs {
				if ctx.Err() != nil {
					return
				}
				offlineStates[i] = c.sysfsOffline(ctx, disks[i].ID)
			}
		}()
	}
	for i := range disks {
		select {
		case stateJobs <- i:
		case <-ctx.Done():
			close(stateJobs)
			stateWorkers.Wait()
			return nil, ctx.Err()
		}
	}
	close(stateJobs)
	stateWorkers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := c.reconcilePendingOffline(ctx, disks, offlineStates); err != nil {
		return nil, err
	}

	// Persist fresh recovery data (UUIDs, mount points) for every live disk,
	// then merge in placeholders for disks that are deep-offlined (and thus
	// invisible to lsblk) so the UI can still show + wake them.
	for i := range disks {
		// Self-heal: if the KERNEL says this device is offline but we have no
		// hibernate record for it (e.g. it was offlined by an older build whose
		// state file was lost), mark it hibernated so the Wake button appears.
		if offlineStates[i] {
			disks[i].SmartDeferred = false
			rec := c.hibernate.getByDisk(&disks[i])
			if rec == nil || !rec.Offline {
				disks[i].PowerState = "offline"
				disks[i].Hibernated = true
				// Persist a minimal recovery record so wake works next time.
				if err := c.hibernate.upsertFromDisk(&disks[i]); err != nil {
					return nil, fmt.Errorf("persist recovery data for %s: %w", disks[i].ID, err)
				}
				if err := c.hibernate.markOffline(stableKey(&disks[i]), true); err != nil {
					return nil, fmt.Errorf("persist offline state for %s: %w", disks[i].ID, err)
				}
				if rec2 := c.hibernate.getByDisk(&disks[i]); rec2 != nil {
					disks[i].OfflineAt = rec2.OfflineAt
				}
				computePowerCaps(&disks[i])
				continue
			}
		}
		// A disk flagged offline in the store must STAY flagged even when it
		// still shows up in lsblk (an offlined device keeps its node with
		// state=offline); otherwise a refresh would flip it back to online
		// and the Wake button would vanish. BUT a system reboot brings every
		// drive back to running, clearing the kernel offline flag while our
		// record still says offline — so reconcile: only keep the offline
		// flag when the kernel actually still reports offline.
		if rec := c.hibernate.getByDisk(&disks[i]); rec != nil && rec.Offline {
			if !offlineStates[i] {
				// Reboot revived the drive: clear the stale offline flag so
				// the panel shows it online (with a 深度下线 button again).
				if err := c.hibernate.markOffline(stableKey(&disks[i]), false); err != nil {
					return nil, fmt.Errorf("clear stale offline state for %s: %w", disks[i].ID, err)
				}
				if err := c.hibernate.upsertFromDisk(&disks[i]); err != nil {
					return nil, fmt.Errorf("refresh recovery data for %s: %w", disks[i].ID, err)
				}
				computePowerCaps(&disks[i])
				continue
			}
			disks[i].PowerState = "offline"
			disks[i].Hibernated = true
			disks[i].SmartDeferred = false
			disks[i].OfflineAt = rec.OfflineAt
			computePowerCaps(&disks[i])
			continue // don't refresh recovery data for an offlined disk
		}
		persistErr := c.hibernate.upsertFromDisk(&disks[i])
		if persistErr != nil {
			log.Printf("[hibernate] persist recovery data for %s: %v", disks[i].ID, persistErr)
		}
		computePowerCaps(&disks[i])
		if persistErr != nil {
			disks[i].NoSleepReason = "恢复数据无法持久化，已禁用电源操作"
			disks[i].NoOfflineReason = "恢复数据无法持久化，已禁用深度下线"
			disks[i].CanSleep = false
			disks[i].CanOffline = false
		}
		if rec := c.hibernate.getByDisk(&disks[i]); rec != nil && rec.PendingOffline {
			disks[i].NoSleepReason = "上次下线未完成，请先恢复存储卷"
			disks[i].NoOfflineReason = "上次下线未完成，请先恢复存储卷"
			disks[i].CanSleep = false
			disks[i].CanOffline = false
		}
	}
	disks = c.mergeOfflinePlaceholders(disks)
	return disks, nil
}

// sysfsOffline reports whether the kernel currently marks the device offline
// (/sys/block/<id>/device/state == "offline"). Read-only probe.
func (c *Collector) sysfsOffline(ctx context.Context, id string) bool {
	out, _, err := c.run.Run(ctx, "cat", "/sys/block/"+id+"/device/state")
	if err != nil {
		return false
	}
	return strings.TrimSpace(strings.ToLower(out)) == "offline"
}

// mergeOfflinePlaceholders appends a stub disk for every hibernate record
// still flagged offline whose disk is absent from the live inventory.
func (c *Collector) mergeOfflinePlaceholders(disks []model.Disk) []model.Disk {
	live := map[string]bool{}
	for _, d := range disks {
		live[d.ID] = true
	}
	for _, rec := range c.hibernate.offlineRecords() {
		if live[rec.ID] {
			continue // came back online (e.g. after reboot); flag cleared on wake
		}
		d := recordToDisk(rec)
		computePowerCaps(&d)
		disks = append(disks, d)
	}
	return disks
}

// Topology returns the full storage tree (physical disks + md/lvm/fs layers)
// with each physical disk's current health attached.
func (c *Collector) Topology(ctx context.Context) ([]*model.StorageNode, error) {
	inv, err := c.enumDisks(ctx)
	if err != nil {
		return nil, err
	}
	disks, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	return c.buildTopologyTree(disks, inv.roots), nil
}

// Get returns one disk by id (kernel name like "sda" or "nvme0n1").
func (c *Collector) Get(ctx context.Context, id string) (*model.Disk, error) {
	disks, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range disks {
		if disks[i].ID == id {
			return &disks[i], nil
		}
	}
	return nil, nil
}

// enumDisks returns top-level whole-disk block devices (skips partitions,
// loop, rom, and device-mapper virtual devices).
func (c *Collector) enumDisks(ctx context.Context) (inventory, error) {
	// No -d: keep the partition tree so we can sum filesystem usage per disk.
	out, _, err := c.run.Run(ctx, c.cfg.LsblkPath, "-J", "-O", "-b")
	if err != nil {
		return inventory{}, err
	}
	var lo lsblkOutput
	if err := json.Unmarshal([]byte(out), &lo); err != nil {
		return inventory{}, err
	}
	var disks []lsblkNode
	for _, n := range lo.BlockDevices {
		if n.Type != "disk" {
			continue
		}
		name := n.Name
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") ||
			strings.HasPrefix(name, "dm-") ||
			strings.HasPrefix(name, "sr") || strings.HasPrefix(name, "zram") {
			continue
		}
		disks = append(disks, n)
	}
	return inventory{disks: disks, roots: lo.BlockDevices}, nil
}

// sumFsUsage recursively sums fsused across a node and its children.
func sumFsUsage(n lsblkNode) int64 {
	var total int64
	if v, err := n.FsUsed.Int64(); err == nil {
		total += v
	}
	for _, ch := range n.Children {
		total += sumFsUsage(ch)
	}
	return total
}

// extractPartitions flattens the partition tree into Partition records (D).
func extractPartitions(n lsblkNode) []model.Partition {
	var out []model.Partition
	var walk func(x lsblkNode)
	walk = func(x lsblkNode) {
		if x.Type == "part" {
			p := model.Partition{
				Name:       x.Name,
				FsType:     x.FsType,
				Label:      x.Label,
				UUID:       x.UUID,
				MountPoint: x.MountPoint,
				UsePercent: -1,
			}
			if v, err := x.Size.Int64(); err == nil {
				p.Size = v
			}
			if v, err := x.FsUsed.Int64(); err == nil {
				p.Used = v
			}
			// fsuse% like "42%"
			if s := strings.TrimSuffix(x.FsUsePct, "%"); s != "" {
				if pct, err := strconv.Atoi(s); err == nil {
					p.UsePercent = pct
				}
			}
			out = append(out, p)
		}
		for _, ch := range x.Children {
			walk(ch)
		}
	}
	walk(n)
	return out
}

// baseDisk builds a Disk with identity fields from lsblk before SMART.
func (c *Collector) baseDisk(n lsblkNode) model.Disk {
	id := n.Name
	path := n.Path
	if path == "" {
		path = "/dev/" + n.Name
	}
	d := model.Disk{
		ID:                id,
		Name:              path,
		Model:             strings.TrimSpace(n.Model),
		Serial:            strings.TrimSpace(n.Serial),
		Interface:         tranToInterface(n.Tran),
		PercentUsed:       -1,
		PercentLifeRemain: -1,
		ManufactureDate:   "未知",
		Health:            model.GradeUnknown,
	}
	if sz, err := n.Size.Int64(); err == nil {
		d.Capacity = sz
	}
	// Filesystem usage summed across mounted partitions of this disk.
	d.UsedCapacity = sumFsUsage(n)
	if d.Capacity > 0 && d.UsedCapacity > 0 {
		d.UsagePercent = int(d.UsedCapacity * 100 / d.Capacity)
	} else {
		d.UsagePercent = -1
	}
	// D: partition detail.
	d.Partitions = extractPartitions(n)
	if n.Rota != nil {
		d.Rotational = *n.Rota
	}
	// Type from transport + rotational hint; SMART refines later.
	switch n.Tran {
	case "nvme":
		d.Type = model.TypeNVMe
	case "sata", "ata", "":
		if d.Rotational {
			d.Type = model.TypeHDD
		} else {
			d.Type = model.TypeSSD
		}
	case "usb":
		if d.Rotational {
			d.Type = model.TypeHDD
		} else {
			d.Type = model.TypeSSD
		}
	default:
		d.Type = model.TypeUnknown
	}
	return d
}

func tranToInterface(tran string) model.InterfaceKind {
	switch strings.ToLower(tran) {
	case "sata", "ata":
		return model.IfSATA
	case "nvme":
		return model.IfNVMe
	case "usb":
		return model.IfUSB
	case "scsi", "sas":
		return model.IfSCSI
	default:
		return model.IfUnk
	}
}

// fillSmart gathers SMART for one disk, choosing the right access path.
func (c *Collector) fillSmart(ctx context.Context, d *model.Disk, lsusb map[string]string) {
	c.fillSmartWithPowerProbe(ctx, d, lsusb, true)
}

func (c *Collector) fillSmartWithPowerProbe(ctx context.Context, d *model.Disk, lsusb map[string]string, refreshPower bool) {
	dev := d.Name

	// E: link info by transport. SATA link is parsed from smartctl output;
	// NVMe and USB are filled here from sysfs/bridge knowledge.
	defer c.fillLink(ctx, d)
	if refreshPower {
		defer c.fillPowerState(ctx, d)
	}

	// NVMe: try nvme-cli first for the health log, fall back to smartctl -d nvme.
	if d.Interface == model.IfNVMe {
		if c.trySmartctl(ctx, d, dev, "nvme") {
			return
		}
		// Leave SmartAvail=false if neither worked.
		return
	}

	// USB bridge: resolve chip + passthrough probing.
	if d.Interface == model.IfUSB {
		if d.Bridge == nil {
			d.Bridge = c.resolveBridge(ctx, d, lsusb)
		}
		for _, pt := range smart.PassthroughOrder(d.Bridge) {
			if c.trySmartctl(ctx, d, dev, pt) {
				if d.Bridge == nil {
					d.Bridge = &model.BridgeInfo{}
				}
				d.Bridge.Passthrough = pt
				return
			}
		}
		// All passthroughs failed.
		d.SmartAvail = false
		return
	}

	// SATA / SCSI direct.
	if !c.trySmartctl(ctx, d, dev, "sat") {
		c.trySmartctl(ctx, d, dev, "auto")
	}
}

// fillPowerState queries `smartctl -n standby <dev>` for the current power
// state (active / idle / standby). Read-only probe; never changes state.
func (c *Collector) fillPowerState(ctx context.Context, d *model.Disk) {
	c.probePowerState(ctx, d)
}

// probePowerState checks power before any full SMART command is issued. For a
// USB bridge it tries the supported passthrough order and remembers the first
// protocol that can report a state.
func (c *Collector) probePowerState(ctx context.Context, d *model.Disk) string {
	var types []string
	switch d.Interface {
	case model.IfNVMe:
		types = []string{"nvme"}
	case model.IfUSB:
		if d.Bridge != nil && d.Bridge.Passthrough != "" {
			types = []string{d.Bridge.Passthrough}
		} else {
			types = smart.PassthroughOrder(d.Bridge)
		}
	default:
		types = []string{"sat", "auto"}
	}

	for _, dtype := range types {
		args := []string{c.cfg.SmartctlPath, "-n", "standby"}
		if dtype != "" && dtype != "auto" {
			args = append(args, "-d", dtype)
		}
		args = append(args, d.Name)
		out, _, _ := c.run.Run(ctx, args...)
		state := parsePowerState(out)
		if state == "" {
			continue
		}
		d.PowerState = state
		if d.Interface == model.IfUSB && d.Bridge != nil {
			d.Bridge.Passthrough = dtype
		}
		return state
	}
	d.PowerState = ""
	return ""
}

// parsePowerState extracts the power state from `smartctl -n standby` output.
func parsePowerState(out string) string {
	s := strings.ToLower(out)
	switch {
	case strings.Contains(s, "standby"):
		return "standby"
	case strings.Contains(s, "idle"):
		return "idle"
	case strings.Contains(s, "active") || strings.Contains(s, "active_or_idle"):
		return "active"
	default:
		return ""
	}
}

// fillLink populates E (link/topology) for NVMe and USB disks. SATA is
// already handled in ParseSmartctl via interface_speed.
func (c *Collector) fillLink(ctx context.Context, d *model.Disk) {
	if d.Link != nil {
		return // SATA link already set from smartctl
	}
	switch d.Interface {
	case model.IfNVMe:
		d.Link = c.nvmeLink(ctx, d.ID)
	case model.IfUSB:
		d.Link = c.usbLink(d)
	}
}

// nvmeLink reads the negotiated PCIe link speed/width from sysfs.
// /sys/class/nvme/<ctrl>/device exposes current_link_speed / current_link_width
// and max_link_speed / max_link_width on the PCI device.
func (c *Collector) nvmeLink(ctx context.Context, id string) *model.LinkInfo {
	// id is like nvme0n1; the controller is nvme0.
	ctrl := id
	// strip namespace suffix: nvme0n1 -> nvme0 (find "n" after the digits).
	if strings.HasPrefix(id, "nvme") {
		i := 4
		for i < len(id) && id[i] >= '0' && id[i] <= '9' {
			i++
		}
		if i < len(id) && id[i] == 'n' {
			ctrl = id[:i]
		}
	}
	read := func(f string) string {
		out, _, err := c.run.Run(ctx, "cat", "/sys/class/nvme/"+ctrl+"/device/"+f)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(out)
	}
	curSpeed, curWidth := read("current_link_speed"), read("current_link_width")
	maxSpeed, maxWidth := read("max_link_speed"), read("max_link_width")
	if curSpeed == "" && maxSpeed == "" {
		return &model.LinkInfo{Kind: "NVMe", Negotiated: "PCIe", DeviceMax: "PCIe"}
	}
	neg := strings.TrimSpace(strings.Join([]string{curSpeed, "x" + curWidth}, " "))
	max := strings.TrimSpace(strings.Join([]string{maxSpeed, "x" + maxWidth}, " "))
	neg = strings.TrimSpace(strings.Trim(neg, " x"))
	max = strings.TrimSpace(strings.Trim(max, " x"))
	// Downgraded when negotiated speed OR width is below device max.
	down := false
	if lessNum(curSpeed, maxSpeed) || lessNum(curWidth, maxWidth) {
		down = true
	}
	li := &model.LinkInfo{Kind: "NVMe", Negotiated: "PCIe " + neg, DeviceMax: "PCIe " + max, Downgraded: down}
	if down {
		li.Note = "协商链路低于设备最大规格，可能为插槽/主板限制"
	}
	return li
}

// lessNum compares two numeric-ish strings ("16.0GT/s", "4", "6.0 Gb/s").
// Returns true only when both parse and a < b. Unparseable => false.
func lessNum(a, b string) bool {
	fa, ok1 := leadingFloat(a)
	fb, ok2 := leadingFloat(b)
	if !ok1 || !ok2 {
		return false
	}
	return fa < fb
}

// leadingFloat extracts the first numeric token from a string.
func leadingFloat(s string) (float64, bool) {
	for _, tok := range strings.Fields(strings.TrimSpace(s)) {
		if v, err := strconv.ParseFloat(tok, 64); err == nil {
			return v, true
		}
	}
	// also handle tokens like "16.0GT/s" (no space)
	s = strings.TrimSpace(s)
	num := ""
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			num += string(r)
		} else if num != "" {
			break
		}
	}
	if num == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(num, 64)
	return v, err == nil
}

// usbLink builds a USB link descriptor from the bridge chip knowledge.
func (c *Collector) usbLink(d *model.Disk) *model.LinkInfo {
	li := &model.LinkInfo{Kind: "USB", Negotiated: "USB", DeviceMax: "USB"}
	if d.Bridge != nil && d.Bridge.Chip != "" {
		li.Negotiated = "USB · " + d.Bridge.Chip
		li.DeviceMax = "USB · " + d.Bridge.Chip
	}
	li.Note = "USB 桥接盘的实际速率取决于桥接芯片与主机端口版本"
	return li
}

// trySmartctl runs smartctl -a -j -d <dtype> dev and merges into d. Returns
// true when SMART data was successfully parsed.
func (c *Collector) trySmartctl(ctx context.Context, d *model.Disk, dev, dtype string) bool {
	args := []string{c.cfg.SmartctlPath, "-a", "-j"}
	if dtype != "" && dtype != "auto" {
		args = append(args, "-d", dtype)
	}
	args = append(args, dev)
	out, _, err := c.run.Run(ctx, args...)
	if err != nil && strings.TrimSpace(out) == "" {
		return false
	}
	if !strings.Contains(out, "{") {
		return false
	}
	// smartctl returns non-zero for various benign conditions (e.g. SMART
	// disabled, some attributes failing) but still prints JSON. We accept the
	// payload whenever it parses and carries device info.
	probe := model.Disk{
		Interface:  d.Interface,
		Type:       d.Type,
		Rotational: d.Rotational,
	}
	if err := smart.ParseSmartctl([]byte(out), &probe); err != nil {
		return false
	}
	if probe.Model == "" && probe.Serial == "" && !probe.SmartAvail {
		return false
	}
	mergeDisk(d, &probe)
	return true
}

// mergeDisk copies SMART-derived fields from src into dst, preserving
// identity fields that src left empty.
func mergeDisk(dst, src *model.Disk) {
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.Serial != "" {
		dst.Serial = src.Serial
	}
	if src.Firmware != "" {
		dst.Firmware = src.Firmware
	}
	if src.Capacity > 0 {
		dst.Capacity = src.Capacity
	}
	dst.SmartSupported = src.SmartSupported
	dst.SmartEnabled = src.SmartEnabled
	dst.SmartOK = src.SmartOK
	dst.SmartAvail = src.SmartAvail
	if src.Temperature > 0 {
		dst.Temperature = src.Temperature
	}
	if src.PowerOnHours > 0 {
		dst.PowerOnHours = src.PowerOnHours
	}
	if src.PowerCycles > 0 {
		dst.PowerCycles = src.PowerCycles
	}
	dst.UnsafeShutdowns = src.UnsafeShutdowns
	dst.PercentUsed = src.PercentUsed
	dst.PercentLifeRemain = src.PercentLifeRemain
	dst.TotalBytesWritten = src.TotalBytesWritten
	if src.WriteSource != "" {
		dst.WriteSource = src.WriteSource
	}
	if src.LifeSource != "" {
		dst.LifeSource = src.LifeSource
	}
	if src.Vendor != "" {
		dst.Vendor = src.Vendor
	}
	if src.Controller != "" {
		dst.Controller = src.Controller
	}
	dst.UsageYears = src.UsageYears
	if len(src.Attrs) > 0 {
		dst.Attrs = src.Attrs
	}
	// Media type from SMART is authoritative when present.
	if src.Type != "" && src.Type != model.TypeUnknown {
		dst.Type = src.Type
	}
	if src.Type == model.TypeNVMe || src.RotationRate > 0 || !dst.Rotational {
		dst.Rotational = src.Rotational
	}
	if src.NVMe != nil {
		dst.NVMe = src.NVMe
	}
	// A/E extras parsed from smartctl.
	if src.LogicalSectorSize > 0 {
		dst.LogicalSectorSize = src.LogicalSectorSize
	}
	if src.PhysicalSectorSize > 0 {
		dst.PhysicalSectorSize = src.PhysicalSectorSize
	}
	if src.RotationRate > 0 {
		dst.RotationRate = src.RotationRate
	}
	if src.FormFactor != "" {
		dst.FormFactor = src.FormFactor
	}
	dst.TrimSupported = src.TrimSupported
	if len(src.SelfTests) > 0 {
		dst.SelfTests = src.SelfTests
	}
	if len(src.ATAErrorLog) > 0 {
		dst.ATAErrorLog = src.ATAErrorLog
	}
	if len(src.DevStats) > 0 {
		dst.DevStats = src.DevStats
	}
	if src.Link != nil {
		dst.Link = src.Link
	}
	if src.WWN != "" {
		dst.WWN = src.WWN
	}
	if src.CacheSizeBytes > 0 {
		dst.CacheSizeBytes = src.CacheSizeBytes
	}
	if src.NCQDepth > 0 {
		dst.NCQDepth = src.NCQDepth
	}
	if src.TempHistory != nil {
		dst.TempHistory = src.TempHistory
	}
	if src.LoadCycleCount > 0 {
		dst.LoadCycleCount = src.LoadCycleCount
	}
	if len(src.SATAPhy) > 0 {
		dst.SATAPhy = src.SATAPhy
	}
	if src.APMLevel > 0 {
		dst.APMLevel = src.APMLevel
	}
}

// resolveBridge maps udev USB ids for a disk to a known bridge chip.
func (c *Collector) resolveBridge(ctx context.Context, d *model.Disk, lsusb map[string]string) *model.BridgeInfo {
	vid, pid := c.usbIDs(ctx, d.ID)
	if vid == "" {
		return &model.BridgeInfo{Chip: "USB 桥接"}
	}
	return smart.BridgeFor(vid, pid, lsusb)
}

// usbIDs reads the USB vendor/product ids for a block device from sysfs.
// Path: /sys/block/<name>/device is a symlink up the chain; the nearest USB
// device ancestor carries idVendor/idProduct.
func (c *Collector) usbIDs(ctx context.Context, name string) (string, string) {
	base := "/sys/block/" + name + "/device"
	// Walk up to 6 ancestors looking for idVendor/idProduct.
	out, _, err := c.run.Run(ctx, "sh", "-c",
		"p=$(readlink -f "+base+"); for i in 1 2 3 4 5 6; do "+
			"if [ -f \"$p/idVendor\" ]; then cat \"$p/idVendor\"; echo; cat \"$p/idProduct\"; exit 0; fi; "+
			"p=$(dirname \"$p\"); done; exit 1")
	if err != nil {
		return "", ""
	}
	parts := strings.Fields(out)
	if len(parts) >= 2 {
		return strings.ToLower(parts[0]), strings.ToLower(parts[1])
	}
	return "", ""
}

// loadLsusb returns the vendor:product -> description map, empty on failure.
func (c *Collector) loadLsusb(ctx context.Context) map[string]string {
	out, _, err := c.run.Run(ctx, c.cfg.LsusbPath)
	if err != nil {
		return map[string]string{}
	}
	return smart.ParseLsusb(out)
}
