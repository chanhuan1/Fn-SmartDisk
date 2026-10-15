package collector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"smartdisk/internal/model"
	"smartdisk/internal/smart"
)

// Power-control errors surfaced to the API layer (and shown to the user).
var (
	ErrSystemDisk      = errors.New("系统盘不允许电源操作")
	ErrNotHDD          = errors.New("仅 SATA 机械硬盘需要休眠（固态硬盘不需要，USB 桥接不支持）")
	ErrNeedRaidConfirm = errors.New("该盘是 md RAID 阵列成员，深度下线会导致阵列降级，需确认风险后重试")
	ErrDiskMissing     = errors.New("磁盘不存在或已下线")
	ErrNotOffline      = errors.New("磁盘当前不是下线状态，无需唤醒")
	ErrDiskBusy        = errors.New("检测到该盘正在读写，强制下线可能丢失正在写入的数据，需确认风险后重试")
)

// PowerResult is returned by Sleep/Offline/Wake.
type PowerResult struct {
	OK      bool        `json:"ok"`
	Message string      `json:"message"`
	Disk    *model.Disk `json:"disk,omitempty"`
}

// StepFunc is called as a power op progresses through its steps. detail is a
// human-readable line like "同步数据 (sync)" or "卸载卷 /vol2".
type StepFunc func(step string, detail string)

// stepEmitter holds an optional progress callback for one power operation.
// nil-safe: emit() is a no-op when unset, so existing callers are unaffected.
type stepEmitter struct{ fn StepFunc }

func (s stepEmitter) emit(step, detail string) {
	if s.fn != nil {
		s.fn(step, detail)
	}
}

// offlineCtx bundles the inputs for one offline operation.
type offlineCtx struct {
	confirmRaid   bool
	confirmBusyIO bool // user acknowledged the disk is mid-IO and still wants offline
	steps         stepEmitter
}

var powerIDRe = regexp.MustCompile(`^[a-z0-9]+$`)

// wakeTimeout bounds how long Wake waits for the device node to reappear
// (HDD spin-up can take ~10s). A package var so tests can shrink it.
var wakeTimeout = 15 * time.Second

// statDevice exists so tests can stub device-node detection if needed.
var statDevice = os.Stat

// wakeMu serializes power ops (Sleep/Offline/Wake) so two concurrent
// requests cannot drive the same state machine into an inconsistent state.
var wakeMu sync.Mutex

// computePowerCaps fills the Can*/NoSleep/NoOffline capability flags used by
// the UI to render (and disable) the power buttons. Safe to call on both
// live disks and offline placeholders.
func computePowerCaps(d *model.Disk) {
	d.CanSleep, d.CanOffline, d.CanWake = false, false, false
	d.NoSleepReason, d.NoOfflineReason = "", ""
	d.NeedsRaidConfirm = false
	if d.Hibernated || d.PowerState == "offline" {
		d.CanWake = true
		return
	}
	if isSystemDisk(d) {
		d.NoSleepReason = "系统盘"
		d.NoOfflineReason = "系统盘"
		return
	}
	if !d.Rotational || d.Interface != model.IfSATA {
		reason := "仅 SATA 机械硬盘支持"
		switch {
		case d.Interface == model.IfNVMe:
			reason = "固态硬盘不需要休眠（NVMe）"
		case d.Interface == model.IfUSB:
			reason = "USB 桥接盘不支持可靠休眠"
		case !d.Rotational:
			reason = "固态硬盘不需要休眠"
		}
		d.NoSleepReason = reason
		d.NoOfflineReason = reason
		return
	}
	// SATA HDD.
	d.CanSleep = true
	d.CanOffline = true
	if isRaidMember(d) {
		d.NeedsRaidConfirm = true
	}
}

// Sleep puts a SATA HDD into standby immediately (hdparm -y). Read-only with
// respect to mounts and arrays; fnOS may still wake the disk on access.
func (c *Collector) Sleep(ctx context.Context, id string) (*PowerResult, error) {
	wakeMu.Lock()
	defer wakeMu.Unlock()
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	d, err := c.liveDiskLocked(ctx, id)
	if err != nil {
		return nil, err
	}
	computePowerCaps(d)
	if !d.CanSleep {
		return nil, fmt.Errorf("%w：%s", ErrNotHDD, d.NoSleepReason)
	}
	if out, _, err := c.run.Run(ctx, c.cfg.HdparmPath, "-y", d.Name); err != nil {
		e := fmt.Errorf("hdparm 休眠失败: %v (%s)", err, strings.TrimSpace(out))
		c.logPower(d, "sleep", false, e.Error())
		return nil, e
	}
	// Re-probe power state only (cheap, drive now reports standby).
	c.fillPowerState(ctx, d)
	// Push the fresh state into the overview cache so the UI shows 休眠
	// immediately (liveDisk works on a separate slice, not c.cached).
	c.updateCachedPowerState(d.ID, d)
	c.logPower(d, "sleep", true, "进入休眠 standby")
	return &PowerResult{OK: true, Message: fmt.Sprintf("%s 已进入休眠（standby）", d.ID), Disk: d}, nil
}

// logPower appends a power-control event. Safe to call with a nil disk.
func (c *Collector) logPower(d *model.Disk, action string, ok bool, detail string) {
	if c.powerLog == nil {
		return
	}
	e := model.PowerEvent{Action: action, OK: ok, Detail: detail}
	if d != nil {
		e.DiskID = d.ID
		e.Model = d.Model
	}
	c.powerLog.add(e)
}

// Offline deep-hibernates a disk: sync, umount its volumes, spin down, then
// mark the device offline in the kernel so fnOS background services can no
// longer reach it. confirmRaid must be true for md RAID member disks;
// confirmBusyIO is a separate acknowledgement for a disk with active IO.
func (c *Collector) Offline(ctx context.Context, id string, confirmRaid, confirmBusyIO bool) (*PowerResult, error) {
	return c.offline(ctx, id, offlineCtx{confirmRaid: confirmRaid, confirmBusyIO: confirmBusyIO})
}

// OfflineWithSteps is Offline with a live progress callback (used by the SSE
// endpoint to stream each step to the UI as it happens).
func (c *Collector) OfflineWithSteps(ctx context.Context, id string, confirmRaid, confirmBusyIO bool, steps StepFunc) (*PowerResult, error) {
	return c.offline(ctx, id, offlineCtx{confirmRaid: confirmRaid, confirmBusyIO: confirmBusyIO, steps: stepEmitter{fn: steps}})
}

func (c *Collector) offline(ctx context.Context, id string, octx offlineCtx) (*PowerResult, error) {
	wakeMu.Lock()
	defer wakeMu.Unlock()
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	d, err := c.liveDiskLocked(ctx, id)
	if err != nil {
		return nil, err
	}
	steps := octx.steps
	computePowerCaps(d)
	if !d.CanOffline {
		return nil, fmt.Errorf("%w：%s", ErrNotHDD, d.NoOfflineReason)
	}
	if d.NeedsRaidConfirm && !octx.confirmRaid {
		return nil, ErrNeedRaidConfirm
	}

	// Refuse to yank a disk that is actively doing IO: a lazy umount of a
	// busy filesystem can lose in-flight writes. Require explicit ack.
	if !octx.confirmBusyIO {
		if busy, detail := c.diskIOActive(ctx, d.ID); busy {
			c.logPower(d, "offline", false, "拒绝下线："+detail)
			return nil, fmt.Errorf("%w（%s）", ErrDiskBusy, detail)
		}
	}

	// Make sure the recovery record is fresh before the disk disappears.
	c.hibernate.upsertFromDisk(d)

	// 1. flush dirty pages so umount is fast and clean.
	steps.emit("sync", "同步数据 (sync)")
	_, _, _ = c.run.Run(ctx, "sync")

	// 2. lazily umount every mount point reachable from this disk. We use
	// `umount -l`: fnOS keeps a read-only monitor handle (trim_file_monit)
	// on every volume, so a strict umount always fails with EBUSY. Lazy
	// detach removes the mount from the namespace immediately while the
	// kernel finishes cleanup once the monitor releases it; combined with
	// the sync above and the device going offline right after, this is the
	// approach the community cold-backup guides rely on.
	mounts := mountPointsOf(d)
	for _, mp := range mounts {
		if err := validateMountPoint(mp); err != nil {
			c.logPower(d, "offline", false, "拒绝非法挂载点: "+mp)
			return nil, err
		}
		steps.emit("umount", "懒卸载卷 "+mp)
		if out, _, err := c.run.Run(ctx, "umount", "-l", mp); err != nil {
			e := fmt.Errorf("懒卸载 %s 失败: %v (%s)", mp, err, strings.TrimSpace(out))
			c.logPower(d, "offline", false, e.Error())
			return nil, e
		}
	}

	// 3. spin the drive down before cutting it off.
	steps.emit("spindown", "停转硬盘 (hdparm -y)")
	_, _, _ = c.run.Run(ctx, c.cfg.HdparmPath, "-y", d.Name)

	// 4. kernel-level isolation.
	steps.emit("offline", "内核标记 device offline")
	if err := c.writeSysfsState(ctx, d.ID, "offline"); err != nil {
		// Best-effort rollback: remount what we unmounted.
		for _, mp := range mounts {
			_, _, _ = c.run.Run(ctx, "mount", mp)
		}
		c.logPower(d, "offline", false, "写入内核 offline 失败: "+err.Error())
		return nil, fmt.Errorf("写入内核 offline 状态失败: %w", err)
	}
	c.hibernate.markOffline(stableKey(d), true)

	d.PowerState = "offline"
	d.Hibernated = true
	d.OfflineAt = time.Now().Unix()
	computePowerCaps(d)
	c.updateCachedPowerState(d.ID, d)
	c.logPower(d, "offline", true, fmt.Sprintf("深度下线（懒卸载 %d 个卷 + 内核隔离）", len(mounts)))
	return &PowerResult{OK: true, Message: fmt.Sprintf("%s 已深度下线（内核隔离），飞牛后台无法再唤醒它", d.ID), Disk: d}, nil
}

// Wake brings a deep-offlined disk back: state=running, wait for the device
// node, rescan the bus to re-read the partition table (giving the kernel a
// chance to reassemble any md array first), then restore any mounts that
// have not come back on their own.
func (c *Collector) Wake(ctx context.Context, id string) (*PowerResult, error) {
	return c.wake(ctx, id, stepEmitter{})
}

// WakeWithSteps is Wake with a live progress callback (SSE streaming).
func (c *Collector) WakeWithSteps(ctx context.Context, id string, steps StepFunc) (*PowerResult, error) {
	return c.wake(ctx, id, stepEmitter{fn: steps})
}

func (c *Collector) wake(ctx context.Context, id string, steps stepEmitter) (*PowerResult, error) {
	wakeMu.Lock()
	defer wakeMu.Unlock()
	rec := c.hibernate.get(id)
	if rec == nil {
		// Resolve by stable identity too: the request id is a kernel name
		// which may have changed across reboots.
		rec = c.findOfflineByID(ctx, id)
	}
	if rec == nil || !rec.Offline {
		// Maybe it is actually online already (e.g. after a reboot).
		if d, err := c.liveDisk(ctx, id); err == nil {
			return &PowerResult{OK: true, Message: d.ID + " 已在线", Disk: d}, nil
		}
		return nil, ErrNotOffline
	}
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	// Use the device name captured when the disk was offlined. A system
	// reboot clears the kernel offline flag (drive comes back on its own),
	// so wake is only meaningful when the OS did NOT reboot — the kernel
	// name is therefore still valid.
	devID := rec.ID
	wakeDisk := &model.Disk{ID: devID, Model: rec.Model}
	steps.emit("running", "写入内核 running")
	if err := c.writeSysfsState(ctx, devID, "running"); err != nil {
		c.logPower(wakeDisk, "wake", false, "写入 running 失败: "+err.Error())
		return nil, fmt.Errorf("写入内核 running 状态失败: %w", err)
	}

	// Wait for the device node (spin-up can take ~10s on HDDs).
	steps.emit("wait", "等待硬盘起旋（最长 15 秒）")
	devPath := "/dev/" + devID
	deadline := time.Now().Add(wakeTimeout)
	for time.Now().Before(deadline) {
		if _, err := statDevice(devPath); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if _, err := statDevice(devPath); err != nil {
		c.logPower(wakeDisk, "wake", false, "等待设备节点超时（起旋失败）")
		return nil, fmt.Errorf("等待 %s 出现超时（盘体起旋失败？）", devPath)
	}
	steps.emit("rescan", "重扫总线，读取分区表")
	// Device is back: force a bus rescan so the partition table is re-read.
	// Doing this after the device is present lets the kernel reassemble an
	// md array (member visible) before we look for mounts to restore.
	_ = c.writeSysfsRescan(ctx, devID)

	// Restore mounts that did not come back automatically.
	var remountErrs []string
	var remounted []string
	for _, m := range rec.Mounts {
		if m.Noauto || m.UUID == "" || m.MountPoint == "" {
			continue
		}
		if err := validateMountPoint(m.MountPoint); err != nil {
			remountErrs = append(remountErrs, m.MountPoint+": 非法挂载点，已跳过")
			continue
		}
		if isMounted(ctx, c.run, m.MountPoint) {
			continue
		}
		if out, _, err := c.run.Run(ctx, "mount", "UUID="+m.UUID, m.MountPoint); err != nil {
			remountErrs = append(remountErrs, fmt.Sprintf("%s: %v (%s)", m.MountPoint, err, strings.TrimSpace(out)))
		} else {
			remounted = append(remounted, m.MountPoint)
		}
	}
	if len(remounted) > 0 {
		steps.emit("remount", "补挂载 "+strings.Join(remounted, ", "))
	}
	// Keep the record flagged offline when some mounts could not be restored,
	// so the UI still offers Wake (and the operator can see it is degraded)
	// instead of the disk silently dropping off the panel.
	if len(remountErrs) == 0 {
		c.hibernate.markOffline(stableKeyRec(rec), false)
	}

	// Refresh ONLY the woken disk's cache entry. A full Refresh would run
	// smartctl -a on EVERY disk and spin up any that are currently asleep —
	// waking the very drives this feature is meant to keep dormant. We
	// re-read just this disk (lsblk basics + SMART), leaving other disks'
	// cached state untouched.
	d, rerr := c.refreshSingleDiskLocked(ctx, devID)
	if rerr != nil {
		return nil, fmt.Errorf("磁盘已唤醒，但刷新该盘数据失败: %w", rerr)
	}
	msg := devID + " 已唤醒上线（请前往飞牛「硬盘设置」点击「挂载硬盘」后访问数据）"
	degraded := false
	// Degraded-array guard: only a genuine regression counts. If the disk had
	// NO mounted volume when it was offlined (rec.VolumePaths empty), having
	// none after wake is normal — not a failure. Only flag it when a volume
	// that WAS mounted before offline failed to come back.
	hadVolumes := len(rec.VolumePaths) > 0 || len(rec.Mounts) > 0
	if hadVolumes && d != nil && !raidVolumeBack(d) {
		degraded = true
		msg += "；⚠️ 提示：唤醒前有挂载的存储空间暂未回挂，请在飞牛中检查（可能需在存储管理手动挂载，或阵列未自动重组需 mdadm --assemble）"
	}
	if len(remountErrs) > 0 {
		msg += "（部分挂载点恢复失败: " + strings.Join(remountErrs, "; ") + "）"
	}
	// Log wake + any remount outcome.
	c.logPower(wakeDisk, "wake", len(remountErrs) == 0 && !degraded, msg)
	for _, mp := range remounted {
		c.logPower(wakeDisk, "remount", true, "补挂载 "+mp)
	}
	for _, re := range remountErrs {
		c.logPower(wakeDisk, "remount", false, re)
	}
	return &PowerResult{OK: true, Message: msg, Disk: d}, nil
}

// raidVolumeBack reports whether a RAID-member disk regained at least one
// mount point after wake. When a degraded/offlined member is rescanned the
// array often stays down, so none of its volumes reappear.
func raidVolumeBack(d *model.Disk) bool {
	return len(d.VolumePaths) > 0
}

// validateMountPoint rejects anything that is not a safe absolute mount path.
// Defense-in-depth: mount/umount run as root, so a crafted path from the
// hibernate store must never reach them.
func validateMountPoint(mp string) error {
	if mp == "" || mp == "/" {
		return fmt.Errorf("非法挂载点 %q", mp)
	}
	if !strings.HasPrefix(mp, "/") {
		return fmt.Errorf("挂载点必须是绝对路径: %q", mp)
	}
	if strings.Contains(mp, "..") {
		return fmt.Errorf("挂载点含非法 '..': %q", mp)
	}
	for _, r := range mp {
		if r == ' ' || r == '\t' || r == '\n' || r == ';' || r == '|' || r == '&' || r == '$' || r == '`' {
			return fmt.Errorf("挂载点含非法字符 %q: %q", string(r), mp)
		}
	}
	return nil
}

// liveDisk returns a fresh (non-cached) view of one disk for power ops, or
// ErrDiskMissing when the kernel no longer exposes it.
func (c *Collector) liveDisk(ctx context.Context, id string) (*model.Disk, error) {
	if !powerIDRe.MatchString(id) {
		return nil, fmt.Errorf("非法磁盘标识 %q", id)
	}
	d, err := c.refreshSingleDisk(ctx, id)
	return validateLiveDisk(id, d, err)
}

// liveDiskLocked is used by power operations while they own scanMu, keeping
// SMART collection from overlapping hardware state changes.
func (c *Collector) liveDiskLocked(ctx context.Context, id string) (*model.Disk, error) {
	if !powerIDRe.MatchString(id) {
		return nil, fmt.Errorf("非法磁盘标识 %q", id)
	}
	d, err := c.refreshSingleDiskLocked(ctx, id)
	return validateLiveDisk(id, d, err)
}

func validateLiveDisk(id string, d *model.Disk, err error) (*model.Disk, error) {
	if err != nil {
		return nil, err
	}
	if d != nil && d.ID == id && !d.Hibernated {
		return d, nil
	}
	return nil, ErrDiskMissing
}

// refreshSingleDisk re-collects ONLY one disk (lsblk basics + SMART + grade)
// and patches it into the cache, without touching other disks. Used by Wake
// so waking one drive does not spin up the rest via a full Refresh.
func (c *Collector) refreshSingleDisk(ctx context.Context, id string) (*model.Disk, error) {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	return c.refreshSingleDiskLocked(ctx, id)
}

func (c *Collector) refreshSingleDiskLocked(ctx context.Context, id string) (*model.Disk, error) {
	c.mu.RLock()
	startPowerRevision := c.powerRevision
	c.mu.RUnlock()

	inv, err := c.enumDisks(ctx)
	if err != nil {
		return nil, err
	}
	var node *lsblkNode
	for i := range inv.disks {
		if inv.disks[i].Name == id {
			node = &inv.disks[i]
			break
		}
	}
	if node == nil {
		return nil, ErrDiskMissing
	}
	lsusb := c.loadLsusb(ctx)
	d := c.baseDisk(*node)
	c.fillSmart(ctx, &d, lsusb)
	d.ManufactureDate = smart.InferManufactureDate(d.Model, d.Serial)
	d.CollectedAt = time.Now().Unix()
	// topology + btrfs for this disk only.
	single := []model.Disk{d}
	c.annotateTopology(single, inv.roots)
	d = single[0]
	c.fillBtrfsErrors(ctx, &d)
	smart.Grade(&d)
	// Re-attach the persisted user note; the fresh Disk starts empty and the
	// note must survive per-disk refreshes and wake flows.
	d.UserNote = c.hibernate.noteFor(&d)
	// Fresh, online disk: clear any hibernate flag and refresh recovery data.
	d.Hibernated = false
	d.OfflineAt = 0
	computePowerCaps(&d)
	c.hibernate.upsertFromDisk(&d)

	// Patch into the cache (replace by stable identity, else append).
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.powerStateRevision[d.ID] > startPowerRevision {
		for i := range c.cached {
			if c.cached[i].ID == d.ID {
				copyPowerState(&d, &c.cached[i])
				break
			}
		}
	}
	if c.hasCached {
		updated := cloneDisks(c.cached)
		for i := range c.cached {
			if c.cached[i].ID == d.ID || stableKey(&c.cached[i]) == stableKey(&d) {
				updated[i] = d
				c.cached = updated
				c.cachedAt = time.Now()
				if data, err := c.buildOverviewJSON(c.cached); err == nil {
					c.overview = overviewCache(data)
				}
				return &d, nil
			}
		}
		c.cached = append(updated, d)
		c.cachedAt = time.Now()
		if data, err := c.buildOverviewJSON(c.cached); err == nil {
			c.overview = overviewCache(data)
		}
	}
	return &d, nil
}

// RefreshDisk re-collects a single disk by id and patches the cache, without
// probing (and thus without waking) any other disk. Used by the detail page's
// per-disk refresh button.
func (c *Collector) RefreshDisk(ctx context.Context, id string) (*model.Disk, error) {
	if !powerIDRe.MatchString(id) {
		return nil, fmt.Errorf("非法磁盘标识 %q", id)
	}
	return c.refreshSingleDisk(ctx, id)
}

// findOfflineByID resolves an offline record for a request id that may be a
// current kernel name: it matches either the recorded (offline-time) device
// name or the serial/WWN of a currently-live disk with that name.
func (c *Collector) findOfflineByID(ctx context.Context, id string) *HibernateRecord {
	for _, rec := range c.hibernate.offlineRecords() {
		if rec.ID == id {
			cp := rec
			return &cp
		}
	}
	// The request id is a live kernel name; match by stable identity so a
	// re-lettered drive still resolves to its offline record.
	if disks, err := c.List(ctx); err == nil {
		for i := range disks {
			if disks[i].ID != id {
				continue
			}
			key := stableKey(&disks[i])
			for _, rec := range c.hibernate.offlineRecords() {
				if stableKeyRec(&rec) == key {
					cp := rec
					return &cp
				}
			}
		}
	}
	return nil
}

// mountPointsOf returns all mount points reachable from the disk (direct
// partitions first, then storage-chain nodes like lvm-on-md).
func mountPointsOf(d *model.Disk) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range d.Partitions {
		if p.MountPoint != "" && !seen[p.MountPoint] {
			seen[p.MountPoint] = true
			out = append(out, p.MountPoint)
		}
	}
	for _, hop := range d.Chain {
		if hop.MountPoint != "" && !seen[hop.MountPoint] {
			seen[hop.MountPoint] = true
			out = append(out, hop.MountPoint)
		}
	}
	return out
}

// writeSysfsState writes offline/running to the device state node. It goes
// through the runner (sh -c ... tee) so mock mode can intercept it exactly
// like every other command; sysfs writes need root, which the app has.
func (c *Collector) writeSysfsState(ctx context.Context, id, state string) error {
	p := fmt.Sprintf("/sys/block/%s/device/state", id)
	out, _, err := c.run.Run(ctx, "sh", "-c", fmt.Sprintf("echo %s | tee %s", state, p))
	if err != nil {
		return fmt.Errorf("%s -> %s: %w (%s)", p, state, err, strings.TrimSpace(out))
	}
	return nil
}

func (c *Collector) writeSysfsRescan(ctx context.Context, id string) error {
	p := fmt.Sprintf("/sys/block/%s/device/rescan", id)
	_, _, err := c.run.Run(ctx, "sh", "-c", fmt.Sprintf("echo 1 | tee %s", p))
	return err
}

func isMounted(ctx context.Context, run Runner, mp string) bool {
	out, _, err := run.Run(ctx, "findmnt", "-n", "-o", "TARGET", mp)
	return err == nil && strings.TrimSpace(out) != ""
}

// diskIOSectors returns the total sectors read+written for a disk from
// /proc/diskstats (fields 6 and 10, 1-indexed, after the device name).
// Returns -1 when the disk is not found.
func diskIOSectors(stats, id string) int64 {
	for _, line := range strings.Split(stats, "\n") {
		f := strings.Fields(line)
		if len(f) < 10 || f[2] != id {
			continue
		}
		read, err1 := strconv.ParseInt(f[5], 10, 64)
		written, err2 := strconv.ParseInt(f[9], 10, 64)
		if err1 != nil || err2 != nil {
			return -1
		}
		return read + written
	}
	return -1
}

// ioProbeInterval is how long we wait between the two diskstats samples. A
// package var so tests can shrink it.
var ioProbeInterval = 300 * time.Millisecond

// diskIOActive reports whether the disk shows any read/write sector progress
// between two quick samples of /proc/diskstats (i.e. it is mid-IO right now).
// When the diskstats are unreadable it errs on the safe side and reports
// false (don't block offline on a failed probe).
func (c *Collector) diskIOActive(ctx context.Context, id string) (bool, string) {
	read := func() (int64, bool) {
		out, _, err := c.run.Run(ctx, "cat", "/proc/diskstats")
		if err != nil {
			return -1, false
		}
		v := diskIOSectors(out, id)
		return v, v >= 0
	}
	a, ok := read()
	if !ok {
		return false, ""
	}
	select {
	case <-ctx.Done():
		return false, ""
	case <-time.After(ioProbeInterval):
	}
	b, ok := read()
	if !ok {
		return false, ""
	}
	if b > a {
		return true, fmt.Sprintf("IO 计数 %d → %d 正在增长", a, b)
	}
	return false, ""
}
