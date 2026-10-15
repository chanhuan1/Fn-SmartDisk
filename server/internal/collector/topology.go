package collector

import (
	"strconv"
	"strings"

	"smartdisk/internal/model"
)

// annotateTopology walks the lsblk tree (including md/lvm virtual devices) and
// annotates each physical disk with its storage role, the volumes it serves,
// and the outward chain (disk -> part -> raid -> lvm -> fs -> mountpoint).
func (c *Collector) annotateTopology(disks []model.Disk, roots []lsblkNode) {
	byName := map[string]*model.Disk{}
	for i := range disks {
		byName[disks[i].ID] = &disks[i]
	}

	for _, root := range roots {
		d, ok := byName[root.Name]
		if !ok {
			continue
		}
		var chain []model.ChainHop
		var vols []string
		isSystem := false
		var walk func(n lsblkNode)
		walk = func(n lsblkNode) {
			// skip the physical disk itself; record descendants
			if n.Name != root.Name {
				hop := model.ChainHop{
					Name:  n.Name,
					Type:  nodeTypeLabel(n),
					Label: n.Label,
					UUID:  n.UUID,
				}
				hop.IsRaid = n.FsType == "linux_raid_member" || strings.HasPrefix(n.Type, "raid")
				if v, err := n.Size.Int64(); err == nil {
					hop.Size = v
				}
				mp := firstMount(n)
				hop.MountPoint = mp
				chain = append(chain, hop)

				if mp != "" {
					vols = append(vols, mp)
					if mp == "/" {
						isSystem = true
					}
				}
			}
			for _, ch := range n.Children {
				walk(ch)
			}
		}
		walk(root)

		d.Chain = chain
		d.VolumePaths = unique(vols)

		// Roles
		roles := []string{}
		if isSystem || hasRootMount(root) {
			roles = append(roles, "系统盘")
		}
		if hasRaidMember(root) {
			roles = append(roles, "RAID 成员")
		}
		if len(roles) == 0 {
			roles = append(roles, "独立盘")
		}
		d.Roles = roles
		d.Role = strings.Join(roles, " · ")
	}
}

// buildTopologyTree builds the full storage tree for the overview page,
// attaching each physical disk's health to its node.
func (c *Collector) buildTopologyTree(disks []model.Disk, roots []lsblkNode) []*model.StorageNode {
	health := map[string]model.HealthGrade{}
	for _, d := range disks {
		health[d.ID] = d.Health
	}
	var out []*model.StorageNode
	for _, r := range roots {
		if skipTopoRoot(r.Name) {
			continue
		}
		out = append(out, buildNode(r, health))
	}
	return out
}

// skipTopoRoot reports whether a top-level device should be hidden from the
// topology tree (virtual loop/ram/rom devices, not real storage).
func skipTopoRoot(name string) bool {
	return strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") ||
		strings.HasPrefix(name, "sr") || strings.HasPrefix(name, "zram") ||
		strings.HasPrefix(name, "dm-")
}

func buildNode(n lsblkNode, health map[string]model.HealthGrade) *model.StorageNode {
	sn := &model.StorageNode{
		Name:       n.Name,
		Type:       nodeTypeLabel(n),
		Label:      n.Label,
		FsType:     n.FsType,
		MountPoint: firstMount(n),
		UsePercent: -1,
	}
	if v, err := n.Size.Int64(); err == nil {
		sn.Size = v
	}
	if v, err := n.FsUsed.Int64(); err == nil {
		sn.Used = v
	}
	if s := strings.TrimSuffix(n.FsUsePct, "%"); s != "" {
		if p, err := strconv.Atoi(s); err == nil {
			sn.UsePercent = p
		}
	}
	if n.Type == "disk" {
		sn.DiskID = n.Name
		sn.Health = health[n.Name]
	}
	for _, ch := range n.Children {
		sn.Children = append(sn.Children, buildNode(ch, health))
	}
	return sn
}

// nodeTypeLabel gives a friendly type label for a tree node.
func nodeTypeLabel(n lsblkNode) string {
	switch {
	case n.Type == "disk":
		return "物理盘"
	case n.Type == "part":
		return "分区"
	case strings.HasPrefix(n.Type, "raid"):
		return strings.ToUpper(n.Type)
	case n.Type == "lvm":
		return "LVM"
	default:
		if n.FsType != "" {
			return n.FsType
		}
		return n.Type
	}
}

// firstMount returns the node's first mount point (single or list form).
func firstMount(n lsblkNode) string {
	if n.MountPoint != "" {
		return n.MountPoint
	}
	for _, m := range n.MountPoints {
		if m != "" {
			return m
		}
	}
	return ""
}

func hasRootMount(n lsblkNode) bool {
	if firstMount(n) == "/" {
		return true
	}
	for _, ch := range n.Children {
		if hasRootMount(ch) {
			return true
		}
	}
	return false
}

func hasRaidMember(n lsblkNode) bool {
	if n.FsType == "linux_raid_member" {
		return true
	}
	for _, ch := range n.Children {
		if hasRaidMember(ch) {
			return true
		}
	}
	return false
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
