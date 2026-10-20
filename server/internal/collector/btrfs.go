package collector

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"smartdisk/internal/model"
)

// btrfsErrRe parses one line of `btrfs device stats` output:
//
//	[/dev/mapper/trim_vol-0].write_io_errs    0
var btrfsErrRe = regexp.MustCompile(`\[([^\]]+)\]\.(\w+)\s+(\d+)`)

// fillBtrfsErrors queries `btrfs device stats` for each btrfs mount the disk
// backs and records the error counters for this disk's backing device.
// fnOS stacks physical disk -> partition -> md -> lvm -> btrfs; the btrfs
// device path is the lvm (dm) node, resolved through the disk's chain.
func (c *Collector) fillBtrfsErrors(ctx context.Context, d *model.Disk) {
	for _, mp := range d.VolumePaths {
		if !strings.HasPrefix(mp, "/vol") && mp != "/" {
			continue
		}
		dev := chainDeviceForMount(d, mp)
		if dev == "" {
			continue
		}
		errs := c.btrfsDeviceStats(ctx, mp, dev)
		if errs != nil {
			d.BtrfsErrs = errs
			d.BtrfsMount = mp
			return // one btrfs pool per disk in fnOS layout
		}
	}
}

// chainDeviceForMount returns the deepest chain node mounted at mp. This keeps
// branch A separate from branch B on disks that expose multiple filesystems.
func chainDeviceForMount(d *model.Disk, mp string) string {
	bestDepth := -1
	bestName := ""
	for _, hop := range d.Chain {
		if hop.MountPoint == mp && hop.Depth >= bestDepth {
			bestDepth = hop.Depth
			bestName = hop.Name
		}
	}
	if bestName == "" {
		return ""
	}
	return "/dev/" + bestName
}

// btrfsDeviceStats runs `btrfs device stats <mount>` and returns the error
// counters for the device matching devPath (or the single device if the pool
// has exactly one, since fnOS single-device pools do).
func (c *Collector) btrfsDeviceStats(ctx context.Context, mount, devPath string) *model.BtrfsErrors {
	out, _, err := c.run.Run(ctx, "btrfs", "device", "stats", mount)
	if err != nil {
		return nil
	}
	byDev := map[string]*model.BtrfsErrors{}
	var first *model.BtrfsErrors
	for _, line := range strings.Split(out, "\n") {
		m := btrfsErrRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		dev, field, val := m[1], m[2], m[3]
		n, _ := strconv.ParseInt(val, 10, 64)
		e := byDev[dev]
		if e == nil {
			e = &model.BtrfsErrors{}
			byDev[dev] = e
			if first == nil {
				first = e
			}
		}
		switch field {
		case "write_io_errs":
			e.WriteIOErrs = n
		case "read_io_errs":
			e.ReadIOErrs = n
		case "flush_io_errs":
			e.FlushIOErrs = n
		case "corruption_errs":
			e.CorruptionErrs = n
		case "generation_errs":
			e.GenerationErrs = n
		}
	}
	if e, ok := byDev[devPath]; ok {
		return e
	}
	if len(byDev) == 1 {
		return first
	}
	return nil
}
