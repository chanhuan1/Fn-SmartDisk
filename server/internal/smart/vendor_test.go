package smart

import (
	"testing"

	"smartdisk/internal/model"
)

func TestVendorOf(t *testing.T) {
	cases := map[string]VendorKind{
		"Samsung SSD 870 EVO 1TB":   VendorSamsung,
		"SAMSUNG SSD 860 PRO":       VendorSamsung,
		"INTEL SSDSC2BW480A4":       VendorIntel,
		"CT500MX500SSD1":            VendorMicron,
		"Micron_1100_MTFDDAK256TBN": VendorMicron,
		"WDC WDS500G2B0A":           VendorWD,
		"PHISON PS3111 SATA SSD":    VendorPhison,
		"LENOVO SSD SL700 256G":     VendorPhison,
		"SM2258G":                   VendorSMI,
		"SPCC Solid State Disk":     VendorNone,
		"WDC WD40EFRX-68N32N0":      VendorWD,
	}
	for model, want := range cases {
		if got := vendorOf(model); got != want {
			t.Errorf("vendorOf(%q)=%v want %v", model, got, want)
		}
	}
}

func TestSamsungTBWFor(t *testing.T) {
	series, tbw := samsungTBWFor("Samsung SSD 870 EVO 1TB")
	if series != "870 EVO" || tbw != 600*1024 {
		t.Errorf("870 EVO 1TB: series=%q tbw=%d", series, tbw)
	}
	series, tbw = samsungTBWFor("Samsung SSD 860 PRO 512GB")
	if series != "860 PRO" || tbw != 600*1024 {
		t.Errorf("860 PRO 512GB: series=%q tbw=%d", series, tbw)
	}
	if s, tb := samsungTBWFor("Samsung SSD 980 PRO 2TB"); s != "" || tb != 0 {
		t.Errorf("980 PRO should not match SATA table, got %q %d", s, tb)
	}
}

func TestDeriveVendorLifeSamsung(t *testing.T) {
	// 870 EVO 1TB, TBW 600TB(=600*1024 GB). attr241 chosen so written=100GB:
	// written(GB) = lbas * 512 / 1024^3 = 100 => lbas = 209715200
	d := &model.Disk{Model: "Samsung SSD 870 EVO 1TB"}
	attrs := []model.SmartAttr{
		{ID: 241, Name: "Total_LBAs_Written", RawValue: 209715200},
	}
	remain, src, ok := deriveVendorLife(d, VendorSamsung, attrs, 512)
	if !ok {
		t.Fatalf("expected ok")
	}
	if remain < 99 || remain > 100 {
		t.Errorf("remain=%d want ~100", remain)
	}
	if src == "" {
		t.Errorf("expected source label")
	}
	t.Logf("src=%s remain=%d", src, remain)
}

func TestDeriveVendorLifeSMI(t *testing.T) {
	d := &model.Disk{Model: "SM2258G SSD"}
	attrs := []model.SmartAttr{{ID: 169, Name: "Remaining_Life_Perc", RawValue: 87}}
	remain, src, ok := deriveVendorLife(d, VendorSMI, attrs, 512)
	if !ok || remain != 87 {
		t.Errorf("smi remain=%d ok=%v", remain, ok)
	}
	if src != "attr169 慧荣剩余寿命" {
		t.Errorf("src=%q", src)
	}
}

func TestDeriveVendorLifeAttr231(t *testing.T) {
	d := &model.Disk{Model: "PHISON PS3111"}
	attrs := []model.SmartAttr{{ID: 231, Name: "SSD_Life_Left", RawValue: 95}}
	remain, src, ok := deriveVendorLife(d, VendorPhison, attrs, 512)
	if !ok || remain != 95 {
		t.Errorf("phison remain=%d ok=%v", remain, ok)
	}
	if src != "attr231 厂商归一化" {
		t.Errorf("src=%q", src)
	}
}

func TestVendorAttrName(t *testing.T) {
	if got := vendorAttrName(VendorSamsung, 241); got != "总LBA写入" {
		t.Errorf("samsung 241=%q", got)
	}
	if got := vendorAttrName(VendorSMI, 169); got != "剩余寿命百分比" {
		t.Errorf("smi 169=%q", got)
	}
	if got := vendorAttrName(VendorNone, 241); got != "" {
		t.Errorf("none should be empty, got %q", got)
	}
}

func TestVendorOfExtended(t *testing.T) {
	cases := map[string]VendorKind{
		"HFS256G39TND-N210A":        VendorHynix, // SK Hynix SC311
		"SK HYNIX SC300 SATA 256GB": VendorHynix,
		"TOSHIBA THNSNH256GBST":     VendorToshiba,
		"KIOXIA EXCERIA SATA SSD":   VendorToshiba,
		"SanDisk SDSSDH3 500G":      VendorSanDisk,
		"KINGSTON SA400S37480G":     VendorKingston,
		"ADATA SU650":               VendorADATA,
		"LITE-ON L9H-256V2G-HP":     VendorLiteOn,
		"PLEXTOR PX-256M8VC":        VendorPlextor,
		"ST500LM000-SSHD-8GB":       VendorSeagate,
		"Lexar 512GB SSD":           VendorLexar,
		"PATRIOT PSA100-01-SATA":    VendorPatriot,
	}
	for model, want := range cases {
		if got := vendorOf(model); got != want {
			t.Errorf("vendorOf(%q)=%v want %v", model, got, want)
		}
	}
}

func TestControllerLabelExtended(t *testing.T) {
	cases := []struct {
		model, want string
	}{
		{"HFS256G39TND-N210A", "海力士 SK Hynix 自研主控"},
		{"KIOXIA EXCERIA SATA SSD", "铠侠 Kioxia 自研主控"},
		{"TOSHIBA THNSNH256GBST", "东芝 Toshiba 自研主控"},
		{"SanDisk SDSSDH3 500G", "闪迪 SanDisk 自研主控"},
		{"KINGSTON SA400S37480G", "金士顿 A400（群联 S11 方案）"},
		{"ADATA SU650", "威刚 ADATA"},
	}
	for _, c := range cases {
		d := &model.Disk{Model: c.model, Type: model.TypeSSD}
		if got := ControllerLabel(d); got != c.want {
			t.Errorf("ControllerLabel(%q)=%q want %q", c.model, got, c.want)
		}
	}
}
