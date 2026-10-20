package collector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"smartdisk/internal/config"
	"smartdisk/internal/model"
)

// TestMockKeyStable documents the fixture filename derivation so mock files
// on disk stay in sync with mockKey().
func TestMockKeyStable(t *testing.T) {
	got := mockKey("lsblk", "-J", "-O", "-b")
	want := "lsblk_-J_-O_-b.txt"
	if got != want {
		t.Errorf("mockKey lsblk = %q want %q", got, want)
	}
	got = mockKey("smartctl", "-a", "-j", "-d", "sat", "/dev/sda")
	want = "smartctl_-a_-j_-d_sat__dev_sda.txt"
	if got != want {
		t.Errorf("mockKey smartctl = %q want %q", got, want)
	}
	got = mockKey("lsusb")
	want = "lsusb.txt"
	if got != want {
		t.Errorf("mockKey lsusb = %q want %q", got, want)
	}
}

// TestCollectorMockEndToEnd runs the full collector against the bundled mock
// fixtures and asserts the assembled inventory.
func TestCollectorMockEndToEnd(t *testing.T) {
	dir := findMockDir(t)
	cfg := config.Config{
		MockDir:      dir,
		LsblkPath:    "lsblk",
		LsusbPath:    "lsusb",
		SmartctlPath: "smartctl",
		DataDir:      t.TempDir(),
	}
	c := New(cfg)
	disks, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(disks) != 4 {
		t.Fatalf("expected 4 disks (loop filtered), got %d: %+v", len(disks), ids(disks))
	}

	byID := map[string]*model.Disk{}
	for i := range disks {
		byID[disks[i].ID] = &disks[i]
	}

	sda := byID["sda"]
	if sda == nil || sda.Model == "" || sda.Temperature != 38 {
		t.Errorf("sda wrong: %+v", sda)
	}
	// sda carries 2 btrfs corruption errors in the fixture, so it is now 警.
	if sda != nil && sda.Health != model.GradeWarning {
		t.Errorf("sda health=%v want 警 (btrfs corruption)", sda.Health)
	}
	// btrfs pool errors + power state + APM.
	if sda != nil {
		if sda.BtrfsErrs == nil || sda.BtrfsErrs.CorruptionErrs != 2 {
			t.Errorf("sda btrfs=%+v want corruption=2", sda.BtrfsErrs)
		}
		if sda.BtrfsMount != "/vol2" {
			t.Errorf("sda btrfsMount=%q want /vol2", sda.BtrfsMount)
		}
		if sda.PowerState != "active" {
			t.Errorf("sda power=%q want active", sda.PowerState)
		}
		if sda.APMLevel != 128 {
			t.Errorf("sda apm=%d want 128", sda.APMLevel)
		}
	}
	// sdc is in standby.
	sdc2 := byID["sdc"]
	if sdc2 != nil && sdc2.PowerState != "standby" {
		t.Errorf("sdc power=%q want standby", sdc2.PowerState)
	}
	// Capacity usage summed from mounted partitions.
	if sda != nil && sda.UsedCapacity != 2900000000000 {
		t.Errorf("sda used=%d want 2900000000000", sda.UsedCapacity)
	}
	if sda != nil && sda.UsagePercent != 72 {
		t.Errorf("sda usage%%=%d want 72", sda.UsagePercent)
	}
	// SMART attr CN name + importance populated.
	if sda != nil {
		var poh *model.SmartAttr
		for i := range sda.Attrs {
			if sda.Attrs[i].ID == 9 {
				poh = &sda.Attrs[i]
			}
		}
		if poh == nil || poh.Cn != "通电时间" || !poh.Important {
			t.Errorf("attr 9 cn/important wrong: %+v", poh)
		}
	}
	// A: sector sizes, self-test log, device stats.
	if sda != nil && sda.PhysicalSectorSize != 4096 {
		t.Errorf("sda physSector=%d want 4096", sda.PhysicalSectorSize)
	}
	if sda != nil && len(sda.SelfTests) != 3 {
		t.Errorf("sda selfTests=%d want 3", len(sda.SelfTests))
	}
	if sda != nil && len(sda.DevStats) == 0 {
		t.Errorf("sda devStats empty")
	}
	// E: SATA link.
	if sda != nil && (sda.Link == nil || sda.Link.Kind != "SATA") {
		t.Errorf("sda link=%+v", sda.Link)
	}
	// D: partitions.
	if sda != nil && len(sda.Partitions) != 1 {
		t.Errorf("sda partitions=%d want 1", len(sda.Partitions))
	}
	// Usage is aggregated from the whole subtree (incl. lvm layer for RAID).
	if sda != nil && sda.UsagePercent != 72 {
		t.Errorf("sda usage%%=%d want 72", sda.UsagePercent)
	}

	nvme := byID["nvme0n1"]
	if nvme == nil || nvme.PercentUsed != 7 || nvme.Type != model.TypeNVMe {
		t.Errorf("nvme wrong: %+v", nvme)
	}
	// E: NVMe link from sysfs.
	if nvme != nil && nvme.Link == nil {
		t.Errorf("nvme link nil")
	}

	// SATA SSD link downgraded (3.0 vs 6.0 Gb/s).
	sdb := byID["sdb"]
	if sdb != nil && (sdb.Link == nil || !sdb.Link.Downgraded) {
		t.Errorf("sdb link downgraded expected: %+v", sdb.Link)
	}

	// Topology: sda is a RAID member feeding md125->lvm->/vol2; sdb is system.
	if sda != nil {
		if sda.Role != "RAID 成员" {
			t.Errorf("sda role=%q want RAID 成员", sda.Role)
		}
		found := false
		for _, v := range sda.VolumePaths {
			if v == "/vol2" {
				found = true
			}
		}
		if !found {
			t.Errorf("sda vols=%v want to contain /vol2", sda.VolumePaths)
		}
		if len(sda.Chain) != 3 {
			t.Errorf("sda chain=%d want 3 (part,raid,lvm)", len(sda.Chain))
		}
	}
	if sdb != nil && sdb.Role != "系统盘" {
		t.Errorf("sdb role=%q want 系统盘", sdb.Role)
	}

	// Extra hardware detail (WWN, cache, NCQ, temp history, load cycles, PHY).
	if sda != nil {
		if sda.WWN == "" {
			t.Errorf("sda WWN empty")
		}
		if sda.CacheSizeBytes != 268435456 {
			t.Errorf("sda cache=%d", sda.CacheSizeBytes)
		}
		if sda.NCQDepth != 32 {
			t.Errorf("sda ncq=%d", sda.NCQDepth)
		}
		if sda.TempHistory == nil || sda.TempHistory.Max != 52 {
			t.Errorf("sda tempHistory=%+v", sda.TempHistory)
		}
		if sda.LoadCycleCount != 1832 {
			t.Errorf("sda loadCycle=%d want 1832", sda.LoadCycleCount)
		}
		if len(sda.SATAPhy) != 2 {
			t.Errorf("sda sataPhy=%d want 2", len(sda.SATAPhy))
		}
	}

	// USB disk sdc is rotational and in standby. The cold-cache scan must keep
	// its base inventory while deferring the SMART command.
	sdc := byID["sdc"]
	if sdc == nil || sdc.Interface != model.IfUSB {
		t.Fatalf("sdc wrong: %+v", sdc)
	}
	if !sdc.SmartDeferred || sdc.SmartAvail || sdc.PowerState != "standby" {
		t.Errorf("sdc should be deferred while standby: %+v", sdc)
	}
	if !sdc.ToSummary().SmartDeferred {
		t.Error("overview summary lost smartDeferred state")
	}
	refreshedSdc, err := c.RefreshDisk(context.Background(), "sdc")
	if err != nil {
		t.Fatalf("RefreshDisk(sdc): %v", err)
	}
	if !refreshedSdc.SmartAvail || refreshedSdc.SmartDeferred {
		t.Errorf("explicit single-disk refresh should read sdc SMART: %+v", refreshedSdc)
	}
}

type recordingRunner struct {
	inner Runner
	mu    sync.Mutex
	calls []string
}

func (r *recordingRunner) Run(ctx context.Context, argv ...string) (string, string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, strings.Join(argv, " "))
	r.mu.Unlock()
	return r.inner.Run(ctx, argv...)
}

func (r *recordingRunner) callsCopy() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func TestColdListChecksPowerBeforeSMARTAndDefersStandby(t *testing.T) {
	dir := findMockDir(t)
	c := New(config.Config{MockDir: dir, LsblkPath: "lsblk", LsusbPath: "lsusb", SmartctlPath: "smartctl", DataDir: t.TempDir()})
	recorder := &recordingRunner{inner: c.run}
	c.run = recorder

	disks, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var sda, sdc *model.Disk
	for i := range disks {
		switch disks[i].ID {
		case "sda":
			sda = &disks[i]
		case "sdc":
			sdc = &disks[i]
		}
	}
	if sda == nil || sda.SmartDeferred || !sda.SmartAvail {
		t.Fatalf("awake disk sda should be read: %+v", sda)
	}
	if sdc == nil || !sdc.SmartDeferred || sdc.SmartAvail {
		t.Fatalf("standby disk sdc should be deferred: %+v", sdc)
	}

	calls := recorder.callsCopy()
	probe, full := -1, -1
	for i, call := range calls {
		if strings.Contains(call, "smartctl -n standby -d sat /dev/sda") && probe < 0 {
			probe = i
		}
		if strings.Contains(call, "smartctl -a -j -d sat /dev/sda") && full < 0 {
			full = i
		}
		if strings.Contains(call, "smartctl -a -j") && strings.Contains(call, "/dev/sdc") {
			t.Fatalf("cold scan issued full SMART read to standby sdc: %q", call)
		}
	}
	if probe < 0 || full < 0 || probe >= full {
		t.Fatalf("expected standby probe before full SMART read for sda; calls=%v", calls)
	}
}

// A full refresh must also defer sleeping rotational disks instead of waking
// them: the user's "refresh" button reports the true standby state read via
// the wake-free smartctl -n standby probe.
func TestRefreshDefersStandbyDisks(t *testing.T) {
	dir := findMockDir(t)
	c := New(config.Config{MockDir: dir, LsblkPath: "lsblk", LsusbPath: "lsusb", SmartctlPath: "smartctl", DataDir: t.TempDir()})
	recorder := &recordingRunner{inner: c.run}
	c.run = recorder

	disks, err := c.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	var sdc *model.Disk
	for i := range disks {
		if disks[i].ID == "sdc" {
			sdc = &disks[i]
		}
	}
	if sdc == nil || !sdc.SmartDeferred || sdc.SmartAvail || sdc.PowerState != "standby" {
		t.Fatalf("refresh must keep standby sdc deferred: %+v", sdc)
	}
	for _, call := range recorder.callsCopy() {
		if strings.Contains(call, "smartctl -a -j") && strings.Contains(call, "/dev/sdc") {
			t.Fatalf("refresh issued full SMART read to standby sdc: %q", call)
		}
	}
	if !strings.Contains(strings.Join(recorder.callsCopy(), "\n"), "smartctl -n standby") {
		t.Fatal("refresh did not run the wake-free standby probe")
	}
}

func TestColdListDefersRotationalDiskWhenPowerStateIsUnknown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lsblk_-J_-O_-b.txt"), []byte(`{"blockdevices":[{"name":"sda","path":"/dev/sda","type":"disk","tran":"sata","size":"2000","model":"WDC","serial":"Z1","rota":true}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c := New(config.Config{MockDir: dir, LsblkPath: "lsblk", SmartctlPath: "smartctl", DataDir: t.TempDir()})
	r := &recordingRunner{inner: c.run}
	c.run = r
	disks, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(disks) != 1 || !disks[0].SmartDeferred || disks[0].SmartAvail {
		t.Fatalf("unknown rotational power state should defer SMART: %+v", disks)
	}
	for _, call := range r.callsCopy() {
		if strings.Contains(call, "smartctl -a -j") {
			t.Fatalf("unknown-power disk was read without confirmation: %q", call)
		}
	}
}

func TestTopologyUsesCachedDiskData(t *testing.T) {
	dir := findMockDir(t)
	c := New(config.Config{MockDir: dir, LsblkPath: "lsblk", LsusbPath: "lsusb", SmartctlPath: "smartctl", DataDir: t.TempDir()})
	recorder := &recordingRunner{inner: c.run}
	c.run = recorder
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("List: %v", err)
	}
	recorder.mu.Lock()
	recorder.calls = nil
	recorder.mu.Unlock()
	if _, err := c.Topology(context.Background()); err != nil {
		t.Fatalf("Topology: %v", err)
	}
	for _, call := range recorder.callsCopy() {
		if strings.Contains(call, "smartctl -a -j") {
			t.Fatalf("Topology performed a new SMART read: %q", call)
		}
	}
}

func TestListReturnsIsolatedCacheSnapshot(t *testing.T) {
	dir := findMockDir(t)
	c := New(config.Config{MockDir: dir, LsblkPath: "lsblk", LsusbPath: "lsusb", SmartctlPath: "smartctl", DataDir: t.TempDir()})
	disks, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for i := range disks {
		if disks[i].ID == "sda" {
			disks[i].Model = "caller mutation"
			if len(disks[i].Attrs) > 0 {
				disks[i].Attrs[0].Name = "caller mutation"
			}
		}
	}
	d, err := c.Get(context.Background(), "sda")
	if err != nil || d == nil {
		t.Fatalf("Get sda: disk=%+v err=%v", d, err)
	}
	if d.Model == "caller mutation" || (len(d.Attrs) > 0 && d.Attrs[0].Name == "caller mutation") {
		t.Fatalf("caller changed cached snapshot: %+v", d)
	}
}

type limitedRunner struct {
	inner  Runner
	active atomic.Int32
	max    atomic.Int32
}

func (r *limitedRunner) Run(ctx context.Context, argv ...string) (string, string, error) {
	fullRead := false
	for _, arg := range argv {
		if arg == "-a" {
			fullRead = true
			break
		}
	}
	if fullRead {
		active := r.active.Add(1)
		defer r.active.Add(-1)
		for max := r.max.Load(); active > max; max = r.max.Load() {
			if r.max.CompareAndSwap(max, active) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return r.inner.Run(ctx, argv...)
}

func TestCollectorCapsConcurrentSMARTReadsAtTwo(t *testing.T) {
	dir := findMockDir(t)
	c := New(config.Config{MockDir: dir, LsblkPath: "lsblk", LsusbPath: "lsusb", SmartctlPath: "smartctl", DataDir: t.TempDir()})
	r := &limitedRunner{inner: c.run}
	c.run = r
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := r.max.Load(); got != 2 {
		t.Fatalf("max concurrent full SMART reads=%d, want 2", got)
	}
}

type blockingRunner struct {
	inner   Runner
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingRunner) Run(ctx context.Context, argv ...string) (string, string, error) {
	joined := strings.Join(argv, " ")
	if strings.Contains(joined, "smartctl -a -j -d sat /dev/sda") {
		r.once.Do(func() {
			close(r.started)
			select {
			case <-r.release:
			case <-ctx.Done():
			}
		})
	}
	return r.inner.Run(ctx, argv...)
}

func TestPowerUpdateDuringRefreshSurvivesSnapshotPublish(t *testing.T) {
	dir := findMockDir(t)
	c := New(config.Config{MockDir: dir, LsblkPath: "lsblk", LsusbPath: "lsusb", SmartctlPath: "smartctl", DataDir: t.TempDir()})
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("initial List: %v", err)
	}
	r := &blockingRunner{inner: c.run, started: make(chan struct{}), release: make(chan struct{})}
	c.run = r
	refreshErr := make(chan error, 1)
	go func() {
		_, err := c.Refresh(context.Background())
		refreshErr <- err
	}()
	select {
	case <-r.started:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not reach the blocked SMART command")
	}
	current, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("cached List during refresh: %v", err)
	}
	var updated model.Disk
	for _, d := range current {
		if d.ID == "sda" {
			updated = d
		}
	}
	if updated.ID == "" {
		t.Fatal("sda missing from cache")
	}
	updated.PowerState = "standby"
	c.updateCachedPowerState("sda", &updated)
	close(r.release)
	if err := <-refreshErr; err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	current, err = c.List(context.Background())
	if err != nil {
		t.Fatalf("List after refresh: %v", err)
	}
	for _, d := range current {
		if d.ID == "sda" && d.PowerState != "standby" {
			t.Fatalf("refresh overwrote newer power state: got %q", d.PowerState)
		}
	}
}

func ids(disks []model.Disk) []string {
	out := []string{}
	for _, d := range disks {
		out = append(out, d.ID)
	}
	return out
}

// findMockDir locates server/mock relative to the test package.
func findMockDir(t *testing.T) string {
	t.Helper()
	// package dir = server/internal/collector; mock at server/mock
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(wd, "..", "..", "mock")
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(p)
		return abs
	}
	t.Skipf("mock dir not found at %s", p)
	return ""
}
