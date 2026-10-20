package smart

import (
	"testing"

	"smartdisk/internal/model"
)

// Phison attr241 is in GB, not LBAs — a Fanxiang-class Phison SSD writing
// 2048 GB must show ~2 TB, not a few MB.
func TestPhisonWriteUnitIsGB(t *testing.T) {
	attrs := []model.SmartAttr{
		{ID: 241, Name: "Total_LBAs_Written", RawValue: 2048},
	}
	_, _, bw, src := deriveATALife(attrs, VendorPhison)
	want := int64(2048) * 1024 * 1024 * 1024
	if bw != want {
		t.Errorf("phison bytesWritten=%d want %d", bw, want)
	}
	if src == "" {
		t.Error("phison write source empty")
	}
}

// SMI attr241 is in 32MiB units.
func TestSMIWriteUnitIs32MiB(t *testing.T) {
	attrs := []model.SmartAttr{
		{ID: 241, Name: "Host_Writes_32MiB", RawValue: 1000},
	}
	_, _, bw, src := deriveATALife(attrs, VendorSMI)
	want := int64(1000) * 32 * 1024 * 1024
	if bw != want {
		t.Errorf("smi bytesWritten=%d want %d", bw, want)
	}
	if src == "" {
		t.Error("smi write source empty")
	}
}

// Samsung / generic attr241 stays in 512B LBAs.
func TestSamsungWriteUnitIsLBA(t *testing.T) {
	attrs := []model.SmartAttr{
		{ID: 241, Name: "Total_LBAs_Written", RawValue: 209715200},
	}
	_, _, bw, src := deriveATALife(attrs, VendorSamsung)
	want := int64(209715200) * 512
	if bw != want {
		t.Errorf("samsung bytesWritten=%d want %d", bw, want)
	}
	if src == "" {
		t.Error("samsung write source empty")
	}
}
