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
		dev := lastChainDevice(d)
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

// lastChainDevice returns the device path of the final hop in the disk's
// storage chain. In fnOS this is the lvm/dm node backing the btrfs volume;
// when there's no lvm layer it's the md or partition device. btrfsDeviceStats
// falls back to the pool's single device when the path doesn't match, so a
// simplified last-hop lookup is sufficient here.
func lastChainDevice(d *model.Disk) string {
	if len(d.Chain) == 0 {
		return ""
	}
	last := d.Chain[len(d.Chain)-1]
	return "/dev/" + last.Name
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
