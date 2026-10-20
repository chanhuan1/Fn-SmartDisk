package smart

import (
	"fmt"
	"strings"
)

// buildWWN renders a canonical WWN/EUI-64 string from smartctl's wwn/eui64
// or NGUID fields, whichever is present. Returns "" when none available.
func buildWWN(j smartctlJSON) string {
	if j.WWN != nil && (j.WWN.NAA != 0 || j.WWN.OUI != 0 || j.WWN.ID != 0) {
		return fmt.Sprintf("%x %06x %012x", j.WWN.NAA, j.WWN.OUI, j.WWN.ID)
	}
	if j.EUI64 != nil && (j.EUI64.OUI != 0 || j.EUI64.ID != 0) {
		return fmt.Sprintf("%06x %012x", j.EUI64.OUI, j.EUI64.ID)
	}
	if j.NGUID != "" {
		return j.NGUID
	}
	return ""
}

// phyEventCn maps common SATA PHY event names to Chinese.
func phyEventCn(name string) string {
	s := strings.ToLower(name)
	switch {
	case strings.Contains(s, "command") && strings.Contains(s, "icrc"):
		return "命令 CRC 错误"
	case strings.Contains(s, "data") && strings.Contains(s, "icrc"):
		return "数据 CRC 错误"
	case strings.Contains(s, "icrc"):
		return "接口 CRC 错误"
	case strings.Contains(s, "r_err"):
		return "接收错误"
	case strings.Contains(s, "crc"):
		return "CRC 错误"
	case strings.Contains(s, "reset"):
		return "链路复位"
	case strings.Contains(s, "phy") && strings.Contains(s, "ready"):
		return "PHY 就绪变化"
	case strings.Contains(s, "unknown"):
		return "未知错误"
	default:
		return name
	}
}

// isUnknownAttrName reports whether the drive-reported attribute name is a
// generic "Unknown_Attribute"/"Unknown_SSD_Attribute" placeholder.
func isUnknownAttrName(name string) bool {
	s := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(s, "unknown")
}

// knownAttrNameCn maps well-known drive-reported attribute names to Chinese,
// used when the drive gives a real (non-"Unknown") name. This takes precedence
// over the ID-based vendor table so controller-specific IDs (like Phison
// attr233) aren't mislabelled.
func knownAttrNameCn(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	switch s {
	case "media_wearout_indicator":
		return "介质磨损指示"
	case "total_lbas_written":
		return "总写入量(LBA)"
	case "total_lbas_read":
		return "总读取量(LBA)"
	case "host_writes_32mib":
		return "主机写入(32MiB)"
	case "wear_leveling_count":
		return "磨损均衡计数"
	case "percent_lifetime_used":
		return "寿命已用百分比"
	case "percent_life_remaining":
		return "剩余寿命百分比"
	case "ssd_life_left":
		return "SSD 剩余寿命"
	case "reallocated_sector_ct":
		return "重映射扇区数"
	case "current_pending_sector":
		return "待重映射扇区数"
	case "offline_uncorrectable":
		return "离线无法校正扇区数"
	case "udma_crc_error_count":
		return "接口 CRC 错误数"
	case "power_on_hours":
		return "通电时间"
	case "power_cycle_count":
		return "通电次数"
	case "temperature_celsius":
		return "温度"
	case "load_cycle_count":
		return "磁头加载/卸载计数"
	case "reported_uncorrect":
		return "报告的不可校正错误"
	case "program_fail_count_chip":
		return "编程失败计数(芯片)"
	case "erase_fail_count_chip":
		return "擦除失败计数(芯片)"
	case "unused_rsvd_blk_cnt_tot":
		return "未用保留块计数(总)"
	case "used_rsvd_blk_cnt_tot":
		return "已用保留块计数(总)"
	case "power-off_retract_count":
		return "断电缩回计数"
	default:
		return ""
	}
}
