package collector

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"smartdisk/internal/model"
)

// HibernateRecord is the persisted power-control state for one disk. It is
// written both after every successful collection (fresh recovery data) and
// when a disk is deep-offlined, so the disk can later be woken and
// re-mounted even though it has vanished from lsblk while offline.
type HibernateRecord struct {
	ID          string     `json:"id"` // kernel name (sda, nvme0n1)
	Name        string     `json:"name"`
	Model       string     `json:"model,omitempty"`
	Serial      string     `json:"serial,omitempty"`
	WWN         string     `json:"wwn,omitempty"`
	Capacity    int64      `json:"capacity,omitempty"`
	Interface   string     `json:"interface,omitempty"`
	Rotational  bool       `json:"rotational"`
	Role        string     `json:"role,omitempty"`
	Roles       []string   `json:"roles,omitempty"`
	System      bool       `json:"system,omitempty"` // backs / or /boot (never controllable)
	VolumePaths []string   `json:"volumePaths,omitempty"`
	PartUUIDs   []string   `json:"partUUIDs,omitempty"` // partition / filesystem UUIDs
	Mounts      []MountRef `json:"mounts,omitempty"`    // UUID -> mountpoint pairs to restore
	RaidMember  bool       `json:"raidMember"`
	Offline     bool       `json:"offline"` // deep-offlined via this app
	OfflineAt   int64      `json:"offlineAt,omitempty"`
	UpdatedAt   int64      `json:"updatedAt"`
	// Note is the user-maintained label for this physical drive. It survives
	// reboots and slot changes because records are keyed by serial.
	Note string `json:"note,omitempty"`
}

// MountRef ties a filesystem UUID to the mount point it should be restored
// to on wake (mount UUID=<uuid> <mountPoint>).
type MountRef struct {
	UUID       string `json:"uuid"`
	MountPoint string `json:"mountPoint"`
	Noauto     bool   `json:"noauto,omitempty"` // skip auto re-mount on wake (raid member)
}

// hibernateStore is a JSON-file-backed map of disk id -> HibernateRecord.
type hibernateStore struct {
	path string
	mu   sync.Mutex
	recs map[string]*HibernateRecord
}

// stableKey returns the reboot-stable identifier for a disk: serial number
// when known, else WWN, else the (unstable) kernel name as a last resort.
// Using the kernel name alone breaks wake after a reboot, since fnOS may
// re-letter drives (sda <-> sdb ...).
func stableKey(d *model.Disk) string {
	if s := strings.TrimSpace(d.Serial); s != "" {
		return "sn:" + s
	}
	if w := strings.TrimSpace(d.WWN); w != "" {
		return "wwn:" + w
	}
	return "dev:" + d.ID
}

// stableKeyRec mirrors stableKey for a persisted record.
func stableKeyRec(r *HibernateRecord) string {
	if s := strings.TrimSpace(r.Serial); s != "" {
		return "sn:" + s
	}
	if w := strings.TrimSpace(r.WWN); w != "" {
		return "wwn:" + w
	}
	return "dev:" + r.ID
}

func loadHibernateStore(path string) *hibernateStore {
	s := &hibernateStore{path: path, recs: map[string]*HibernateRecord{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return s // first run / missing file is fine
	}
	var list []*HibernateRecord
	if json.Unmarshal(b, &list) != nil {
		return s
	}
	// Index by stable key (serial), merging any duplicate entries for the
	// same physical drive that older builds may have written (one keyed by
	// kernel name, one by serial). Offline=true wins so a stale online
	// duplicate can never mask a real offline state.
	for _, r := range list {
		if r == nil || r.ID == "" {
			continue
		}
		key := stableKeyRec(r)
		if existing, ok := s.recs[key]; ok {
			mergeHibernateRecord(existing, r)
			continue
		}
		s.recs[key] = r
	}
	return s
}

// mergeHibernateRecord folds src into dst, preferring the offline state and
// the freshest recovery data. Used to dedupe duplicate records for one drive.
func mergeHibernateRecord(dst, src *HibernateRecord) {
	if src.Offline && !dst.Offline {
		dst.Offline = true
		dst.OfflineAt = src.OfflineAt
	}
	if src.UpdatedAt > dst.UpdatedAt {
		// Fresher identity/recovery data wins, but keep the offline flag.
		off, offAt := dst.Offline, dst.OfflineAt
		*dst = *src
		dst.Offline, dst.OfflineAt = off, offAt
	}
}

func (s *hibernateStore) saveLocked() {
	if s.path == "" {
		return
	}
	list := make([]*HibernateRecord, 0, len(s.recs))
	for _, r := range s.recs {
		list = append(list, r)
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		log.Printf("[hibernate] write temp state %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Printf("[hibernate] rename state %s -> %s: %v", tmp, s.path, err)
	}
}

// upsertFromDisk refreshes the recovery data for a live disk. Never touches
// the Offline flag, so a collected (online) disk keeps its record fresh.
func (s *hibernateStore) upsertFromDisk(d *model.Disk) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := stableKey(d)
	r := s.recs[key]
	if r == nil {
		r = &HibernateRecord{}
		s.recs[key] = r
	}
	r.ID = d.ID
	r.Name = d.Name
	r.Model = d.Model
	r.Serial = d.Serial
	r.WWN = d.WWN
	r.Capacity = d.Capacity
	r.Interface = string(d.Interface)
	r.Rotational = d.Rotational
	r.Role = d.Role
	r.Roles = append([]string(nil), d.Roles...)
	r.System = isSystemDisk(d)
	r.VolumePaths = append([]string(nil), d.VolumePaths...)
	r.RaidMember = isRaidMember(d)
	r.PartUUIDs = r.PartUUIDs[:0]
	r.Mounts = r.Mounts[:0]
	mountedSet := map[string]bool{}
	mountPtSet := map[string]bool{}
	for _, p := range d.Partitions {
		if p.UUID != "" {
			r.PartUUIDs = append(r.PartUUIDs, p.UUID)
			if p.MountPoint != "" {
				r.Mounts = append(r.Mounts, MountRef{UUID: p.UUID, MountPoint: p.MountPoint})
				mountedSet[p.UUID] = true
				mountPtSet[p.MountPoint] = true
			}
		}
	}
	// Also record UUIDs/mount points seen along the storage chain. A disk that
	// feeds an md array (RAID 成员) must NOT have its chain mounts restored by
	// hand: the kernel reassembles the array on rescan and the fs comes back on
	// its own, so those mounts are marked Noauto.
	underRaid := false
	for _, hop := range d.Chain {
		if hop.IsRaid {
			underRaid = true
		}
		if hop.UUID == "" {
			continue
		}
		dup := false
		for _, u := range r.PartUUIDs {
			if u == hop.UUID {
				dup = true
				break
			}
		}
		if !dup {
			r.PartUUIDs = append(r.PartUUIDs, hop.UUID)
		}
		if hop.MountPoint == "" || mountedSet[hop.UUID] || mountPtSet[hop.MountPoint] {
			continue
		}
		dupMount := false
		for _, m := range r.Mounts {
			if m.UUID == hop.UUID {
				dupMount = true
				break
			}
		}
		if !dupMount {
			mr := MountRef{UUID: hop.UUID, MountPoint: hop.MountPoint}
			if underRaid {
				mr.Noauto = true
			}
			r.Mounts = append(r.Mounts, mr)
			mountPtSet[mr.MountPoint] = true
		}
	}
	r.UpdatedAt = time.Now().Unix()
	s.saveLocked()
}

// markOffline flags a disk as deep-offlined.
func (s *hibernateStore) markOffline(key string, offline bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.recs[key]; r != nil {
		r.Offline = offline
		if offline {
			r.OfflineAt = time.Now().Unix()
		} else {
			r.OfflineAt = 0
		}
		r.UpdatedAt = time.Now().Unix()
		s.saveLocked()
	}
}

// noteFor returns the persisted user note for a disk by stable identity, or
// the legacy kernel-name key, or "".
func (s *hibernateStore) noteFor(d *model.Disk) string {
	if r := s.getByDisk(d); r != nil {
		return r.Note
	}
	return ""
}

// setNote updates the user note on the record matching this disk, creating a
// minimal record when none exists (e.g. note added before first full scan).
func (s *hibernateStore) setNote(d *model.Disk, note string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := stableKey(d)
	r := s.recs[key]
	if r == nil {
		if note == "" {
			return
		}
		r = &HibernateRecord{ID: d.ID, Name: d.Name}
		s.recs[key] = r
	}
	r.Note = note
	r.UpdatedAt = time.Now().Unix()
	s.saveLocked()
}

// get returns a copy of the record for key, or nil.
func (s *hibernateStore) get(key string) *HibernateRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.recs[key]; r != nil {
		cp := *r
		return &cp
	}
	return nil
}

// getByDisk finds the record for a disk by trying its stable key first and
// falling back to the (legacy) kernel-name key, so records written before
// the stable-key change still resolve.
func (s *hibernateStore) getByDisk(d *model.Disk) *HibernateRecord {
	if r := s.get(stableKey(d)); r != nil {
		return r
	}
	return s.get("dev:" + d.ID)
}

// offlineRecords returns copies of all records flagged offline.
func (s *hibernateStore) offlineRecords() []HibernateRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []HibernateRecord
	for _, r := range s.recs {
		if r.Offline {
			out = append(out, *r)
		}
	}
	return out
}

// isRaidMember reports whether the disk feeds an md RAID array.
func isRaidMember(d *model.Disk) bool {
	for _, r := range d.Roles {
		if r == "RAID 成员" {
			return true
		}
	}
	return false
}

// isSystemDisk reports whether the disk backs / or /boot (never controllable).
func isSystemDisk(d *model.Disk) bool {
	for _, r := range d.Roles {
		if r == "系统盘" {
			return true
		}
	}
	for _, v := range d.VolumePaths {
		if v == "/" || v == "/boot" || v == "/boot/efi" {
			return true
		}
	}
	return false
}

// recordToDisk builds a placeholder Disk for an offline disk that no longer
// appears in lsblk, so the UI can still show a card with a Wake button.
func recordToDisk(r HibernateRecord) model.Disk {
	d := model.Disk{
		ID:                r.ID,
		Name:              r.Name,
		Model:             r.Model,
		Serial:            r.Serial,
		WWN:               r.WWN,
		Capacity:          r.Capacity,
		Rotational:        r.Rotational,
		Role:              r.Role,
		Roles:             append([]string(nil), r.Roles...),
		VolumePaths:       append([]string(nil), r.VolumePaths...),
		PowerState:        "offline",
		Hibernated:        true,
		OfflineAt:         r.OfflineAt,
		UserNote:          r.Note,
		PercentUsed:       -1,
		PercentLifeRemain: -1,
		UsagePercent:      -1,
		ManufactureDate:   "未知",
		Health:            model.GradeUnknown,
	}
	switch r.Interface {
	case "NVMe":
		d.Interface = model.IfNVMe
		d.Type = model.TypeNVMe
	case "USB":
		d.Interface = model.IfUSB
	case "SCSI":
		d.Interface = model.IfSCSI
	default:
		d.Interface = model.IfSATA
	}
	if d.Type == "" {
		if r.Rotational {
			d.Type = model.TypeHDD
		} else {
			d.Type = model.TypeSSD
		}
	}
	return d
}
