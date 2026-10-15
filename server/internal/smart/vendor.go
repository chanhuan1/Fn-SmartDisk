package smart

import (
	"regexp"
	"strconv"
	"strings"

	"smartdisk/internal/model"
)

// Vendor / controller identification and per-vendor SMART attribute decoding,
// adapted from community knowledge (fnsmartdisk). Attribute names use the
// vendor's own semantics; values marked as estimates are labelled, never
// fabricated.

// VendorKind identifies the SSD controller vendor.
type VendorKind string

const (
	VendorNone     VendorKind = ""
	VendorSamsung  VendorKind = "samsung"
	VendorIntel    VendorKind = "intel"
	VendorMicron   VendorKind = "micron"
	VendorWD       VendorKind = "wd"
	VendorPhison   VendorKind = "phison"
	VendorJMicron  VendorKind = "jmicron"
	VendorSMI      VendorKind = "smi"
	VendorHynix    VendorKind = "hynix"   // SK Hynix
	VendorToshiba  VendorKind = "toshiba" // Toshiba / Kioxia
	VendorSanDisk  VendorKind = "sandisk"
	VendorKingston VendorKind = "kingston"
	VendorLiteOn   VendorKind = "liteon"
	VendorADATA    VendorKind = "adata"
	VendorPlextor  VendorKind = "plextor"
	VendorSeagate  VendorKind = "seagate"
	VendorHP       VendorKind = "hp"
	VendorPatriot  VendorKind = "patriot"
	VendorLexar    VendorKind = "lexar"
)

// vendorKeys maps model-name tokens to controller vendors. Order matters.
var vendorKeys = []struct {
	v VendorKind
	t string
}{
	{VendorSamsung, "SAMSUNG"},
	{VendorIntel, "INTEL"},
	{VendorMicron, "MICRON"},
	{VendorMicron, "CRUCIAL"},
	{VendorHynix, "HYNIX"},
	{VendorHynix, "SK HYNIX"},
	{VendorHynix, "HFS"},
	{VendorHynix, "SH910"},
	{VendorHynix, "SC300"},
	{VendorHynix, "SC311"},
	{VendorHynix, "PC300"},
	{VendorHynix, "PC400"},
	{VendorToshiba, "TOSHIBA"},
	{VendorToshiba, "KIOXIA"},
	{VendorToshiba, "THNSN"},
	{VendorToshiba, "KXG"},
	{VendorSanDisk, "SANDISK"},
	{VendorSanDisk, "SDSSDH"},
	{VendorSanDisk, "SDSSDA"},
	{VendorKingston, "KINGSTON"},
	{VendorKingston, "SV300"},
	{VendorKingston, "SA400"},
	{VendorKingston, "SKC"},
	{VendorKingston, "KC600"},
	{VendorKingston, "A400"},
	{VendorLiteOn, "LITE-ON"},
	{VendorLiteOn, "LITEON"},
	{VendorLiteOn, "L9H"},
	{VendorADATA, "ADATA"},
	{VendorADATA, "SU650"},
	{VendorADATA, "SU800"},
	{VendorADATA, "SX6000"},
	{VendorPlextor, "PLEXTOR"},
	{VendorSeagate, "SEAGATE"},
	{VendorSeagate, "ST500"},
	{VendorSeagate, "ST1000"},
	{VendorSeagate, "ST2000"},
	{VendorSeagate, "ST3000"},
	{VendorSeagate, "ST4000"},
	{VendorSeagate, "ST8000"},
	{VendorHP, "HP "},
	{VendorHP, "HP_"},
	{VendorPatriot, "PATRIOT"},
	{VendorPatriot, "PSA100"},
	{VendorLexar, "LEXAR"},
	{VendorPhison, "PHISON"},
	{VendorWD, "WDC"},
	{VendorWD, "WD "},
	{VendorJMicron, "JMF"},
	{VendorJMicron, "JMICRON"},
	{VendorSMI, "SILICONMOTION"},
}

// brandCtrl maps rebadged brands to their actual controller (only entries with
// stable evidence, e.g. Lenovo SL700 is a Phison S11).
var brandCtrl = []struct {
	token string
	v     VendorKind
}{
	{"LENOVO SSD SL700", VendorPhison},
	// Fanxiang (梵想) consumer SATA SSDs use Phison controllers (S11/S13-class).
	{"FANXIANG", VendorPhison},
	{"S100PRO", VendorPhison},
	{"S100 PRO", VendorPhison},
	{"S100", VendorPhison},
}

var smiModelRe = regexp.MustCompile(`SM(22\d\d|22\d|2246|816|2258|2259)`)
var crucialModelRe = regexp.MustCompile(`^\s*CT\d`)

// vendorOf identifies the SSD controller vendor from the model string.
func vendorOf(model string) VendorKind {
	m := strings.ToUpper(model)
	for _, k := range vendorKeys {
		if strings.Contains(m, k.t) {
			return k.v
		}
	}
	for _, b := range brandCtrl {
		if strings.Contains(m, b.token) {
			return b.v
		}
	}
	if crucialModelRe.MatchString(m) {
		return VendorMicron
	}
	if smiModelRe.MatchString(m) {
		return VendorSMI
	}
	return VendorNone
}

// vendorAttrNames maps controller vendor -> attribute ID -> Chinese name.
// Used to refine the generic attribute names with vendor-specific semantics.
var vendorAttrNames = map[VendorKind]map[int]string{
	VendorIntel: {
		170: "可用保留空间", 171: "编程失败计数", 172: "擦除失败计数",
		174: "意外掉电计数", 180: "备用块计数", 184: "端到端错误计数",
		233: "介质磨损指示器",
	},
	VendorSamsung: {
		177: "磨损均衡计数", 179: "已用保留块计数(总)", 180: "未用保留块计数(总)",
		181: "编程失败计数(总)", 182: "擦除失败计数(总)", 183: "运行时坏块计数",
		184: "端到端错误计数", 187: "不可纠正错误计数", 190: "气流温度",
		195: "ECC错误率", 199: "CRC错误计数", 202: "SSD模式状态",
		235: "上电恢复计数", 241: "总LBA写入", 242: "总LBA读取",
		250: "SATA接口降速计数",
	},
	VendorMicron: {
		171: "编程失败计数", 172: "擦除失败计数", 173: "平均块擦除计数",
		174: "意外掉电计数", 180: "未用保留/备用NAND块", 183: "SATA接口降速",
		184: "错误纠正计数", 187: "报告不可纠正错误", 196: "重分配事件计数",
		197: "当前挂起ECC计数", 198: "离线扫描不可纠正错误", 202: "剩余寿命百分比",
		206: "写入错误率", 210: "RAIN成功恢复计数", 246: "累计主机扇区写入",
		247: "主机写入NAND页数", 248: "FTL写入NAND页数",
	},
	VendorWD: {
		230: "介质磨损指示器", // WD Blue/Green SSD
	},
	VendorPhison: {
		1: "ECC错误次数", 167: "写保护模式", 168: "SATA PHY错误计数", 169: "坏块率",
		170: "早期/晚期坏块计数(总)", 172: "擦除失败计数", 173: "最大/平均擦除计数",
		181: "编程失败计数", 182: "擦除失败计数", 192: "非安全关机计数",
		193: "掉电未完成计数", 195: "掉电健康度", 199: "SATA CRC错误",
		207: "温控限速次数", 218: "CRC错误计数", 231: "SSD剩余寿命",
		232: "读取失败计数", 233: "闪存写入量(GiB)", 241: "终身写入量(GB)",
		242: "终身读取量(GB)", 244: "平均擦除计数", 245: "最大擦除计数", 246: "总擦除计数",
	},
	VendorSMI: {
		148: "SLC总擦除计数", 149: "SLC最大擦除计数", 150: "SLC最小擦除计数",
		151: "SLC平均擦除计数", 159: "DRAM 1位错误计数", 160: "不可纠正错误计数",
		161: "有效备用块计数", 162: "缓存块计数", 163: "初始坏块计数",
		164: "总擦除计数", 165: "最大擦除计数", 166: "最小擦除计数",
		167: "平均擦除计数", 168: "规格最大擦除计数", 169: "剩余寿命百分比",
		170: "保留块计数", 178: "运行时无效块计数", 231: "SSD剩余寿命",
		241: "主机写入(32MiB)", 242: "主机读取(32MiB)", 245: "TLC写入(32MiB)",
		246: "SLC写入(32MiB)", 247: "RAID重建计数",
	},
	VendorJMicron: {
		168: "SATA PHY错误计数", 170: "坏块计数", 173: "擦除计数",
		175: "坏簇表计数", 192: "意外掉电计数",
	},
}

// samsungTBW maps Samsung SATA series -> capacity(GB) -> official TBW(GB).
// Source: Samsung official spec sheets (860 PRO datasheet Rev.3 etc).
// Only models with a stable official TBW are listed; others are not estimated.
var samsungTBW = map[string]map[int]int64{
	"870 EVO": {250: 150 * 1024, 500: 300 * 1024, 1024: 600 * 1024, 2048: 1200 * 1024, 4096: 2400 * 1024},
	"870 QVO": {1024: 360 * 1024, 2048: 720 * 1024, 4096: 1440 * 1024, 8192: 2880 * 1024},
	"860 EVO": {250: 150 * 1024, 500: 300 * 1024, 1024: 600 * 1024, 2048: 1200 * 1024, 4096: 2400 * 1024},
	"860 PRO": {256: 300 * 1024, 512: 600 * 1024, 1024: 1200 * 1024, 2048: 2400 * 1024, 4096: 4800 * 1024},
	"860 QVO": {1024: 360 * 1024, 2048: 720 * 1024, 4096: 1440 * 1024},
	"850 EVO": {250: 75 * 1024, 500: 150 * 1024, 1024: 150 * 1024},
	"850 PRO": {256: 150 * 1024, 512: 300 * 1024, 1024: 300 * 1024, 2048: 450 * 1024},
}

var samsungCapRe = regexp.MustCompile(`(\d+)\s*(TB|GB)`)

// samsungTBWFor returns (series, tbwGB) for a Samsung SATA model, or ("",0).
func samsungTBWFor(model string) (string, int64) {
	m := strings.ToUpper(model)
	var series string
	for _, k := range []string{"870 QVO", "870 EVO", "860 QVO", "860 EVO", "860 PRO", "850 EVO", "850 PRO"} {
		if strings.Contains(m, k) {
			series = k
			break
		}
	}
	if series == "" {
		return "", 0
	}
	mx := samsungCapRe.FindAllStringSubmatch(m, -1)
	if len(mx) == 0 {
		return "", 0
	}
	val, uni := mx[len(mx)-1][1], mx[len(mx)-1][2]
	var capGB int
	if uni == "TB" {
		capGB = atoi(val) * 1024
	} else {
		capGB = atoi(val)
	}
	if tbw, ok := samsungTBW[series][capGB]; ok {
		return series, tbw
	}
	return "", 0
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// deriveVendorLife computes remaining life (%) and a provenance label using
// vendor-specific sources. Returns (lifeRemainPct, source, ok).
// Priority: attr231 (Phison/Intel/generic) -> SMI attr169 -> Samsung TBW.
func deriveVendorLife(d *model.Disk, vendor VendorKind, attrs []model.SmartAttr, logicalSector int64) (int, string, bool) {
	attrVal := func(id int) (int64, bool) {
		for _, a := range attrs {
			if a.ID == id {
				return a.RawValue, true
			}
		}
		return 0, false
	}
	attrName := func(id int) string {
		for _, a := range attrs {
			if a.ID == id {
				return strings.ToLower(a.Name)
			}
		}
		return ""
	}

	// 1. attr231 (SSD life left) — only trust it when the attribute is actually
	// a life attribute. Some Phison-based SSDs (e.g. Fanxiang S100Pro) expose
	// attr231 as an unknown/non-life attribute reading 0, which must NOT be
	// misread as "0% life remaining".
	if v, ok := attrVal(231); ok && v >= 0 && v <= 100 && isLifeAttrName(attrName(231)) {
		return int(v), "attr231 厂商归一化", true
	}
	// 2. SMI attr169 (remaining life).
	if vendor == VendorSMI {
		if v, ok := attrVal(169); ok && v >= 0 && v <= 100 {
			return int(v), "attr169 慧荣剩余寿命", true
		}
	}
	// 3. Samsung SATA: estimate from attr241 (LBA written) vs official TBW.
	if vendor == VendorSamsung {
		if series, tbwGB := samsungTBWFor(d.Model); series != "" && tbwGB > 0 {
			if lbas, ok := attrVal(241); ok && lbas >= 0 {
				sec := logicalSector
				if sec <= 0 {
					sec = 512
				}
				writtenGB := float64(lbas) * float64(sec) / (1024.0 * 1024.0 * 1024.0)
				pct := (1 - writtenGB/float64(tbwGB)) * 100.0
				if pct < 0 {
					pct = 0
				}
				if pct > 100 {
					pct = 100
				}
				src := "attr241×" + itoa64(sec) + "B 已写 " + itoa64(int64(writtenGB)) + "GB / 官方TBW " + itoa64(tbwGB/1024) + "TB(" + series + ")"
				return int(pct + 0.5), src, true
			}
		}
	}
	return -1, "", false
}

// isLifeAttrName reports whether an attribute name indicates a "life left"
// attribute (vs an unrelated attribute that happens to share the ID).
func isLifeAttrName(name string) bool {
	return strings.Contains(name, "life") || strings.Contains(name, "ssd_life") ||
		strings.Contains(name, "remaining") || strings.Contains(name, "wear_level")
}

func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}

// vendorAttrName returns the vendor-specific Chinese name for an attribute ID.
func vendorAttrName(vendor VendorKind, id int) string {
	if vendor == VendorNone {
		return ""
	}
	if m, ok := vendorAttrNames[vendor]; ok {
		if cn, ok2 := m[id]; ok2 {
			return cn
		}
	}
	return ""
}

// controllerLabel maps vendor + model hints to a friendly controller name.
// This is a best-effort identification from the model/firmware strings; when
// we cannot pin a specific controller we return the vendor-level label.
func ControllerLabel(d *model.Disk) string {
	// Only SSDs have a NAND controller worth naming.
	if d.Type != model.TypeSSD && d.Type != model.TypeNVMe {
		return ""
	}
	m := strings.ToUpper(d.Model)
	fw := strings.ToUpper(d.Firmware)

	// NVMe: surface as NVMe controller; many NVMe expose no SATA-style vendor.
	if d.Type == model.TypeNVMe {
		return "NVMe 主控"
	}

	v := vendorOf(d.Model)

	// Phison: try to pin S11/S12/S10 from firmware prefix (SBFM*/SBF*) or model.
	if v == VendorPhison {
		switch {
		case strings.HasPrefix(fw, "SBFM"), strings.HasPrefix(fw, "SBFG"):
			return "群联 Phison S11"
		case strings.Contains(m, "PS3111"), strings.Contains(m, "S11"):
			return "群联 Phison S11"
		case strings.Contains(m, "PS3112"), strings.Contains(m, "S12"):
			return "群联 Phison S12"
		case strings.Contains(m, "PS3110"), strings.Contains(m, "S10"):
			return "群联 Phison S10"
		default:
			return "群联 Phison"
		}
	}

	// SMI (Silicon Motion): SM2258/2259/2246/2262 etc.
	if v == VendorSMI {
		for _, c := range []string{"SM2259", "SM2258", "SM2262", "SM2263", "SM2246"} {
			if strings.Contains(m, c) {
				return "慧荣 " + c
			}
		}
		return "慧荣 Silicon Motion"
	}

	// Samsung SATA: MJX/MGX controller codenames appear in model strings rarely;
	// fall back to 三星主控.
	if v == VendorSamsung {
		return "三星主控"
	}

	if v == VendorMicron {
		return "美光/英睿达主控"
	}
	if v == VendorIntel {
		return "Intel 主控"
	}
	if v == VendorWD {
		return "西数主控"
	}
	if v == VendorJMicron {
		return "智微 JMicron"
	}

	// --- additional vendors ---
	if v == VendorHynix {
		// SK Hynix uses in-house controllers; pin common SATA lines when known.
		for _, c := range []struct{ tok, name string }{
			{"SC311", "海力士 SC311 自研主控"},
			{"SC300", "海力士 SC300 自研主控"},
			{"SH910", "海力士 SH910 自研主控"},
			{"SL308", "海力士 SL308 自研主控"},
			{"PC300", "海力士 PC300 自研主控"},
			{"PC400", "海力士 PC400 自研主控"},
		} {
			if strings.Contains(m, c.tok) {
				return c.name
			}
		}
		return "海力士 SK Hynix 自研主控"
	}
	if v == VendorToshiba {
		if strings.Contains(m, "KIOXIA") {
			return "铠侠 Kioxia 自研主控"
		}
		return "东芝 Toshiba 自研主控"
	}
	if v == VendorSanDisk {
		return "闪迪 SanDisk 自研主控"
	}
	if v == VendorKingston {
		for _, c := range []struct{ tok, name string }{
			{"A400", "金士顿 A400（群联 S11 方案）"},
			{"SA400", "金士顿 A400（群联 S11 方案）"},
			{"KC600", "金士顿 KC600（慧荣方案）"},
			{"SV300", "金士顿 V300（东芝/群联方案）"},
		} {
			if strings.Contains(m, c.tok) {
				return c.name
			}
		}
		return "金士顿 Kingston"
	}
	if v == VendorLiteOn {
		return "建兴 Lite-On 主控"
	}
	if v == VendorADATA {
		return "威刚 ADATA"
	}
	if v == VendorPlextor {
		return "浦科特 Plextor"
	}
	if v == VendorSeagate {
		return "希捷 Seagate"
	}
	if v == VendorHP {
		return "惠普 HP（多为群联/慧荣方案）"
	}
	if v == VendorPatriot {
		return "爱国者 Patriot（多为群联方案）"
	}
	if v == VendorLexar {
		return "雷克沙 Lexar"
	}

	return ""
}
