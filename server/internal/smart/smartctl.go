// Package smart parses smartctl JSON output (-a -j) into the canonical model.
// It tolerates partial / vendor-specific payloads and never fails hard on a
// missing field; unknown values are represented with the model's sentinels.
package smart

import (
	"encoding/json"
	"strconv"
	"strings"

	"smartdisk/internal/model"
)

// smartctlJSON mirrors the subset of smartctl -a -j we consume. Fields are
// pointers or zero-tolerant so partial payloads still parse.
type smartctlJSON struct {
	Device struct {
		Name     string `json:"name"`
		InfoName string `json:"info_name"`
		Type     string `json:"type"`
		Protocol string `json:"protocol"`
	} `json:"device"`
	ModelName    string `json:"model_name"`
	ModelFamily  string `json:"model_family"`
	SerialNumber string `json:"serial_number"`
	Firmware     string `json:"firmware_version"`
	WWN          *struct {
		NAA int   `json:"naa"`
		OUI int   `json:"oui"`
		ID  int64 `json:"id"`
	} `json:"wwn"`
	EUI64 *struct {
		OUI int   `json:"oui"`
		ID  int64 `json:"id"`
	} `json:"eui64"`
	NGUID string `json:"nguid"`
	// Drive write cache / NCQ (from smartctl -i JSON where available).
	ReadCacheLookahead *struct {
		Enabled bool `json:"enabled"`
	} `json:"read_cache_lookahead"`
	WriteCache *struct {
		Enabled    bool  `json:"enabled"`
		Size       int64 `json:"size"`
		Reordering bool  `json:"reordering"`
	} `json:"write_cache"`
	NCQDepth     int `json:"ncq_depth"`
	APMLevel     int `json:"apm_level"`
	UserCapacity struct {
		Blocks int64 `json:"blocks"`
		Bytes  int64 `json:"bytes"`
	} `json:"user_capacity"`
	NVMeCapacity struct {
		Bytes int64 `json:"bytes"`
	} `json:"nvme_total_capacity"`
	RotationRate int `json:"rotation_rate"` // 0 => SSD
	SmartSupport struct {
		Available bool `json:"available"`
		Enabled   bool `json:"enabled"`
	} `json:"smart_support"`
	SmartStatus struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature struct {
		Current int `json:"current"`
	} `json:"temperature"`
	PowerOnTime struct {
		Hours int64 `json:"hours"`
	} `json:"power_on_time"`
	PowerCycleCount int64 `json:"power_cycle_count"`

	ATASmartAttributes *struct {
		Table []struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			Value      int    `json:"value"`
			Worst      int    `json:"worst"`
			Thresh     int    `json:"thresh"`
			WhenFailed string `json:"when_failed"`
			Raw        struct {
				Value  int64  `json:"value"`
				String string `json:"string"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`

	NVMeSmartHealthLog *struct {
		Temperature          int   `json:"temperature"`
		AvailableSpare       int   `json:"available_spare"`
		AvailableSpareThresh int   `json:"available_spare_threshold"`
		PercentageUsed       int   `json:"percentage_used"`
		DataUnitsRead        int64 `json:"data_units_read"`
		DataUnitsWritten     int64 `json:"data_units_written"`
		HostReadCommands     int64 `json:"host_read_commands"`
		HostWriteCommands    int64 `json:"host_write_commands"`
		ControllerBusyTime   int64 `json:"controller_busy_time"`
		PowerOnHours         int64 `json:"power_on_hours"`
		PowerCycles          int64 `json:"power_cycles"`
		UnsafeShutdowns      int64 `json:"unsafe_shutdowns"`
		MediaErrors          int64 `json:"media_errors"`
		NumErrLogEntries     int64 `json:"num_err_log_entries"`
		WarningTempTime      int64 `json:"warning_temp_time"`
		CriticalTempTime     int64 `json:"critical_temp_time"`
		CriticalWarning      int   `json:"critical_warning"`
	} `json:"nvme_smart_health_information_log"`

	// --- A: extra SMART depth ---
	LogicalBlockSize  int `json:"logical_block_size"`
	PhysicalBlockSize int `json:"physical_block_size"`
	FormFactor        *struct {
		ATAVersion string `json:"ata_version_string"`
		Name       string `json:"name"`
	} `json:"form_factor"`
	Trim struct {
		Supported bool `json:"supported"`
	} `json:"trim"`
	InterfaceSpeed *struct {
		Max struct {
			SataValue int    `json:"sata_value"`
			String    string `json:"string"`
			Units     string `json:"units"`
		} `json:"max"`
		Current struct {
			SataValue int    `json:"sata_value"`
			String    string `json:"string"`
			Units     string `json:"units"`
		} `json:"current"`
	} `json:"interface_speed"`

	ATASelfTestLog *struct {
		Table []struct {
			Type struct {
				Value  int    `json:"value"`
				String string `json:"string"`
			} `json:"type"`
			Status struct {
				Value  int    `json:"value"`
				String string `json:"string"`
				Passed *bool  `json:"passed"`
			} `json:"status"`
			LifetimeHours int64  `json:"lifetime_hours"`
			LBAFirstErr   *int64 `json:"lba_of_first_error"`
		} `json:"table"`
	} `json:"ata_smart_self_test_log"`

	ATAErrorLog *struct {
		Table []struct {
			ErrorNumber   int    `json:"error_number"`
			LifetimeHours int64  `json:"lifetime_hours"`
			Description   string `json:"description"`
		} `json:"table"`
	} `json:"ata_smart_error_log"`

	DeviceStatistics *struct {
		Pages []struct {
			Number int    `json:"number"`
			Name   string `json:"name"`
			Table  []struct {
				Offset int    `json:"offset"`
				Name   string `json:"name"`
				Value  int64  `json:"value"`
				Flags  int    `json:"flags"`
			} `json:"table"`
		} `json:"pages"`
	} `json:"ata_device_statistics"`

	// SCT temperature history.
	SCTTemp *struct {
		Current         int   `json:"current"`
		Max             int   `json:"max"`
		Min             int   `json:"min"`
		LimitMax        int   `json:"limit_max"`
		LimitMin        int   `json:"limit_min"`
		OverLimitCount  int64 `json:"over_limit_count"`
		UnderLimitCount int64 `json:"under_limit_count"`
	} `json:"scttemp"`

	// SATA PHY event counters (smartctl -l sataphy -j merged into -a on 7.x).
	ATAPHYLog *struct {
		Table []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Value int64  `json:"value"`
		} `json:"table"`
	} `json:"ata_sata_phy_event_counters"`
}

// ParseSmartctl converts smartctl -a -j output into a partially-filled Disk.
// Identity fields (interface type, bridge) are filled in by the collector
// which has the udev/lsusb context; this fills everything SMART-derived.
func ParseSmartctl(data []byte, d *model.Disk) error {
	var j smartctlJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}

	if j.ModelName != "" {
		d.Model = j.ModelName
	}
	if j.SerialNumber != "" {
		d.Serial = j.SerialNumber
	}
	if j.Firmware != "" {
		d.Firmware = j.Firmware
	}
	if j.UserCapacity.Bytes > 0 {
		d.Capacity = j.UserCapacity.Bytes
	} else if j.NVMeCapacity.Bytes > 0 {
		d.Capacity = j.NVMeCapacity.Bytes
	}

	d.SmartSupported = j.SmartSupport.Available
	d.SmartEnabled = j.SmartSupport.Enabled
	d.SmartOK = j.SmartStatus.Passed
	// Treat SMART as available only when smartctl reports the support flag or
	// a usable health payload (NVMe log / ATA attribute table). A bare error
	// JSON from a failed probe must not be mistaken for readable SMART data.
	d.SmartAvail = j.SmartSupport.Available ||
		j.NVMeSmartHealthLog != nil ||
		(j.ATASmartAttributes != nil && len(j.ATASmartAttributes.Table) > 0)

	// Media type
	if j.NVMeSmartHealthLog != nil {
		d.Type = model.TypeNVMe
	} else if j.RotationRate > 0 {
		// Drive reports a spindle speed: definitely rotational (HDD).
		d.Type = model.TypeHDD
		d.Rotational = true
	} else if !d.Rotational {
		// rotation_rate == 0 is unreliable (some HDDs / USB bridges report 0
		// or omit it). Only trust it as "SSD" when lsblk did NOT already tell
		// us the device is rotational.
		d.Type = model.TypeSSD
		d.Rotational = false
	}

	if j.Temperature.Current > 0 {
		d.Temperature = j.Temperature.Current
	}
	if j.PowerOnTime.Hours > 0 {
		d.PowerOnHours = j.PowerOnTime.Hours
	}
	if j.PowerCycleCount > 0 {
		d.PowerCycles = j.PowerCycleCount
	}

	// ATA attribute table
	if t := j.ATASmartAttributes; t != nil {
		vendor := vendorOf(j.ModelName)
		d.Vendor = string(vendor)
		d.Attrs = make([]model.SmartAttr, 0, len(t.Table))
		for _, a := range t.Table {
			meta := AttrMetaFor(a.ID)
			cn := meta.Cn
			// Vendor-specific names fill in only when the drive reports a
			// generic "Unknown_Attribute"/"Unknown_SSD_Attribute" name. When the
			// drive gives a real name, prefer translating that name (avoids
			// mislabelling e.g. Phison attr233 which differs across controllers).
			if isUnknownAttrName(a.Name) {
				if vn := vendorAttrName(vendor, a.ID); vn != "" {
					cn = vn
				}
			} else if vn := knownAttrNameCn(a.Name); vn != "" {
				cn = vn
			}
			d.Attrs = append(d.Attrs, model.SmartAttr{
				ID:         a.ID,
				Name:       a.Name,
				Cn:         cn,
				Important:  meta.Important,
				Value:      a.Value,
				Worst:      a.Worst,
				Thresh:     a.Thresh,
				WhenFailed: a.WhenFailed,
				Raw:        a.Raw.String,
				RawValue:   a.Raw.Value,
			})
		}
		d.PercentUsed, d.PercentLifeRemain, d.TotalBytesWritten = deriveATALife(d.Attrs, vendorOf(d.Model))
		// Vendor-aware life estimate. deriveVendorLife intentionally overrides
		// the generic deriveATALife result with the more authoritative
		// vendor-specific source (attr231/169/Samsung-TBW) when available, and
		// records the provenance in LifeSource.
		if remain, src, ok := deriveVendorLife(d, vendor, d.Attrs, int64(j.LogicalBlockSize)); ok {
			d.PercentLifeRemain = remain
			d.PercentUsed = clamp100(100 - remain)
			d.LifeSource = src
		}
	}

	// NVMe health log
	if nv := j.NVMeSmartHealthLog; nv != nil {
		d.PercentUsed = nv.PercentageUsed
		d.PercentLifeRemain = clamp100(100 - nv.PercentageUsed)
		if nv.PowerOnHours > 0 {
			d.PowerOnHours = nv.PowerOnHours
		}
		if nv.PowerCycles > 0 {
			d.PowerCycles = nv.PowerCycles
		}
		d.UnsafeShutdowns = nv.UnsafeShutdowns
		if nv.Temperature > 0 {
			d.Temperature = nv.Temperature
		}
		// NVMe data_units_written is in units of 1000 * 512 bytes.
		if nv.DataUnitsWritten > 0 {
			d.TotalBytesWritten = nv.DataUnitsWritten * 1000 * 512
		}
		// Expose the full health log for the NVMe SMART table.
		d.NVMe = &model.NVMeHealth{
			CriticalWarning:      nv.CriticalWarning,
			Temperature:          nv.Temperature,
			AvailableSpare:       nv.AvailableSpare,
			AvailableSpareThresh: nv.AvailableSpareThresh,
			PercentageUsed:       nv.PercentageUsed,
			DataUnitsRead:        nv.DataUnitsRead,
			DataUnitsWritten:     nv.DataUnitsWritten,
			HostReadCommands:     nv.HostReadCommands,
			HostWriteCommands:    nv.HostWriteCommands,
			ControllerBusyTime:   nv.ControllerBusyTime,
			PowerCycles:          nv.PowerCycles,
			PowerOnHours:         nv.PowerOnHours,
			UnsafeShutdowns:      nv.UnsafeShutdowns,
			MediaErrors:          nv.MediaErrors,
			NumErrLogEntries:     nv.NumErrLogEntries,
			WarningTempTime:      nv.WarningTempTime,
			CriticalTempTime:     nv.CriticalTempTime,
		}
	}

	// --- A: extra detail ---
	if j.LogicalBlockSize > 0 {
		d.LogicalSectorSize = j.LogicalBlockSize
	}
	if j.PhysicalBlockSize > 0 {
		d.PhysicalSectorSize = j.PhysicalBlockSize
	}
	if j.RotationRate > 0 {
		d.RotationRate = j.RotationRate
	}
	if j.FormFactor != nil && j.FormFactor.Name != "" {
		d.FormFactor = j.FormFactor.Name
	}
	d.TrimSupported = j.Trim.Supported

	// WWN / EUI-64 global identifier.
	d.WWN = buildWWN(j)

	// Write cache size and NCQ depth.
	if j.WriteCache != nil && j.WriteCache.Size > 0 {
		d.CacheSizeBytes = j.WriteCache.Size
	}
	if j.NCQDepth > 0 {
		d.NCQDepth = j.NCQDepth
	}
	if j.APMLevel > 0 {
		d.APMLevel = j.APMLevel
	}

	// SCT temperature history.
	if t := j.SCTTemp; t != nil {
		d.TempHistory = &model.TempHistory{
			Current:      t.Current,
			Max:          t.Max,
			Min:          t.Min,
			OverTempMin:  t.OverLimitCount,
			UnderTempMin: t.UnderLimitCount,
		}
	}

	// SATA PHY link error counters.
	if ph := j.ATAPHYLog; ph != nil {
		for _, e := range ph.Table {
			d.SATAPhy = append(d.SATAPhy, model.SATAPhyEvent{
				Name:  e.Name,
				Cn:    phyEventCn(e.Name),
				Value: e.Value,
			})
		}
	}

	// Load/Unload cycle count (SMART 193) — HDD head parking wear.
	for _, a := range d.Attrs {
		if a.ID == 193 {
			d.LoadCycleCount = a.RawValue
		}
	}

	// E: link speed (SATA). interface_speed only appears for ATA/SATA devices,
	// so its presence is enough; NVMe/USB link is filled by the collector.
	if sp := j.InterfaceSpeed; sp != nil {
		cur := sp.Current.String
		max := sp.Max.String
		down := false
		// Compare the parsed Gb/s from the display strings, not the SataValue
		// enum (its mapping to actual speed varies and can disagree with the
		// string, e.g. both strings "6.0 Gb/s" while enums differ).
		curG, maxG := parseGbps(cur), parseGbps(max)
		if curG > 0 && maxG > 0 && curG < maxG {
			down = true
		}
		d.Link = &model.LinkInfo{
			Kind:       "SATA",
			Negotiated: cur,
			DeviceMax:  max,
			Downgraded: down,
		}
		if down {
			d.Link.Note = "协商速率低于设备最大速率，可能为线缆/端口/背板限制"
		}
	}

	// A: self-test log (latest first as reported).
	if st := j.ATASelfTestLog; st != nil {
		for _, e := range st.Table {
			typeCn := SelfTestTypeCn(e.Type.String)
			statusCn, passed := SelfTestStatusCn(e.Status.String)
			// smartctl's own passed flag takes precedence when present.
			if e.Status.Passed != nil {
				passed = *e.Status.Passed
			}
			entry := model.SelfTestEntry{
				Type:      e.Type.String,
				TypeCn:    typeCn,
				Status:    e.Status.String,
				StatusCn:  statusCn,
				Passed:    passed,
				LifetimeH: e.LifetimeHours,
			}
			if e.LBAFirstErr != nil {
				entry.LBAFirstErr = formatInt64(*e.LBAFirstErr)
			}
			d.SelfTests = append(d.SelfTests, entry)
		}
	}

	// A: ATA error log.
	if el := j.ATAErrorLog; el != nil {
		for _, e := range el.Table {
			d.ATAErrorLog = append(d.ATAErrorLog, model.ATAErrorEntry{
				ErrorNum:    e.ErrorNumber,
				LifetimeH:   e.LifetimeHours,
				Description: e.Description,
			})
		}
	}

	// A: device statistics (flatten pages).
	if ds := j.DeviceStatistics; ds != nil {
		for _, pg := range ds.Pages {
			for _, row := range pg.Table {
				name := pg.Name + " · " + row.Name
				d.DevStats = append(d.DevStats, model.DevStatEntry{
					Name:    name,
					Cn:      DevStatCn(name),
					Value:   formatInt64(row.Value),
					Flagged: row.Flags&0x1 != 0,
				})
			}
		}
	}

	d.UsageYears = float64(d.PowerOnHours) / 24.0 / 365.0
	// SSD controller identification (display label).
	d.Controller = ControllerLabel(d)
	return nil
}

// parseGbps extracts the numeric rate from a string like "6.0 Gb/s" or
// "16.0GT/s". Returns 0 when unparseable.
func parseGbps(s string) float64 {
	s = strings.TrimSpace(s)
	var f float64
	for _, tok := range strings.Fields(s) {
		if v, err := strconv.ParseFloat(tok, 64); err == nil {
			f = v
			break
		}
	}
	return f
}

func formatInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}

func clamp100(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// writeUnitBytes returns the byte multiplier for a "total written" attribute
// raw value for a given vendor + attribute id. Phison attr241 is in GB, SMI
// attr241/245/246 are in 32MiB units, everything else defaults to 512B LBAs.
// This avoids the classic "write amount shown as a few MB" mis-parse.
func writeUnitBytes(vendor VendorKind, id int) int64 {
	switch vendor {
	case VendorPhison:
		if id == 241 {
			return 1024 * 1024 * 1024 // GB
		}
	case VendorSMI:
		if id == 241 || id == 245 || id == 246 {
			return 32 * 1024 * 1024 // 32MiB
		}
	}
	return 512 // LBA sectors (default)
}

// deriveATALife extracts SSD life / wear metrics from vendor ATA attributes.
// Attribute IDs and write units vary by vendor; we check the common ones.
func deriveATALife(attrs []model.SmartAttr, vendor VendorKind) (percentUsed, lifeRemain int, bytesWritten int64) {
	percentUsed, lifeRemain = -1, -1
	for _, a := range attrs {
		name := strings.ToLower(a.Name)
		switch a.ID {
		case 231: // Life_Left (Crucial/Samsung) or Temperature on some; check name
			if strings.Contains(name, "life") || strings.Contains(name, "ssd_life") {
				lifeRemain = clamp100(a.RawValueToInt())
			}
		case 202: // Percent_Lifetime_Used / Remaining
			if strings.Contains(name, "remain") {
				lifeRemain = clamp100(a.RawValueToInt())
			} else {
				percentUsed = clamp100(a.RawValueToInt())
			}
		case 177: // Wear_Leveling_Count (Samsung) raw ~ used count
			if percentUsed < 0 {
				percentUsed = clamp100(a.RawValueToInt())
			}
		case 241: // Total_LBAs_Written
			bytesWritten = a.RawValue * writeUnitBytes(vendor, a.ID)
		case 242: // Total_LBAs_Read (ignored for write life)
		}
		if strings.Contains(name, "total_lbas_written") || strings.Contains(name, "lifetime_writes") {
			if bytesWritten == 0 {
				bytesWritten = a.RawValue * writeUnitBytes(vendor, a.ID)
			}
		}
		if strings.Contains(name, "percent_life_remaining") || strings.Contains(name, "ssd_life_left") {
			lifeRemain = clamp100(a.RawValueToInt())
		}
		if strings.Contains(name, "percent_lifetime_used") || strings.Contains(name, "wear_range") {
			percentUsed = clamp100(a.RawValueToInt())
		}
	}
	// Reconcile: if we only have one of used/remain, derive the other.
	if percentUsed >= 0 && lifeRemain < 0 {
		lifeRemain = clamp100(100 - percentUsed)
	}
	if lifeRemain >= 0 && percentUsed < 0 {
		percentUsed = clamp100(100 - lifeRemain)
	}
	return
}
