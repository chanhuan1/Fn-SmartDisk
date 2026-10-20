package collector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"smartdisk/internal/config"
	"smartdisk/internal/model"
)

// scriptRunner answers commands by exact argv match and records the call
// sequence so tests can assert ordering (sync -> umount -> hdparm -> sysfs).
type scriptRunner struct {
	calls []string
	// resp maps a command prefix (joined argv) to stdout; missing -> error.
	resp map[string]string
}

func (s *scriptRunner) Run(_ context.Context, argv ...string) (string, string, error) {
	key := strings.Join(argv, " ")
	s.calls = append(s.calls, key)
	// longest-prefix match so fixtures can key on "smartctl -a -j" etc.
	best := ""
	for k := range s.resp {
		if strings.HasPrefix(key, k) && len(k) > len(best) {
			best = k
		}
	}
	if best == "" {
		return "", "", fmt.Errorf("no fixture for %q", key)
	}
	return s.resp[best], "", nil
}

func newTestCollector(t *testing.T, resp map[string]string) (*Collector, *scriptRunner) {
	t.Helper()
	r := &scriptRunner{resp: resp}
	cfg := config.Config{
		SmartctlPath: "smartctl",
		HdparmPath:   "hdparm",
		LsblkPath:    "lsblk",
		DataDir:      t.TempDir(),
	}
	c := &Collector{
		cfg:       cfg,
		run:       r,
		hibernate: loadHibernateStore(filepath.Join(cfg.DataDir, "hibernate.json")),
		powerLog:  loadPowerLogStore(filepath.Join(cfg.DataDir, "power-events.json")),
	}
	return c, r
}

// sataHDDLsblk is a minimal one-disk inventory: sda -> sda1 (ext4, /vol9).
const sataHDDLsblk = `{"blockdevices":[{"name":"sda","path":"/dev/sda","type":"disk","tran":"sata",
"size":"2000398934016","model":"WDC WD40","serial":"WD-WX11","rota":true,
"children":[{"name":"sda1","path":"/dev/sda1","type":"part","size":"2000396746752",
"fstype":"ext4","uuid":"uu-11","mountpoint":"/vol9","mountpoints":["/vol9"]}]}]}`

// sysHDDLsblk has the root mount on the disk -> must be untouchable.
const sysHDDLsblk = `{"blockdevices":[{"name":"sdb","path":"/dev/sdb","type":"disk","tran":"sata",
"size":"500107862016","model":"ST500","serial":"ST-1","rota":true,
"children":[{"name":"sdb1","path":"/dev/sdb1","type":"part","size":"500096991232",
"fstype":"ext4","uuid":"root-1","mountpoint":"/","mountpoints":["/"]}]}]}`

// raidHDDLsblk: sdc -> sdc1 (raid member) -> md0 (btrfs /vol3).
const raidHDDLsblk = `{"blockdevices":[{"name":"sdc","path":"/dev/sdc","type":"disk","tran":"sata",
"size":"2000398934016","model":"WDC WD40","serial":"WD-WX22","rota":true,
"children":[{"name":"sdc1","path":"/dev/sdc1","type":"part","size":"2000396746752",
"fstype":"linux_raid_member","uuid":"raid-uu",
"children":[{"name":"md0","path":"/dev/md0","type":"raid1","size":"2000395603968",
"fstype":"btrfs","uuid":"btrfs-uu","mountpoint":"/vol3","mountpoints":["/vol3"]}]}]}]}`

func smartctlStandby(state string) string {
	return "Device is in " + state + " mode, exit(2)"
}

// baseResp answers everything the collector needs for a successful collect.
func baseResp(lsblk, smartState string) map[string]string {
	return map[string]string{
		"lsblk -J -O -b": lsblk,
		"smartctl -a -j -d sat /dev/sda": `{"device":{"name":"/dev/sda"},"model_name":"WDC WD40",
"serial_number":"WD-WX11","rotation_rate":5400,"smart_status":{"passed":true},
"ata_smart_attributes":{"revision":16,"table":[]}}`,
		"smartctl -a -j -d sat /dev/sdb": `{"device":{"name":"/dev/sdb"},"model_name":"ST500",
"serial_number":"ST-1","rotation_rate":7200,"smart_status":{"passed":true},
"ata_smart_attributes":{"revision":16,"table":[]}}`,
		"smartctl -a -j -d sat /dev/sdc": `{"device":{"name":"/dev/sdc"},"model_name":"WDC WD40",
"serial_number":"WD-WX22","rotation_rate":5400,"smart_status":{"passed":true},
"ata_smart_attributes":{"revision":16,"table":[]}}`,
		"smartctl -n standby":                  smartctlStandby(smartState),
		"lsusb":                                "",
		"sync":                                 "",
		"hdparm -y":                            "",
		"umount":                               "",
		"mount":                                "",
		"findmnt":                              "",
		"sh -c echo offline | tee /sys/block/": "offline\n",
		"sh -c echo running | tee /sys/block/": "running\n",
		"sh -c echo 1 | tee /sys/block/":       "1\n",
	}
}

func TestSleepIssuesHdparmOnly(t *testing.T) {
	c, r := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	res, err := c.Sleep(context.Background(), "sda")
	if err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if !res.OK {
		t.Fatalf("not ok: %+v", res)
	}
	// Must contain exactly one hdparm -y and no umount/sysfs writes.
	var hdparm, umount, sysfs int
	for _, call := range r.calls {
		switch {
		case strings.HasPrefix(call, "hdparm -y"):
			hdparm++
		case strings.HasPrefix(call, "umount"):
			umount++
		case strings.HasPrefix(call, "sh -c echo"):
			sysfs++
		}
	}
	if hdparm != 1 {
		t.Errorf("hdparm calls=%d want 1; calls=%v", hdparm, r.calls)
	}
	if umount != 0 || sysfs != 0 {
		t.Errorf("sleep must not umount/touch sysfs: umount=%d sysfs=%d", umount, sysfs)
	}
}

// After Sleep, the overview cache must immediately report the new power
// state (previously it stayed stale until the next manual refresh).
func TestSleepUpdatesCachedPowerState(t *testing.T) {
	// Start ACTIVE; after hdparm -y the drive reports STANDBY.
	resp := baseResp(sataHDDLsblk, "ACTIVE")
	c, r := newTestCollector(t, resp)
	// Prime the cache (active).
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("List: %v", err)
	}
	// Now make the standby probe report standby for subsequent calls.
	resp["smartctl -n standby"] = "Device is in STANDBY mode, exit(2)"
	_ = r
	if _, err := c.Sleep(context.Background(), "sda"); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	disks, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var d *model.Disk
	for i := range disks {
		if disks[i].ID == "sda" {
			d = &disks[i]
		}
	}
	if d == nil {
		t.Fatal("sda missing")
	}
	if d.PowerState != "standby" {
		t.Fatalf("cached PowerState=%q, want standby (UI would show stale state)", d.PowerState)
	}
}

func TestSleepRejectsSystemDisk(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(sysHDDLsblk, "ACTIVE"))
	_, err := c.Sleep(context.Background(), "sdb")
	if !errors.Is(err, ErrSystemDisk) {
		t.Fatalf("want ErrSystemDisk, got %v", err)
	}
	if !strings.Contains(err.Error(), "系统盘") {
		t.Fatalf("error should mention 系统盘: %v", err)
	}
}

func TestOfflineSequence(t *testing.T) {
	c, r := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	res, err := c.Offline(context.Background(), "sda", false, false)
	if err != nil {
		t.Fatalf("Offline: %v", err)
	}
	if !res.Disk.Hibernated || res.Disk.PowerState != "offline" {
		t.Fatalf("disk not marked offline: %+v", res.Disk)
	}
	// Find indices of the key commands to assert ordering.
	idx := func(prefix string) int {
		for i, call := range r.calls {
			if strings.HasPrefix(call, prefix) {
				return i
			}
		}
		return -1
	}
	iSync := idx("sync")
	iUmount := idx("umount -l /vol9")
	iHdparm := idx("hdparm -y /dev/sda")
	iSysfs := idx("sh -c echo offline")
	if iSync < 0 || iUmount < 0 || iHdparm < 0 || iSysfs < 0 {
		t.Fatalf("missing commands: %v", r.calls)
	}
	if !(iSync < iUmount && iUmount < iHdparm && iHdparm < iSysfs) {
		t.Fatalf("bad order: sync=%d umount=%d hdparm=%d sysfs=%d", iSync, iUmount, iHdparm, iSysfs)
	}
	// Hibernate record persisted with mount + UUID.
	rec := c.hibernate.get("sn:WD-WX11")
	if rec == nil || !rec.Offline {
		t.Fatalf("record not persisted: %+v", rec)
	}
	if len(rec.Mounts) != 1 || rec.Mounts[0].UUID != "uu-11" || rec.Mounts[0].MountPoint != "/vol9" {
		t.Fatalf("bad mounts: %+v", rec.Mounts)
	}
}

func TestOfflineRaidMemberNeedsConfirm(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(raidHDDLsblk, "ACTIVE"))
	_, err := c.Offline(context.Background(), "sdc", false, false)
	if !errors.Is(err, ErrNeedRaidConfirm) {
		t.Fatalf("want ErrNeedRaidConfirm, got %v", err)
	}
	// With confirmation it proceeds (umount of /vol3 comes from the chain).
	res, err := c.Offline(context.Background(), "sdc", true, true)
	if err != nil {
		t.Fatalf("Offline with confirm: %v", err)
	}
	if !res.OK {
		t.Fatalf("not ok: %+v", res)
	}
	rec := c.hibernate.get("sn:WD-WX22")
	if rec == nil || !rec.RaidMember {
		t.Fatalf("raid flag missing: %+v", rec)
	}
	// btrfs on raid must be marked Noauto (kernel reassembles on rescan).
	if len(rec.Mounts) != 1 || !rec.Mounts[0].Noauto {
		t.Fatalf("btrfs-on-raid mount should be Noauto: %+v", rec.Mounts)
	}
}

func TestWakeRestoresMount(t *testing.T) {
	c, r := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	if _, err := c.Offline(context.Background(), "sda", false, false); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	// Pretend the device node does NOT exist on this dev machine; wake should
	// fail waiting for it unless we fake it. Create a placeholder file.
	// On macOS /dev/sda never appears, so wake hits the spin-up timeout path
	// (waitForDevice polls statDevice until the 15s deadline). To keep the
	// test fast we shrink the deadline via the package var.
	old := wakeTimeout
	wakeTimeout = 300 * time.Millisecond
	defer func() { wakeTimeout = old }()
	_, err := c.Wake(context.Background(), "sda")
	if err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("expected spin-up timeout on dev machine, got %v", err)
	}
	// running must have been issued (rescan now happens only after the
	// device node appears, which never occurs in this timeout test).
	var running bool
	for _, call := range r.calls {
		if strings.HasPrefix(call, "sh -c echo running") {
			running = true
		}
	}
	if !running {
		t.Fatalf("wake did not write running: %v", r.calls)
	}
}

func TestWakeWhenNotOffline(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	res, err := c.Wake(context.Background(), "sda")
	if err != nil {
		t.Fatalf("wake of online disk should be a no-op success: %v", err)
	}
	if !strings.Contains(res.Message, "已在线") {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestMergeOfflinePlaceholder(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	if _, err := c.Offline(context.Background(), "sda", false, false); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	// Now lsblk no longer lists sda (drive is offline): simulate an empty
	// inventory and verify the placeholder shows up with a Wake capability.
	r2 := &scriptRunner{resp: baseResp(`{"blockdevices":[]}`, "ACTIVE")}
	c.run = r2
	disks, err := c.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("want 1 placeholder, got %d: %+v", len(disks), disks)
	}
	d := disks[0]
	if d.ID != "sda" || !d.Hibernated || d.PowerState != "offline" || !d.CanWake {
		t.Fatalf("bad placeholder: %+v", d)
	}
	if d.Model != "WDC WD40" || d.Serial != "WD-WX11" {
		t.Fatalf("placeholder lost identity: %+v", d)
	}
}

// An offlined disk that STILL appears in lsblk (offlined devices keep their
// node with state=offline) must stay flagged hibernated after a refresh, so
// the Wake button does not vanish. Regression for the real-device case.
func TestOfflineFlagSurvivesRefreshWhenDiskStillVisible(t *testing.T) {
	resp := baseResp(sataHDDLsblk, "ACTIVE")
	c, _ := newTestCollector(t, resp)
	if _, err := c.Offline(context.Background(), "sda", false, false); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	// After a real offline the kernel reports state=offline for the device,
	// so the refresh below still sees sda as offline (not self-healed away).
	resp["cat /sys/block/sda/device/state"] = "offline\n"
	// Refresh with the SAME lsblk (sda still present, state now standby-ish).
	disks, err := c.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	var d *model.Disk
	for i := range disks {
		if disks[i].ID == "sda" {
			d = &disks[i]
		}
	}
	if d == nil {
		t.Fatal("sda missing")
	}
	if !d.Hibernated || d.PowerState != "offline" || !d.CanWake {
		t.Fatalf("offline flag lost after refresh: hibernated=%v power=%q canWake=%v",
			d.Hibernated, d.PowerState, d.CanWake)
	}
}

func TestPowerCaps(t *testing.T) {
	sys := &model.Disk{ID: "sdb", Interface: model.IfSATA, Rotational: true, Roles: []string{"系统盘"}}
	computePowerCaps(sys)
	if sys.CanSleep || sys.CanOffline || sys.NoSleepReason == "" {
		t.Fatalf("system disk must be blocked: %+v", sys)
	}
	nvme := &model.Disk{ID: "nvme0n1", Interface: model.IfNVMe}
	computePowerCaps(nvme)
	if nvme.CanSleep || !strings.Contains(nvme.NoSleepReason, "NVMe") {
		t.Fatalf("nvme must be blocked: %+v", nvme)
	}
	ssd := &model.Disk{ID: "sdd", Interface: model.IfSATA, Rotational: false}
	computePowerCaps(ssd)
	if ssd.CanSleep || !strings.Contains(ssd.NoSleepReason, "固态") {
		t.Fatalf("ssd must be blocked: %+v", ssd)
	}
	hdd := &model.Disk{ID: "sda", Interface: model.IfSATA, Rotational: true}
	computePowerCaps(hdd)
	if !hdd.CanSleep || !hdd.CanOffline || hdd.CanWake || hdd.NeedsRaidConfirm {
		t.Fatalf("plain hdd caps wrong: %+v", hdd)
	}
	raid := &model.Disk{ID: "sdc", Interface: model.IfSATA, Rotational: true, Roles: []string{"RAID 成员"}}
	computePowerCaps(raid)
	if !raid.CanOffline || !raid.NeedsRaidConfirm {
		t.Fatalf("raid member needs confirm: %+v", raid)
	}
	off := &model.Disk{ID: "sda", Hibernated: true}
	computePowerCaps(off)
	if !off.CanWake || off.CanSleep {
		t.Fatalf("offline disk caps wrong: %+v", off)
	}
}

func TestInvalidIDRejected(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	for _, bad := range []string{"../etc", "sda;rm", "sd a", ""} {
		if _, err := c.Sleep(context.Background(), bad); err == nil {
			t.Fatalf("id %q must be rejected", bad)
		}
	}
}

// Wake must resolve the offline record even when the drive's kernel name
// changed across the offline/wake boundary (fnOS re-letters drives). The
// record is keyed by serial, so a request using the NEW kernel name (same
// serial) still finds it, and sysfs writes use the recorded offline-time
// device name.
func TestWakeResolvesReletteredDrive(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	if _, err := c.Offline(context.Background(), "sda", false, false); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	// Record is keyed by serial now.
	rec := c.hibernate.get("sn:WD-WX11")
	if rec == nil || !rec.Offline {
		t.Fatalf("offline record not keyed by serial: %+v", rec)
	}
	// findOfflineByID must map a current kernel name (same serial, possibly
	// re-lettered) back to the offline record.
	live := &model.Disk{ID: "sdd", Serial: "WD-WX11"}
	if got := c.hibernate.getByDisk(live); got == nil || got.ID != "sda" {
		t.Fatalf("getByDisk(relettered serial) = %+v, want sda record", got)
	}
	// And the recorded offline-time device name is what wake must use for
	// sysfs writes (sda), independent of the request's kernel name.
	if rec.ID != "sda" {
		t.Fatalf("record ID=%q, want sda (offline-time device name)", rec.ID)
	}
}

// mixedHDDLsblk: one disk feeding BOTH an md array (btrfs /vol3) AND a
// directly-mounted partition (ext4 /vol8). Regression test for the case
// where the direct partition must be re-mounted on wake while the RAID fs
// must be left to the kernel (Noauto).
const mixedHDDLsblk = `{"blockdevices":[{"name":"sdd","path":"/dev/sdd","type":"disk","tran":"sata",
"size":"2000398934016","model":"WDC WD40","serial":"WD-WX33","rota":true,
"children":[
 {"name":"sdd1","path":"/dev/sdd1","type":"part","size":"1000198840320",
  "fstype":"linux_raid_member","uuid":"raid-uu",
  "children":[{"name":"md0","path":"/dev/md0","type":"raid1","size":"1000197697536",
   "fstype":"btrfs","uuid":"btrfs-uu","mountpoint":"/vol3","mountpoints":["/vol3"]}]},
 {"name":"sdd2","path":"/dev/sdd2","type":"part","size":"1000198840320",
  "fstype":"ext4","uuid":"ext4-uu","mountpoint":"/vol8","mountpoints":["/vol8"]}
]}]}`

func TestMixedRaidAndDirectPartition(t *testing.T) {
	resp := baseResp(mixedHDDLsblk, "ACTIVE")
	resp["smartctl -a -j -d sat /dev/sdd"] = `{"device":{"name":"/dev/sdd"},"model_name":"WDC WD40",
"serial_number":"WD-WX33","rotation_rate":5400,"smart_status":{"passed":true},
"ata_smart_attributes":{"revision":16,"table":[]}}`
	c, _ := newTestCollector(t, resp)
	// RAID member: needs confirm.
	if _, err := c.Offline(context.Background(), "sdd", true, true); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	rec := c.hibernate.get("sn:WD-WX33")
	if rec == nil {
		t.Fatal("no record")
	}
	var direct, raid *MountRef
	for i := range rec.Mounts {
		m := &rec.Mounts[i]
		switch m.MountPoint {
		case "/vol8":
			direct = m
		case "/vol3":
			raid = m
		}
	}
	if direct == nil {
		t.Fatalf("direct partition mount missing: %+v", rec.Mounts)
	}
	if direct.Noauto {
		t.Fatalf("direct partition must NOT be Noauto (kernel won't bring it back): %+v", direct)
	}
	if raid == nil || !raid.Noauto {
		t.Fatalf("raid fs must be Noauto: %+v", raid)
	}
}

// dupMountLsblk: the same btrfs UUID appears on both a partition and the
// chain; the mount must be recorded only once.
func TestMountPointDeduplicated(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	if _, err := c.Offline(context.Background(), "sda", false, false); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	rec := c.hibernate.get("sn:WD-WX11")
	count := 0
	for _, m := range rec.Mounts {
		if m.MountPoint == "/vol9" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("mount point recorded %d times, want 1: %+v", count, rec.Mounts)
	}
}

func TestValidateMountPoint(t *testing.T) {
	good := []string{"/vol1", "/vol2/data", "/mnt/disk-1"}
	for _, mp := range good {
		if err := validateMountPoint(mp); err != nil {
			t.Errorf("%q should be valid: %v", mp, err)
		}
	}
	bad := []string{"", "/", "vol1", "/vol1/../etc", "/vol 1", "/vol1;rm", "/vol1|x", "/vol$(id)"}
	for _, mp := range bad {
		if err := validateMountPoint(mp); err == nil {
			t.Errorf("%q should be rejected", mp)
		}
	}
}

// Remount failure must keep the disk flagged offline (UI still offers Wake).
func TestWakeRemountFailureKeepsOfflineFlag(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	if _, err := c.Offline(context.Background(), "sda", false, false); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	// Stub the device node so wake passes the spin-up wait.
	oldStat := statDevice
	statDevice = func(string) (os.FileInfo, error) { return nil, nil }
	defer func() { statDevice = oldStat }()
	// Make mount fail by removing its fixture.
	r2 := &scriptRunner{resp: map[string]string{}}
	for k, v := range baseResp(sataHDDLsblk, "ACTIVE") {
		r2.resp[k] = v
	}
	delete(r2.resp, "mount")
	c.run = r2
	if _, err := c.Wake(context.Background(), "sda"); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	rec := c.hibernate.get("sn:WD-WX11")
	if rec == nil {
		t.Fatal("no record")
	}
	if !rec.Offline {
		t.Fatalf("remount failure must keep Offline flag, got Offline=false")
	}
}

// Waking a RAID-member disk whose array did NOT come back must surface a
// degraded-array warning (and not claim a clean success).
func TestWakeRaidDegradedWarns(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(raidHDDLsblk, "ACTIVE"))
	if _, err := c.Offline(context.Background(), "sdc", true, true); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	// Stub device node present so wake passes the spin-up wait.
	oldStat := statDevice
	statDevice = func(string) (os.FileInfo, error) { return nil, nil }
	defer func() { statDevice = oldStat }()
	// After wake, the array is gone: lsblk shows the disk with NO array,
	// hence no /vol3 mount. All chain mounts are Noauto, so nothing is
	// re-mounted by hand -> VolumePaths ends up empty -> degraded warning.
	degraded := `{"blockdevices":[{"name":"sdc","path":"/dev/sdc","type":"disk","tran":"sata",
"size":"2000398934016","model":"WDC WD40","serial":"WD-WX22","rota":true,
"children":[{"name":"sdc1","path":"/dev/sdc1","type":"part","size":"2000396746752",
"fstype":"linux_raid_member","uuid":"raid-uu"}]}]}`
	r2 := &scriptRunner{resp: baseResp(degraded, "ACTIVE")}
	c.run = r2
	res, err := c.Wake(context.Background(), "sdc")
	if err != nil {
		t.Fatalf("Wake: %v", err)
	}
	if !strings.Contains(res.Message, "存储空间暂未回挂") {
		t.Fatalf("expected degraded-array warning, got: %s", res.Message)
	}
	// A failed-wake event must be logged (not a clean "成功").
	evts := c.PowerEvents(0)
	var wakeEvt *model.PowerEvent
	for i := range evts {
		if evts[i].Action == "wake" {
			wakeEvt = &evts[i]
			break
		}
	}
	if wakeEvt == nil || wakeEvt.OK {
		t.Fatalf("degraded wake must be logged as failure: %+v", wakeEvt)
	}
}

// A disk that had NO mounted volume when offlined (rec.VolumePaths empty)
// must NOT be flagged as degraded after wake — having no volume is its normal
// state. Regression for the ST500 cold-backup disk false alarm.
func TestWakeNoVolumeDiskNotFlagged(t *testing.T) {
	// noMountHDDLsblk: a disk with a partition but no mount point.
	noMount := `{"blockdevices":[{"name":"sdb","path":"/dev/sdb","type":"disk","tran":"sata",
"size":"500107862016","model":"ST500","serial":"ST-9","rota":true,
"children":[{"name":"sdb1","path":"/dev/sdb1","type":"part","size":"500096991232",
"fstype":"linux_raid_member","uuid":"raid-uu",
"children":[{"name":"md126","path":"/dev/md126","type":"raid1","size":"500085293056",
"fstype":"ext4","uuid":"ext4-uu"}]}]}]}`
	resp := baseResp(noMount, "ACTIVE")
	resp["smartctl -a -j -d sat /dev/sdb"] = `{"device":{"name":"/dev/sdb"},"model_name":"ST500",
"serial_number":"ST-9","rotation_rate":7200,"smart_status":{"passed":true},
"ata_smart_attributes":{"revision":16,"table":[]}}`
	c, _ := newTestCollector(t, resp)
	if _, err := c.Offline(context.Background(), "sdb", true, true); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	oldStat := statDevice
	statDevice = func(string) (os.FileInfo, error) { return nil, nil }
	defer func() { statDevice = oldStat }()
	res, err := c.Wake(context.Background(), "sdb")
	if err != nil {
		t.Fatalf("Wake: %v", err)
	}
	if strings.Contains(res.Message, "暂未回挂") || strings.Contains(res.Message, "⚠️") {
		t.Fatalf("no-volume disk falsely flagged: %s", res.Message)
	}
	// wake must be a clean success.
	evts := c.PowerEvents(0)
	var wakeEvt *model.PowerEvent
	for i := range evts {
		if evts[i].Action == "wake" {
			wakeEvt = &evts[i]
			break
		}
	}
	if wakeEvt == nil || !wakeEvt.OK {
		t.Fatalf("no-volume wake must be a clean success: %+v", wakeEvt)
	}
}

// A disk whose kernel state is offline but which has NO hibernate record
// (offlined by an older build whose state file was lost) must be self-healed
// into the hibernated state on collection, so the Wake button appears.
func TestSelfHealOrphanedOfflineDisk(t *testing.T) {
	resp := baseResp(sataHDDLsblk, "ACTIVE")
	// Kernel says sda is offline; app has no record of it.
	resp["cat /sys/block/sda/device/state"] = "offline\n"
	c, _ := newTestCollector(t, resp)
	disks, err := c.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	var d *model.Disk
	for i := range disks {
		if disks[i].ID == "sda" {
			d = &disks[i]
		}
	}
	if d == nil {
		t.Fatal("sda missing")
	}
	if !d.Hibernated || d.PowerState != "offline" || !d.CanWake {
		t.Fatalf("orphaned offline disk not self-healed: hibernated=%v power=%q canWake=%v",
			d.Hibernated, d.PowerState, d.CanWake)
	}
	// A recovery record must have been persisted so wake works next time.
	rec := c.hibernate.getByDisk(d)
	if rec == nil || !rec.Offline {
		t.Fatalf("self-heal did not persist offline record: %+v", rec)
	}
}

// Waking one disk must NOT run smartctl on OTHER disks (a full Refresh would
// spin up every sleeping drive). Regression for the "wake sdb wakes sda/sdc"
// side effect reported on the real NAS.
func TestWakeDoesNotDisturbOtherDisks(t *testing.T) {
	twoDisks := `{"blockdevices":[
	 {"name":"sdb","path":"/dev/sdb","type":"disk","tran":"sata","size":"500107862016","model":"ST500","serial":"ST-9","rota":true,
	  "children":[{"name":"sdb1","path":"/dev/sdb1","type":"part","size":"500096991232","fstype":"ext4","uuid":"uu-sdb"}]},
	 {"name":"sda","path":"/dev/sda","type":"disk","tran":"sata","size":"2000398934016","model":"WDC WD40","serial":"WD-WX11","rota":true,
	  "children":[{"name":"sda1","path":"/dev/sda1","type":"part","size":"2000396746752","fstype":"ext4","uuid":"uu-sda","mountpoint":"/vol9","mountpoints":["/vol9"]}]}
	]}`
	resp := baseResp(twoDisks, "ACTIVE")
	resp["smartctl -a -j -d sat /dev/sdb"] = `{"device":{"name":"/dev/sdb"},"model_name":"ST500","serial_number":"ST-9","rotation_rate":7200,"smart_status":{"passed":true},"ata_smart_attributes":{"revision":16,"table":[]}}`
	c, r := newTestCollector(t, resp)
	if _, err := c.Offline(context.Background(), "sdb", false, false); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	countSmart := func(dev string) int {
		n := 0
		for _, call := range r.calls {
			if strings.Contains(call, "smartctl -a -j") && strings.Contains(call, dev) {
				n++
			}
		}
		return n
	}
	sdaBefore := countSmart("sda")
	oldStat := statDevice
	statDevice = func(string) (os.FileInfo, error) { return nil, nil }
	defer func() { statDevice = oldStat }()
	if _, err := c.Wake(context.Background(), "sdb"); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	if countSmart("sdb") == 0 {
		t.Errorf("woken disk sdb was not refreshed")
	}
	if countSmart("sda") != sdaBefore {
		t.Errorf("wake disturbed other disk sda: smartctl -a calls %d -> %d", sdaBefore, countSmart("sda"))
	}
}

// diskIOActive must detect growing sector counters.
func TestDiskIOSectors(t *testing.T) {
	stats := `
   8       0 sda 100 0 2000 50 0 300 4000 0 0 0 0
   8      16 sdb 5 0 100 10 0 20 200 0 0 0 0
`
	if v := diskIOSectors(stats, "sda"); v != 2000+4000 {
		t.Errorf("sda=%d want 6000", v)
	}
	if v := diskIOSectors(stats, "sdb"); v != 100+200 {
		t.Errorf("sdb=%d want 300", v)
	}
	if v := diskIOSectors(stats, "sdc"); v != -1 {
		t.Errorf("sdc=%d want -1", v)
	}
}

// Offline must refuse when the disk shows active IO, unless confirmIO is set.
func TestOfflineRefusesBusyDisk(t *testing.T) {
	resp := baseResp(sataHDDLsblk, "ACTIVE")
	// Two diskstats samples: second has higher counters for sda.
	resp["cat /proc/diskstats"] = "" // placeholder; overridden below via custom runner
	c, _ := newTestCollector(t, resp)
	// custom runner that returns increasing diskstats on each call.
	ic := &ioRunner{resp: resp}
	c.run = ic
	old := ioProbeInterval
	ioProbeInterval = time.Millisecond
	defer func() { ioProbeInterval = old }()
	_, err := c.Offline(context.Background(), "sda", false, false)
	if !errors.Is(err, ErrDiskBusy) {
		t.Fatalf("want ErrDiskBusy, got %v", err)
	}
	// RAID acknowledgement alone must not bypass the active-IO guard.
	if _, err := c.Offline(context.Background(), "sda", true, false); !errors.Is(err, ErrDiskBusy) {
		t.Fatalf("confirmRaid without confirmBusyIO must still refuse, got %v", err)
	}
	// An explicit IO acknowledgement is required to proceed.
	if _, err := c.Offline(context.Background(), "sda", true, true); err != nil {
		t.Fatalf("Offline with confirmIO: %v", err)
	}
}

// ioRunner returns growing diskstats counters on successive reads.
type ioRunner struct {
	resp map[string]string
	n    int
}

func (m *ioRunner) Run(ctx context.Context, argv ...string) (string, string, error) {
	key := strings.Join(argv, " ")
	if key == "cat /proc/diskstats" {
		m.n++
		return diskstatsFor("sda", int64(1000+m.n)), "", nil
	}
	r := &scriptRunner{resp: m.resp}
	return r.Run(ctx, argv...)
}

func diskstatsFor(id string, base int64) string {
	return "   8       0 " + id + " 10 0 " +
		strconv.FormatInt(base, 10) + " 5 0 " +
		strconv.FormatInt(base, 10) + " 8 0 0 0 0\n"
}

// A system reboot revives all drives (kernel offline flag cleared), but our
// hibernate record may still say offline. Collection must reconcile: a record
// marked offline whose kernel state is running again must be cleared, so the
// panel shows the disk online instead of a stale "已下线" + 唤醒 button.
func TestReconcileStaleOfflineAfterReboot(t *testing.T) {
	resp := baseResp(sataHDDLsblk, "ACTIVE")
	c, _ := newTestCollector(t, resp)
	if _, err := c.Offline(context.Background(), "sda", false, false); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	// Sanity: flagged offline now.
	if rec := c.hibernate.get("sn:WD-WX11"); rec == nil || !rec.Offline {
		t.Fatalf("not offline after Offline(): %+v", rec)
	}
	// Simulate reboot: kernel now reports running again.
	resp["cat /sys/block/sda/device/state"] = "running\n"
	c.run = &scriptRunner{resp: resp}
	disks, err := c.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	var d *model.Disk
	for i := range disks {
		if disks[i].ID == "sda" {
			d = &disks[i]
		}
	}
	if d == nil {
		t.Fatal("sda missing")
	}
	if d.Hibernated || d.PowerState == "offline" || d.CanWake {
		t.Fatalf("stale offline not reconciled after reboot: hibernated=%v power=%q canWake=%v",
			d.Hibernated, d.PowerState, d.CanWake)
	}
	// Record's offline flag must be cleared too.
	if rec := c.hibernate.get("sn:WD-WX11"); rec != nil && rec.Offline {
		t.Fatalf("offline flag not cleared in store: %+v", rec)
	}
}
