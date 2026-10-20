package smart

import (
	"fmt"

	"smartdisk/internal/model"
)

// Grade computes the overall health verdict and human-readable notes. It
// combines the SMART overall status, key failure indicators, temperature, and
// SSD wear. Conservative: any hard failure indicator escalates the grade.
func Grade(d *model.Disk) {
	notes := []string{}
	score := 0 // higher = worse

	mark := func(a *model.SmartAttr) {
		for i := range d.Attrs {
			if d.Attrs[i].ID == a.ID {
				d.Attrs[i].Critical = true
			}
		}
	}

	attrByID := func(ids ...int) *model.SmartAttr {
		for i := range d.Attrs {
			for _, id := range ids {
				if d.Attrs[i].ID == id {
					return &d.Attrs[i]
				}
			}
		}
		return nil
	}

	if !d.SmartAvail {
		d.Health = model.GradeUnknown
		if d.SmartDeferred {
			d.HealthNotes = []string{"为避免唤醒休眠磁盘，SMART 数据暂缓读取"}
		} else {
			d.HealthNotes = []string{"无法读取 SMART 数据（可能为 USB 桥接不支持透传）"}
		}
		return
	}
	if d.SmartSupported && !d.SmartOK {
		score += 100
		notes = append(notes, "SMART 整体自检未通过（FAILING）")
	}

	if a := attrByID(5); a != nil && a.RawValue > 0 {
		score += 60
		mark(a)
		notes = append(notes, fmt.Sprintf("已重映射扇区 %d 个（出现坏道迹象）", a.RawValue))
	}
	if a := attrByID(197); a != nil && a.RawValue > 0 {
		score += 70
		mark(a)
		notes = append(notes, fmt.Sprintf("待重映射扇区 %d 个（潜在坏道）", a.RawValue))
	}
	if a := attrByID(198); a != nil && a.RawValue > 0 {
		score += 50
		mark(a)
		notes = append(notes, fmt.Sprintf("离线无法校正扇区 %d 个", a.RawValue))
	}
	if a := attrByID(199); a != nil && a.RawValue > 0 {
		score += 20
		mark(a)
		notes = append(notes, fmt.Sprintf("接口 CRC 错误 %d 次（检查数据线/供电/桥接）", a.RawValue))
	}
	if a := attrByID(187); a != nil && a.RawValue > 0 {
		score += 40
		mark(a)
		notes = append(notes, fmt.Sprintf("报告的不可校正错误 %d 次", a.RawValue))
	}

	if d.PercentUsed >= 0 {
		switch {
		case d.PercentUsed >= 95:
			score += 80
			notes = append(notes, fmt.Sprintf("闪存寿命已用 %d%%，接近寿命终点", d.PercentUsed))
		case d.PercentUsed >= 80:
			score += 40
			notes = append(notes, fmt.Sprintf("闪存寿命已用 %d%%，磨损偏高", d.PercentUsed))
		}
	}

	if d.Temperature > 0 {
		warn, crit := tempThresholds(d)
		switch {
		case d.Temperature >= crit:
			score += 50
			notes = append(notes, fmt.Sprintf("温度 %d°C 过高（≥%d°C）", d.Temperature, crit))
		case d.Temperature >= warn:
			score += 15
			notes = append(notes, fmt.Sprintf("温度 %d°C 偏高（建议 <%d°C）", d.Temperature, warn))
		}
	}

	if d.Type == model.TypeNVMe && d.UnsafeShutdowns > 0 && d.PowerCycles > 0 {
		ratio := float64(d.UnsafeShutdowns) / float64(d.PowerCycles)
		if ratio > 0.3 && d.UnsafeShutdowns > 10 {
			score += 10
			notes = append(notes, fmt.Sprintf("异常断电 %d 次（占通电周期 %.0f%%）", d.UnsafeShutdowns, ratio*100))
		}
	}

	// btrfs pool errors: checksum/corruption errors are the most serious
	// (silent data damage), IO errors indicate the pool already hit problems.
	if b := d.BtrfsErrs; b != nil && b.Total() > 0 {
		if b.CorruptionErrs > 0 || b.GenerationErrs > 0 {
			score += 70
			notes = append(notes, fmt.Sprintf("btrfs 存储池检测到 %d 个校验/数据损坏错误（卷 %s，疑似数据静默损坏）", b.CorruptionErrs+b.GenerationErrs, d.BtrfsMount))
		}
		if io := b.WriteIOErrs + b.ReadIOErrs + b.FlushIOErrs; io > 0 {
			score += 40
			notes = append(notes, fmt.Sprintf("btrfs 存储池累计 %d 个读写 IO 错误（卷 %s）", io, d.BtrfsMount))
		}
	}

	switch {
	case score >= 100:
		d.Health = model.GradeCritical
	case score >= 40:
		d.Health = model.GradeWarning
	case score >= 10:
		d.Health = model.GradeFair
	default:
		d.Health = model.GradeGood
		notes = append(notes, "各项指标正常")
	}
	if len(notes) == 0 {
		notes = append(notes, "各项指标正常")
	}
	d.HealthNotes = notes
}

func tempThresholds(d *model.Disk) (int, int) {
	switch d.Type {
	case model.TypeNVMe:
		return 70, 80
	case model.TypeSSD:
		return 60, 70
	default:
		return 50, 60
	}
}
