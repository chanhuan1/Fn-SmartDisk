package smart

import (
	"regexp"
	"strings"

	"smartdisk/internal/model"
)

// usbBridge maps well-known USB storage bridge vendor:product IDs to a chip
// label and the smartctl SAT passthrough most likely to work. IDs are
// lowercase "vvvv:pppp".
type bridgeEntry struct {
	Chip        string
	Passthrough string
}

var usbBridgeDB = map[string]bridgeEntry{
	// JMicron
	"152d:0567": {Chip: "JMicron JMS567", Passthrough: "sat,usbjmicron"},
	"152d:0578": {Chip: "JMicron JMS578", Passthrough: "sat,usbjmicron"},
	"152d:0583": {Chip: "JMicron JMS583", Passthrough: "sntjmicron"},
	"152d:2329": {Chip: "JMicron JM20329", Passthrough: "usbjmicron"},
	// ASMedia
	"174c:1153": {Chip: "ASMedia ASM1153", Passthrough: "sat"},
	"174c:55aa": {Chip: "ASMedia ASM1351/ASM235", Passthrough: "sat"},
	"174c:2364": {Chip: "ASMedia ASM2364", Passthrough: "sat"},
	// Realtek
	"0bda:9210": {Chip: "Realtek RTL9210", Passthrough: "sntrealtek"},
	// Sunplus
	"04fc:0c25": {Chip: "Sunplus SPIF225/6", Passthrough: "usbsunplus"},
	// Innostor
	"1f75:0621": {Chip: "Innostor IS621", Passthrough: "sat"},
	// Cypress
	"04b4:6830": {Chip: "Cypress CY7C68300", Passthrough: "usbcypress"},
	// Prolific
	"067b:2506": {Chip: "Prolific PL2506", Passthrough: "sat"},
	"067b:2573": {Chip: "Prolific PL2573", Passthrough: "usbprolific"},
	// Initio
	"13fd:0840": {Chip: "Initio INIC-1608", Passthrough: "sat"},
	// VIA Labs
	"2109:0715": {Chip: "VIA VL715", Passthrough: "sat"},
	"2109:0717": {Chip: "VIA VL717", Passthrough: "sat"},
}

// passthroughOrder is the fallback probing order for unknown USB bridges.
var passthroughOrder = []string{
	"sat",
	"usbjmicron",
	"usbsunplus",
	"usbrealtek",
	"usbcypress",
	"usbprolific",
	"sntjmicron",
	"sntrealtek",
}

var lsusbRe = regexp.MustCompile(`(?i)ID\s+([0-9a-f]{4}):([0-9a-f]{4})\s+(.*)`)

// ParseLsusb parses `lsusb` output into vendor:product -> description.
func ParseLsusb(out string) map[string]string {
	m := map[string]string{}
	for _, ln := range strings.Split(out, "\n") {
		if mm := lsusbRe.FindStringSubmatch(ln); mm != nil {
			id := strings.ToLower(mm[1] + ":" + mm[2])
			m[id] = strings.TrimSpace(mm[3])
		}
	}
	return m
}

// BridgeFor returns the chip label and preferred passthrough for a
// vendor:product id, consulting lsusb output for a friendlier chip name.
func BridgeFor(vendorID, productID string, lsusb map[string]string) *model.BridgeInfo {
	if vendorID == "" || productID == "" {
		return nil
	}
	id := strings.ToLower(vendorID + ":" + productID)
	b := &model.BridgeInfo{VendorID: vendorID, ProductID: productID}
	if e, ok := usbBridgeDB[id]; ok {
		b.Chip = e.Chip
		b.Passthrough = e.Passthrough
	} else if desc, ok := lsusb[id]; ok && desc != "" {
		b.Chip = desc
	} else {
		b.Chip = "USB 桥接 " + id
	}
	return b
}

// PassthroughOrder returns the candidate smartctl -d values to try for a USB
// disk, preferring the known chip's protocol first when present.
func PassthroughOrder(b *model.BridgeInfo) []string {
	if b == nil || b.Passthrough == "" {
		return append([]string(nil), passthroughOrder...)
	}
	out := []string{b.Passthrough}
	for _, p := range passthroughOrder {
		if p != b.Passthrough {
			out = append(out, p)
		}
	}
	return out
}
