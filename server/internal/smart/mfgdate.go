package smart

import (
	"strings"
)

// InferManufactureDate best-effort infers a drive's manufacture date from its
// serial number using per-vendor rules. Most drives do not expose a date, so
// this returns "未知" unless a rule matches confidently. It never fabricates.
func InferManufactureDate(vendor, serial string) string {
	v := strings.ToLower(strings.TrimSpace(vendor))
	s := strings.TrimSpace(serial)
	if s == "" {
		return "未知"
	}
	switch {
	case strings.Contains(v, "seagate"):
		return "未知" // serial rules inconsistent across families
	case strings.Contains(v, "western digital"), strings.Contains(v, "wdc"), strings.Contains(v, "sandisk"):
		return "未知" // date code is on the label, not in serial
	case strings.Contains(v, "samsung"):
		return "未知" // no stable public serial->date rule
	}
	return "未知"
}
