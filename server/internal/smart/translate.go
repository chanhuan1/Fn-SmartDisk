package smart

import "strings"

// SelfTestTypeCn translates a self-test type string to Chinese.
func SelfTestTypeCn(en string) string {
	s := strings.ToLower(en)
	switch {
	case strings.Contains(s, "short"):
		return "短自检"
	case strings.Contains(s, "extended"):
		return "扩展自检"
	case strings.Contains(s, "conveyance"):
		return "运输自检"
	case strings.Contains(s, "selective"):
		return "选择自检"
	case strings.Contains(s, "offline"):
		return "离线自检"
	default:
		return en
	}
}

// SelfTestStatusCn translates a self-test status string to Chinese and
// reports whether the test passed.
func SelfTestStatusCn(en string) (cn string, passed bool) {
	s := strings.ToLower(en)
	switch {
	case strings.Contains(s, "without error") || strings.Contains(s, "completed_ok"):
		return "完成（无错误）", true
	case strings.Contains(s, "aborted"):
		return "被主机中止", false
	case strings.Contains(s, "interrupted"):
		return "被中断（复位/断电）", false
	case strings.Contains(s, "fatal") || strings.Contains(s, "failed"):
		return "失败/严重错误", false
	case strings.Contains(s, "read") && strings.Contains(s, "failure"):
		return "读取失败", false
	case strings.Contains(s, "electrical"):
		return "电气错误", false
	case strings.Contains(s, "servo"):
		return "伺服/寻道错误", false
	case strings.Contains(s, "in progress") || strings.Contains(s, "in_progress"):
		return "进行中", false
	case strings.Contains(s, "suspended"):
		return "已暂停", false
	case strings.Contains(s, "completed"):
		return "已完成", true
	default:
		return en, false
	}
}

// devStatCn maps common Device Statistics entry names (lowercase) to Chinese.
var devStatCn = map[string]string{
	"lifetime power-on resets":                "通电复位次数",
	"power-on hours":                          "通电小时数",
	"logical sectors written":                 "写入扇区数",
	"logical sectors read":                    "读取扇区数",
	"number of write commands":                "写命令数",
	"number of read commands":                 "读命令数",
	"number of reported uncorrectable errors": "报告的不可校正错误",
	"number of recovered read errors":         "已恢复读取错误",
	"number of hardware resets":               "硬件复位次数",
	"number of spare blocks remaining":        "剩余备用块",
	"percentage used endurance indicator":     "耐久度已用百分比",
	"current temperature":                     "当前温度",
	"highest temperature":                     "历史最高温度",
	"lowest temperature":                      "历史最低温度",
	"specified maximum operating temperature": "额定最高工作温度",
	"specified minimum operating temperature": "额定最低工作温度",
	"number of grown defective blocks":        "新增坏块数",
	"number of erase cycles":                  "擦除次数",
	"number of interface crc errors":          "接口 CRC 错误",
	"number of power-off retract count":       "断电缩回计数",
	"number of load/unload cycles":            "加载/卸载循环",
	"nand writes":                             "NAND 写入量",
	"host writes":                             "主机写入量",
	"available spare space":                   "可用备用空间",
	"wear leveling count":                     "磨损均衡计数",
	"used rsvd blk cnt tot":                   "已用保留块总数",
	"unused rsvd blk cnt tot":                 "未用保留块总数",
}

// DevStatCn translates a Device Statistics page/entry name to Chinese. The
// input may be "Page · Entry"; each part is translated when known.
func DevStatCn(name string) string {
	parts := strings.Split(name, " · ")
	for i, p := range parts {
		if cn, ok := devStatCn[strings.ToLower(strings.TrimSpace(p))]; ok {
			parts[i] = cn
		} else {
			parts[i] = devStatPageCn(p)
		}
	}
	return strings.Join(parts, " · ")
}

// devStatPageCn translates known Device Statistics page names.
func devStatPageCn(p string) string {
	s := strings.ToLower(strings.TrimSpace(p))
	switch s {
	case "general statistics":
		return "常规统计"
	case "general errors statistics":
		return "常规错误统计"
	case "error statistics":
		return "错误统计"
	case "temperature statistics":
		return "温度统计"
	case "transport statistics":
		return "传输统计"
	case "solid state device statistics":
		return "固态设备统计"
	case "rotating media statistics":
		return "旋转介质统计"
	case "zoned device statistics":
		return "分区设备统计"
	default:
		return p
	}
}
