package smart

import (
	"testing"

	"smartdisk/internal/model"
)

func parse(t *testing.T, payload string) model.Disk {
	t.Helper()
	var d model.Disk
	d.PercentUsed, d.PercentLifeRemain = -1, -1
	if err := ParseSmartctl([]byte(payload), &d); err != nil {
		t.Fatalf("ParseSmartctl: %v", err)
	}
	return d
}

func TestParseSATAHDD(t *testing.T) {
	d := parse(t, sampleSATAHDD)
	if d.Model != "WDC WD40EFRX-68N32N0" {
		t.Errorf("model=%q", d.Model)
	}
	if d.Type != model.TypeHDD {
		t.Errorf("type=%v want HDD", d.Type)
	}
	if d.Capacity != 4000787030016 {
		t.Errorf("capacity=%d", d.Capacity)
	}
	if d.Temperature != 38 {
		t.Errorf("temp=%d", d.Temperature)
	}
	if d.PowerOnHours != 21340 {
		t.Errorf("poh=%d", d.PowerOnHours)
	}
	if len(d.Attrs) != 7 {
		t.Errorf("attrs=%d", len(d.Attrs))
	}
	if !d.SmartOK {
		t.Errorf("expected SmartOK")
	}
}

func TestParseSATASSDLife(t *testing.T) {
	d := parse(t, sampleSATASSD)
	if d.Type != model.TypeSSD {
		t.Errorf("type=%v want SSD", d.Type)
	}
	want := int64(42949672960) * 512
	if d.TotalBytesWritten != want {
		t.Errorf("tbw=%d want %d", d.TotalBytesWritten, want)
	}
	// Samsung 870 EVO uses vendor TBW estimate (attr241 written vs official TBW),
	// which is more authoritative than Wear_Leveling_Count. 20TB written of 600TB.
	if d.PercentUsed < 0 || d.PercentUsed > 10 {
		t.Errorf("percentUsed=%d want small (TBW estimate)", d.PercentUsed)
	}
	if d.PercentLifeRemain < 90 {
		t.Errorf("lifeRemain=%d want ~97 (TBW estimate)", d.PercentLifeRemain)
	}
	if d.LifeSource == "" {
		t.Errorf("expected LifeSource for TBW estimate")
	}
	if d.Vendor != "samsung" {
		t.Errorf("vendor=%q want samsung", d.Vendor)
	}
}

func TestParseNVMe(t *testing.T) {
	d := parse(t, sampleNVMe)
	if d.Type != model.TypeNVMe {
		t.Errorf("type=%v want NVMe", d.Type)
	}
	if d.PercentUsed != 7 {
		t.Errorf("percentUsed=%d", d.PercentUsed)
	}
	if d.PercentLifeRemain != 93 {
		t.Errorf("lifeRemain=%d", d.PercentLifeRemain)
	}
	if d.UnsafeShutdowns != 12 {
		t.Errorf("unsafe=%d", d.UnsafeShutdowns)
	}
	if d.Temperature != 45 {
		t.Errorf("temp=%d", d.Temperature)
	}
	want := int64(83886080) * 1000 * 512
	if d.TotalBytesWritten != want {
		t.Errorf("tbw=%d want %d", d.TotalBytesWritten, want)
	}
	// Full NVMe health log exposed for the SMART table.
	if d.NVMe == nil {
		t.Fatalf("expected NVMe health log")
	}
	if d.NVMe.AvailableSpare != 100 || d.NVMe.AvailableSpareThresh != 10 {
		t.Errorf("spare=%d/%d", d.NVMe.AvailableSpare, d.NVMe.AvailableSpareThresh)
	}
	if d.NVMe.MediaErrors != 0 || d.NVMe.NumErrLogEntries != 3 {
		t.Errorf("mediaErr=%d errLog=%d", d.NVMe.MediaErrors, d.NVMe.NumErrLogEntries)
	}
	if d.NVMe.DataUnitsWritten != 83886080 {
		t.Errorf("duw=%d", d.NVMe.DataUnitsWritten)
	}
}

func TestHealthGood(t *testing.T) {
	d := parse(t, sampleSATAHDD)
	Grade(&d)
	if d.Health != model.GradeGood {
		t.Errorf("health=%v want 优, notes=%v", d.Health, d.HealthNotes)
	}
}

func TestHealthCritical(t *testing.T) {
	d := parse(t, sampleFailingHDD)
	Grade(&d)
	if d.Health != model.GradeCritical {
		t.Errorf("health=%v want 危, notes=%v", d.Health, d.HealthNotes)
	}
	var reall, pend bool
	for _, a := range d.Attrs {
		if a.ID == 5 && a.Critical {
			reall = true
		}
		if a.ID == 197 && a.Critical {
			pend = true
		}
	}
	if !reall || !pend {
		t.Errorf("critical flags reall=%v pend=%v", reall, pend)
	}
}

func TestHealthUnknownWhenUnavailable(t *testing.T) {
	var d model.Disk
	d.SmartAvail = false
	Grade(&d)
	if d.Health != model.GradeUnknown {
		t.Errorf("health=%v want 未知", d.Health)
	}
}

func TestBridgeDB(t *testing.T) {
	lsusb := ParseLsusb("Bus 002 Device 003: ID 174c:55aa ASMedia Technology Inc. ASM1051E")
	b := BridgeFor("174c", "55aa", lsusb)
	if b == nil || b.Passthrough != "sat" {
		t.Errorf("bridge=%+v", b)
	}
	b2 := BridgeFor("152d", "0578", lsusb)
	if b2 == nil || b2.Chip == "" || b2.Passthrough == "" {
		t.Errorf("jmicron=%+v", b2)
	}
	b3 := BridgeFor("aaaa", "bbbb", lsusb)
	if b3 == nil || b3.Chip == "" {
		t.Errorf("unknown=%+v", b3)
	}
	ord := PassthroughOrder(b2)
	if len(ord) == 0 || ord[0] != b2.Passthrough {
		t.Errorf("order=%v", ord)
	}
}

func TestManufactureDateUnknown(t *testing.T) {
	if got := InferManufactureDate("WDC", "WD-WCC7K3ABCDEF"); got != "未知" {
		t.Errorf("got %q want 未知", got)
	}
	if got := InferManufactureDate("", ""); got != "未知" {
		t.Errorf("got %q want 未知", got)
	}
}

func TestParseGbps(t *testing.T) {
	cases := map[string]float64{
		"6.0 Gb/s": 6.0,
		"3.0 Gb/s": 3.0,
		"1.5 Gb/s": 1.5,
		"16.0GT/s": 0, // no space; handled by leadingFloat in collector
		"":         0,
		"unknown":  0,
	}
	for in, want := range cases {
		if got := parseGbps(in); got != want {
			t.Errorf("parseGbps(%q)=%v want %v", in, got, want)
		}
	}
}

// Regression: identical current/max speed strings must NOT be flagged as
// downgraded even if the underlying enum values differ.
func TestLinkNotDowngradedWhenEqual(t *testing.T) {
	payload := `{
	  "device": {"name":"/dev/sdx","info_name":"/dev/sdx","type":"scsi","protocol":"SCSI"},
	  "model_name":"Test HDD","serial_number":"ABC","user_capacity":{"bytes":1000000000},
	  "rotation_rate":7200,
	  "interface_speed": {
	    "max": {"sata_value": 14, "string": "6.0 Gb/s", "units": "Gb/s"},
	    "current": {"sata_value": 13, "string": "6.0 Gb/s", "units": "Gb/s"}
	  },
	  "smart_support": {"available": true, "enabled": true},
	  "smart_status": {"passed": true}
	}`
	var d model.Disk
	d.PercentUsed, d.PercentLifeRemain = -1, -1
	if err := ParseSmartctl([]byte(payload), &d); err != nil {
		t.Fatal(err)
	}
	if d.Link == nil {
		t.Fatal("expected link")
	}
	if d.Link.Downgraded {
		t.Errorf("6.0/6.0 should NOT be downgraded, got downgraded=true")
	}
}

func TestLinkDowngradedWhenLower(t *testing.T) {
	payload := `{
	  "device": {"name":"/dev/sdy","info_name":"/dev/sdy","type":"scsi","protocol":"SCSI"},
	  "model_name":"Test SSD","serial_number":"DEF","user_capacity":{"bytes":1000000000},
	  "rotation_rate":0,
	  "interface_speed": {
	    "max": {"sata_value": 14, "string": "6.0 Gb/s", "units": "Gb/s"},
	    "current": {"sata_value": 7, "string": "3.0 Gb/s", "units": "Gb/s"}
	  },
	  "smart_support": {"available": true, "enabled": true},
	  "smart_status": {"passed": true}
	}`
	var d model.Disk
	d.PercentUsed, d.PercentLifeRemain = -1, -1
	if err := ParseSmartctl([]byte(payload), &d); err != nil {
		t.Fatal(err)
	}
	if !d.Link.Downgraded {
		t.Errorf("3.0/6.0 SHOULD be downgraded, got false")
	}
}

// rotation_rate == 0 is unreliable (some HDDs / USB bridges report 0). When
// lsblk already told us the device is rotational (d.Rotational=true), SMART
// must NOT downgrade it to SSD. Regression for ST500DM002-style HDDs.
func TestRotationalPreservedWhenSmartReportsZeroRPM(t *testing.T) {
	d := &model.Disk{Rotational: true, Type: model.TypeHDD}
	if err := ParseSmartctl([]byte(sampleSATASSD), d); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Type != model.TypeHDD || !d.Rotational {
		t.Errorf("lsblk-rotational disk downgraded by SMART rotation_rate=0: type=%v rot=%v", d.Type, d.Rotational)
	}
}
