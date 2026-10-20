package collector

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"smartdisk/internal/model"
)

type failCommandRunner struct {
	inner  Runner
	prefix string
	err    error
}

func (r *failCommandRunner) Run(ctx context.Context, argv ...string) (string, string, error) {
	if strings.HasPrefix(strings.Join(argv, " "), r.prefix) {
		return "", "injected failure", r.err
	}
	return r.inner.Run(ctx, argv...)
}

func TestSMARTZeroRPMDoesNotDowngradeRotationalHDD(t *testing.T) {
	resp := baseResp(sataHDDLsblk, "ACTIVE")
	resp["smartctl -a -j -d sat /dev/sda"] = `{"device":{"name":"/dev/sda"},"model_name":"USB HDD",
	"serial_number":"WD-WX11","rotation_rate":0,"smart_support":{"available":true,"enabled":true},
	"smart_status":{"passed":true},"ata_smart_attributes":{"revision":16,"table":[]}}`
	c, _ := newTestCollector(t, resp)
	disks, err := c.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("disks=%d want 1", len(disks))
	}
	d := disks[0]
	if !d.Rotational || d.Type != model.TypeHDD || !d.CanOffline {
		t.Fatalf("zero-RPM SMART downgraded HDD: rotational=%v type=%v canOffline=%v", d.Rotational, d.Type, d.CanOffline)
	}
}

func TestWholeDiskFilesystemIncludedAndUnmounted(t *testing.T) {
	const lsblk = `{"blockdevices":[{"name":"sda","path":"/dev/sda","type":"disk","tran":"sata",
	"size":"1000000000000","model":"WDC WD10","serial":"WD-WX10","rota":true,
	"fstype":"ext4","uuid":"whole-uuid","mountpoint":"/vol10","mountpoints":["/vol10"]}]}`
	resp := baseResp(lsblk, "ACTIVE")
	c, r := newTestCollector(t, resp)
	disks, err := c.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(disks) != 1 || len(disks[0].VolumePaths) != 1 || disks[0].VolumePaths[0] != "/vol10" {
		t.Fatalf("whole-disk mount missing from volume paths: %+v", disks)
	}
	if len(disks[0].Chain) != 1 || disks[0].Chain[0].Depth != 1 || disks[0].Chain[0].MountPoint != "/vol10" {
		t.Fatalf("whole-disk mount missing from chain: %+v", disks[0].Chain)
	}
	if _, err := c.Offline(context.Background(), "sda", false, false); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	found := false
	for _, call := range r.calls {
		if call == "umount -l /vol10" {
			found = true
		}
	}
	if !found {
		t.Fatalf("whole-disk filesystem was not unmounted: %v", r.calls)
	}
}

func TestOfflineSyncFailureAbortsAndClearsPending(t *testing.T) {
	c, r := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	fail := &failCommandRunner{inner: r, prefix: "sync", err: errors.New("sync failed")}
	c.run = fail
	if _, err := c.Offline(context.Background(), "sda", false, false); err == nil {
		t.Fatal("Offline must fail when sync fails")
	}
	for _, call := range r.calls {
		if strings.HasPrefix(call, "umount") || strings.HasPrefix(call, "sh -c echo offline") {
			t.Fatalf("destructive command ran after sync failure: %q", call)
		}
	}
	rec := c.hibernate.get("sn:WD-WX11")
	if rec == nil || rec.PendingOffline || rec.Offline {
		t.Fatalf("pending state not cleared after sync failure: %+v", rec)
	}
}

func TestOfflinePersistenceFailureAbortsBeforeHardwareMutation(t *testing.T) {
	c, r := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c.hibernate.path = filepath.Join(blocker, "hibernate.json")
	if _, err := c.Offline(context.Background(), "sda", false, false); err == nil {
		t.Fatal("Offline must fail when recovery state cannot be persisted")
	}
	for _, call := range r.calls {
		if strings.HasPrefix(call, "sync") || strings.HasPrefix(call, "umount") || strings.HasPrefix(call, "sh -c echo offline") {
			t.Fatalf("hardware mutation ran without durable recovery state: %q", call)
		}
	}
}

func TestOfflineRollbackUsesUUID(t *testing.T) {
	c, r := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	fail := &failCommandRunner{inner: r, prefix: "sh -c echo offline", err: errors.New("sysfs failed")}
	c.run = fail
	if _, err := c.Offline(context.Background(), "sda", false, false); err == nil {
		t.Fatal("Offline must fail when sysfs write fails")
	}
	found := false
	for _, call := range r.calls {
		if call == "mount UUID=uu-11 /vol9" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rollback did not use UUID mount: %v", r.calls)
	}
	rec := c.hibernate.get("sn:WD-WX11")
	if rec == nil || rec.PendingOffline || rec.Offline {
		t.Fatalf("failed offline left pending state: %+v", rec)
	}
}

func TestWakeWarnsWhenAnyOriginalVolumeMissing(t *testing.T) {
	resp := baseResp(mixedHDDLsblk, "ACTIVE")
	resp["smartctl -a -j -d sat /dev/sdd"] = `{"device":{"name":"/dev/sdd"},"model_name":"WDC WD40",
	"serial_number":"WD-WX33","rotation_rate":5400,"smart_support":{"available":true,"enabled":true},
	"smart_status":{"passed":true},"ata_smart_attributes":{"revision":16,"table":[]}}`
	c, _ := newTestCollector(t, resp)
	disks, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var depths []int
	for _, hop := range disks[0].Chain {
		depths = append(depths, hop.Depth)
	}
	if len(depths) != 3 || depths[0] != 1 || depths[1] != 2 || depths[2] != 1 {
		t.Fatalf("branch depths=%v want [1 2 1]", depths)
	}
	if _, err := c.Offline(context.Background(), "sdd", true, true); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	oldStat := statDevice
	statDevice = func(string) (os.FileInfo, error) { return nil, nil }
	defer func() { statDevice = oldStat }()
	const directOnly = `{"blockdevices":[{"name":"sdd","path":"/dev/sdd","type":"disk","tran":"sata",
	"size":"2000398934016","model":"WDC WD40","serial":"WD-WX33","rota":true,
	"children":[{"name":"sdd2","path":"/dev/sdd2","type":"part","size":"1000198840320",
	"fstype":"ext4","uuid":"ext4-uu","mountpoint":"/vol8","mountpoints":["/vol8"]}]}]}`
	c.run = &scriptRunner{resp: baseResp(directOnly, "ACTIVE")}
	res, err := c.Wake(context.Background(), "sdd")
	if err != nil {
		t.Fatalf("Wake: %v", err)
	}
	if !strings.Contains(res.Message, "/vol3") || !strings.Contains(res.Message, "暂未回挂") {
		t.Fatalf("missing RAID volume not reported: %s", res.Message)
	}
	evts := c.PowerEvents(10)
	var wake *model.PowerEvent
	for i := range evts {
		if evts[i].Action == "wake" {
			wake = &evts[i]
			break
		}
	}
	if wake == nil || wake.OK {
		t.Fatalf("degraded wake logged as success: %+v", wake)
	}
}

func TestPendingOfflineIsReconciledAndCleared(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "hibernate.json")
	records := []*HibernateRecord{{
		ID: "sda", Name: "/dev/sda", Model: "WDC WD40", Serial: "WD-WX11",
		PendingOffline: true, OfflineAt: 0,
		VolumePaths: []string{"/vol9"},
		Mounts:      []MountRef{{UUID: "uu-11", MountPoint: "/vol9"}},
	}}
	b, _ := json.Marshal(records)
	if err := os.WriteFile(storePath, b, 0o600); err != nil {
		t.Fatal(err)
	}
	c, r := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	c.hibernate = loadHibernateStore(storePath)
	if _, err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	found := false
	for _, call := range r.calls {
		if call == "mount UUID=uu-11 /vol9" {
			found = true
		}
	}
	if !found {
		t.Fatalf("pending offline mounts were not restored: %v", r.calls)
	}
	rec := c.hibernate.get("sn:WD-WX11")
	if rec == nil || rec.PendingOffline || rec.Offline {
		t.Fatalf("pending state not reconciled: %+v", rec)
	}
}

func TestBtrfsUsesDeviceForMatchingMount(t *testing.T) {
	d := &model.Disk{ID: "sdd", Chain: []model.ChainHop{
		{Name: "sdd1", MountPoint: "/vol3", Depth: 1},
		{Name: "md0", MountPoint: "/vol3", Depth: 2},
		{Name: "sdd2", MountPoint: "/vol8", Depth: 1},
	}}
	if got := chainDeviceForMount(d, "/vol3"); got != "/dev/md0" {
		t.Fatalf("chainDeviceForMount(/vol3)=%q want /dev/md0", got)
	}
	if got := chainDeviceForMount(d, "/vol8"); got != "/dev/sdd2" {
		t.Fatalf("chainDeviceForMount(/vol8)=%q want /dev/sdd2", got)
	}
}

func TestNotePersistenceFailureIsReturned(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("List: %v", err)
	}
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c.hibernate.path = filepath.Join(blocker, "hibernate.json")
	if _, err := c.SetNote(context.Background(), "sda", "冷备盘"); err == nil {
		t.Fatal("SetNote must report persistence failure")
	}
	if rec := c.hibernate.get("sn:WD-WX11"); rec == nil || rec.Note != "" {
		t.Fatalf("failed note write mutated memory: %+v", rec)
	}
}
