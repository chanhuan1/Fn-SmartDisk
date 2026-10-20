package collector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"smartdisk/internal/model"
)

func TestPowerLogRecordsSleepOffline(t *testing.T) {
	c, _ := newTestCollector(t, baseResp(sataHDDLsblk, "ACTIVE"))
	if _, err := c.Sleep(context.Background(), "sda"); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if _, err := c.Offline(context.Background(), "sda", false, false); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	evts := c.PowerEvents(0)
	// Only summary events are logged (one per op); per-step lines are
	// streamed live to the modal but NOT persisted.
	if len(evts) != 2 {
		t.Fatalf("want 2 summary events, got %d: %+v", len(evts), evts)
	}
	// newest first: offline summary, then sleep.
	if evts[0].Action != "offline" || !evts[0].OK || evts[0].DiskID != "sda" {
		t.Fatalf("bad offline event: %+v", evts[0])
	}
	if evts[len(evts)-1].Action != "sleep" || !evts[len(evts)-1].OK {
		t.Fatalf("bad sleep event: %+v", evts[1])
	}
	if evts[0].Model != "WDC WD40" {
		t.Fatalf("model not recorded: %+v", evts[0])
	}
}

func TestPowerLogFailureRecorded(t *testing.T) {
	resp := baseResp(sataHDDLsblk, "ACTIVE")
	// hdparm fails (no fixture).
	delete(resp, "hdparm -y")
	c, _ := newTestCollector(t, resp)
	if _, err := c.Sleep(context.Background(), "sda"); err == nil {
		t.Fatal("expected sleep to fail without hdparm fixture")
	}
	evts := c.PowerEvents(0)
	if len(evts) != 1 || evts[0].OK || evts[0].Action != "sleep" {
		t.Fatalf("failure not recorded: %+v", evts)
	}
}

func TestPowerLogRingCap(t *testing.T) {
	dir := t.TempDir()
	s := loadPowerLogStore(filepath.Join(dir, "log.json"))
	for i := 0; i < powerLogMax+50; i++ {
		s.add(model.PowerEvent{DiskID: "sda", Action: "sleep", OK: true})
	}
	if got := len(s.list(0)); got != powerLogMax {
		t.Fatalf("ring cap wrong: %d", got)
	}
	// limit respected.
	if got := len(s.list(10)); got != 10 {
		t.Fatalf("limit wrong: %d", got)
	}
	// persistence: reload and count matches.
	s2 := loadPowerLogStore(filepath.Join(dir, "log.json"))
	if got := len(s2.list(0)); got != powerLogMax {
		t.Fatalf("persistence wrong: %d", got)
	}
}

func TestPowerLogPersistenceFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &powerLogStore{path: filepath.Join(blocker, "events.json"), evt: []model.PowerEvent{{DiskID: "sda", Action: "offline"}}}
	before := len(s.list(0))
	if err := s.clear(); err == nil {
		t.Fatal("clear must fail when persistence fails")
	}
	if got := len(s.list(0)); got != before {
		t.Fatalf("failed clear lost in-memory events: got %d want %d", got, before)
	}
}

// Old builds could write duplicate records for one physical drive (one keyed
// by kernel name, one by serial). loadHibernateStore must merge them, with
// offline=true winning, so a stale online duplicate can't hide the offline
// state (which is exactly what hid the Wake button on the real NAS).
func TestLoadMergesDuplicateRecords(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hibernate.json")
	dupes := `[
	  {"id":"sdb","serial":"Z3TDZMRL","offline":false,"updatedAt":100},
	  {"id":"sdb","serial":"Z3TDZMRL","offline":true,"offlineAt":90,"updatedAt":95}
	]`
	if err := os.WriteFile(p, []byte(dupes), 0o600); err != nil {
		t.Fatal(err)
	}
	s := loadHibernateStore(p)
	rec := s.get("sn:Z3TDZMRL")
	if rec == nil {
		t.Fatal("record not merged under serial key")
	}
	if !rec.Offline {
		t.Fatalf("offline=true must win after merge: %+v", rec)
	}
	if rec.OfflineAt != 90 {
		t.Fatalf("offlineAt lost: %+v", rec)
	}
	// only ONE record for the drive
	n := 0
	for k := range s.recs {
		if k == "sn:Z3TDZMRL" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("duplicate records not deduped: %d keys", n)
	}
}
