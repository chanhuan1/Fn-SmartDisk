package smart

import (
	"testing"

	"smartdisk/internal/model"
)

// Fanxiang S100Pro 2TB (Phison S13-class) real smartctl output (trimmed).
// Key facts: attr231 reads 0 but is NOT a life attribute; attr233 is
// Media_Wearout_Indicator; attr241 is a real LBA count (new drive, tiny).
const fanxiangS100Pro = `{
  "device": {"name": "/dev/sdc", "info_name": "/dev/sdc [SAT]", "type": "sat", "protocol": "ATA"},
  "model_name": "Fanxiang S100Pro 2TB",
  "serial_number": "MX_00000000000010108",
  "firmware_version": "SN20527",
  "user_capacity": {"blocks": 4000797360, "bytes": 2048408248320},
  "logical_block_size": 512,
  "physical_block_size": 512,
  "rotation_rate": 0,
  "smart_support": {"available": true, "enabled": true},
  "smart_status": {"passed": true},
  "temperature": {"current": 38},
  "power_on_time": {"hours": 8634},
  "power_cycle_count": 12,
  "ata_smart_attributes": {"revision": 19, "table": [
    {"id": 5, "name": "Reallocated_Sector_Ct", "value": 100, "worst": 100, "thresh": 50, "when_failed": "", "raw": {"value": 0, "string": "0"}},
    {"id": 9, "name": "Power_On_Hours", "value": 100, "worst": 100, "thresh": 0, "when_failed": "", "raw": {"value": 8634, "string": "8634"}},
    {"id": 169, "name": "Unknown_Attribute", "value": 100, "worst": 100, "thresh": 10, "when_failed": "", "raw": {"value": 31457332, "string": "31457332"}},
    {"id": 194, "name": "Temperature_Celsius", "value": 38, "worst": 38, "thresh": 0, "when_failed": "", "raw": {"value": 231929806886, "string": "38 (Min/Max 24/54)"}},
    {"id": 231, "name": "Unknown_SSD_Attribute", "value": 100, "worst": 100, "thresh": 5, "when_failed": "", "raw": {"value": 0, "string": "0"}},
    {"id": 233, "name": "Media_Wearout_Indicator", "value": 100, "worst": 100, "thresh": 0, "when_failed": "", "raw": {"value": 11009, "string": "11009"}},
    {"id": 241, "name": "Total_LBAs_Written", "value": 100, "worst": 100, "thresh": 0, "when_failed": "", "raw": {"value": 5009, "string": "5009"}},
    {"id": 242, "name": "Total_LBAs_Read", "value": 100, "worst": 100, "thresh": 0, "when_failed": "", "raw": {"value": 2181, "string": "2181"}}
  ]}
}`

// The drive's attr231=0 must NOT be shown as "0% life remaining". With no
// reliable life-percentage source, life should be reported as unknown (-1).
func TestFanxiangLifeNotZero(t *testing.T) {
	var d model.Disk
	d.PercentUsed, d.PercentLifeRemain = -1, -1
	if err := ParseSmartctl([]byte(fanxiangS100Pro), &d); err != nil {
		t.Fatal(err)
	}
	if vendorOf(d.Model) != VendorPhison {
		t.Errorf("vendorOf(Fanxiang S100Pro)=%v want phison", vendorOf(d.Model))
	}
	if d.PercentLifeRemain == 0 {
		t.Errorf("PercentLifeRemain must not be 0 (the attr231 bug), got 0")
	}
	var a233 *model.SmartAttr
	for i := range d.Attrs {
		if d.Attrs[i].ID == 233 {
			a233 = &d.Attrs[i]
		}
	}
	if a233 == nil {
		t.Fatalf("attr233 missing")
	}
	if a233.Cn != "介质磨损指示" {
		t.Errorf("attr233 cn=%q want 介质磨损指示", a233.Cn)
	}
	var a241 *model.SmartAttr
	for i := range d.Attrs {
		if d.Attrs[i].ID == 241 {
			a241 = &d.Attrs[i]
		}
	}
	if a241 == nil || a241.Cn != "总写入量(LBA)" {
		t.Errorf("attr241 cn=%q want 总写入量(LBA)", a241.Cn)
	}
}
