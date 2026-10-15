// Package model defines the canonical disk / SMART view model that the
// collector produces and the web panel renders. It is transport-agnostic and
// shared by both process roles.
package model

// InterfaceKind classifies how a disk is attached.
type InterfaceKind string

const (
	IfSATA InterfaceKind = "SATA"
	IfNVMe InterfaceKind = "NVMe"
	IfUSB  InterfaceKind = "USB"
	IfSCSI InterfaceKind = "SCSI"
	IfUnk  InterfaceKind = "未知"
)

// DeviceType is the media kind.
type DeviceType string

const (
	TypeSSD     DeviceType = "SSD"
	TypeHDD     DeviceType = "HDD"
	TypeNVMe    DeviceType = "NVMe"
	TypeUnknown DeviceType = "未知"
)

// HealthGrade is the overall health verdict.
type HealthGrade string

const (
	GradeGood     HealthGrade = "优"
	GradeFair     HealthGrade = "良"
	GradeWarning  HealthGrade = "警"
	GradeCritical HealthGrade = "危"
	GradeUnknown  HealthGrade = "未知"
)

// SmartAttr is one normalized SMART attribute row.
type SmartAttr struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Cn         string `json:"cn,omitempty"`        // 中文名
	Important  bool   `json:"important,omitempty"` // 关键健康指标
	Value      int    `json:"value"`
	Worst      int    `json:"worst"`
	Thresh     int    `json:"thresh"`
	Raw        string `json:"raw"`
	RawValue   int64  `json:"rawValue"`
	WhenFailed string `json:"whenFailed"`
	// Flags marks whether this row influenced the health verdict.
	Critical bool `json:"critical,omitempty"`
}

// RawValueToInt returns RawValue clamped into int range for threshold math.
func (a SmartAttr) RawValueToInt() int {
	v := a.RawValue
	if v < 0 {
		return 0
	}
	if v > 1000000 {
		return 1000000
	}
	return int(v)
}

// BridgeInfo describes a USB bridge chip and the SAT passthrough used.
type BridgeInfo struct {
	VendorID    string `json:"vendorId,omitempty"`
	ProductID   string `json:"productId,omitempty"`
	Chip        string `json:"chip,omitempty"`
	Passthrough string `json:"passthrough,omitempty"` // smartctl -d value that worked
}

// Disk is the full canonical record for one physical disk.
type Disk struct {
	ID           string        `json:"id"`   // stable id (kernel name, e.g. sda / nvme0n1)
	Name         string        `json:"name"` // /dev node
	Model        string        `json:"model"`
	Serial       string        `json:"serial"`
	Firmware     string        `json:"firmware"`
	Capacity     int64         `json:"capacity"`     // bytes
	UsedCapacity int64         `json:"usedCapacity"` // 已用空间(已挂载分区), bytes; 0 if unknown
	UsagePercent int           `json:"usagePercent"` // 0-100, -1 if unknown
	Interface    InterfaceKind `json:"interface"`
	Type         DeviceType    `json:"type"`
	Rotational   bool          `json:"rotational"`

	Temperature int `json:"temperatureC"` // current temp, 0 if unknown

	PowerOnHours    int64 `json:"powerOnHours"`
	PowerCycles     int64 `json:"powerCycles"`
	UnsafeShutdowns int64 `json:"unsafeShutdowns,omitempty"`

	// Life
	PercentUsed       int    `json:"percentUsed"`          // 0-100, -1 if unknown
	PercentLifeRemain int    `json:"percentLifeRemain"`    // 0-100, -1 if unknown
	TotalBytesWritten int64  `json:"totalBytesWritten"`    // 0 if unknown
	LifeSource        string `json:"lifeSource,omitempty"` // provenance of the life estimate
	Vendor            string `json:"vendor,omitempty"`     // detected SSD controller vendor
	Controller        string `json:"controller,omitempty"` // SSD controller label (e.g. 群联 Phison S11)

	SmartSupported bool `json:"smartSupported"`
	SmartEnabled   bool `json:"smartEnabled"`
	SmartOK        bool `json:"smartOk"`       // overall PASSED
	SmartAvail     bool `json:"smartAvail"`    // data could be read at all
	SmartDeferred  bool `json:"smartDeferred"` // deferred to avoid waking a rotational disk

	Attrs []SmartAttr `json:"attrs,omitempty"`

	// NVMe health log (only for NVMe disks), exposed for the SMART table.
	NVMe *NVMeHealth `json:"nvme,omitempty"`

	Bridge *BridgeInfo `json:"bridge,omitempty"`

	ManufactureDate string `json:"manufactureDate"` // ISO or "未知"

	Health      HealthGrade `json:"health"`
	HealthNotes []string    `json:"healthNotes,omitempty"`

	// CollectedAt is when this disk's data was gathered (unix seconds).
	CollectedAt int64 `json:"collectedAt,omitempty"`

	// UsageYears is derived from PowerOnHours (24/7) for display.
	UsageYears float64 `json:"usageYears"`

	// Extra detail (A/D/E). Only fields relevant to the disk type are set.

	// A: SMART depth
	PhysicalSectorSize int             `json:"physicalSectorSize,omitempty"` // bytes
	LogicalSectorSize  int             `json:"logicalSectorSize,omitempty"`  // bytes
	RotationRate       int             `json:"rotationRate,omitempty"`       // rpm, 0 = SSD
	FormFactor         string          `json:"formFactor,omitempty"`
	TrimSupported      bool            `json:"trimSupported,omitempty"`
	SelfTests          []SelfTestEntry `json:"selfTests,omitempty"`
	ATAErrorLog        []ATAErrorEntry `json:"ataErrorLog,omitempty"`
	DevStats           []DevStatEntry  `json:"devStats,omitempty"`

	// D: partitions
	Partitions []Partition `json:"partitions,omitempty"`

	// E: link / topology
	Link *LinkInfo `json:"link,omitempty"`

	// Storage role and the chain this physical disk feeds into (RAID/LVM/vol).
	Role        string     `json:"role,omitempty"`        // 系统盘 / 数据盘 / md成员盘 etc (label)
	Roles       []string   `json:"roles,omitempty"`       // multiple role tags
	VolumePaths []string   `json:"volumePaths,omitempty"` // mount points reachable via this disk
	Chain       []ChainHop `json:"chain,omitempty"`       // storage path from this disk outward

	// Additional hardware detail (officially unavailable on fnOS).
	WWN            string         `json:"wwn,omitempty"`            // WWN / EUI-64 global identifier
	CacheSizeBytes int64          `json:"cacheSizeBytes,omitempty"` // drive write cache size
	NCQDepth       int            `json:"ncqDepth,omitempty"`       // Native Command Queue depth
	TempHistory    *TempHistory   `json:"tempHistory,omitempty"`    // SCT temperature history
	LoadCycleCount int64          `json:"loadCycleCount,omitempty"` // SMART 193 load/unload cycles (HDD)
	SATAPhy        []SATAPhyEvent `json:"sataPhy,omitempty"`        // SATA PHY link error counters

	// btrfs pool error counters for the volume this disk backs (fnOS uses
	// btrfs on md RAID). Only set when the disk feeds a btrfs pool.
	BtrfsErrs  *BtrfsErrors `json:"btrfsErrs,omitempty"`
	BtrfsMount string       `json:"btrfsMount,omitempty"` // e.g. /vol2

	// Power management / runtime state.
	PowerState string `json:"powerState,omitempty"` // active / idle / standby / offline / unknown
	APMLevel   int    `json:"apmLevel,omitempty"`   // 0 = unknown/unsupported

	// Power-control capability flags (drives which buttons the UI shows).
	Hibernated bool  `json:"hibernated,omitempty"` // deep-offlined via this app
	OfflineAt  int64 `json:"offlineAt,omitempty"`  // unix seconds
	CanSleep   bool  `json:"canSleep"`
	CanOffline bool  `json:"canOffline"`
	CanWake    bool  `json:"canWake"`
	// NoSleepReason / NoOfflineReason explain why a button is disabled
	// (系统盘 / NVMe 不支持 / USB 桥接不支持 …). Empty when allowed.
	NoSleepReason   string `json:"noSleepReason,omitempty"`
	NoOfflineReason string `json:"noOfflineReason,omitempty"`
	// NeedsRaidConfirm is true when the disk is an md RAID member: deep
	// offline would degrade the array, so the UI must collect an explicit
	// risk acknowledgement before calling the API.
	NeedsRaidConfirm bool `json:"needsRaidConfirm,omitempty"`

	// UserNote is a user-maintained free-form label persisted per disk.
	UserNote string `json:"userNote,omitempty"`
}

// BtrfsErrors is the per-device btrfs error_stats for a pool.
type BtrfsErrors struct {
	WriteIOErrs    int64 `json:"writeIOErrs"`
	ReadIOErrs     int64 `json:"readIOErrs"`
	FlushIOErrs    int64 `json:"flushIOErrs"`
	CorruptionErrs int64 `json:"corruptionErrs"` // checksum mismatches (bit-rot indicator)
	GenerationErrs int64 `json:"generationErrs"`
}

// Total returns the sum of all btrfs error counters.
func (b *BtrfsErrors) Total() int64 {
	if b == nil {
		return 0
	}
	return b.WriteIOErrs + b.ReadIOErrs + b.FlushIOErrs + b.CorruptionErrs + b.GenerationErrs
}

// TempHistory is the SCT temperature history (drive-recorded min/max/current).
type TempHistory struct {
	Current      int   `json:"current"`
	Max          int   `json:"max"`          // highest ever seen, -1 unknown
	Min          int   `json:"min"`          // lowest ever seen, -1 unknown
	OverTempMin  int64 `json:"overTempMin"`  // minutes spent over limit, -1 unknown
	UnderTempMin int64 `json:"underTempMin"` // minutes spent under limit, -1 unknown
}

// SATAPhyEvent is one SATA PHY error counter (link/cable health).
type SATAPhyEvent struct {
	Name  string `json:"name"`
	Cn    string `json:"cn,omitempty"`
	Value int64  `json:"value"`
}

// PowerEvent is one entry in the power-control event log (sleep/offline/
// wake/remount outcomes), shown in the Power page log panel.
type PowerEvent struct {
	Time   int64  `json:"time"`             // unix seconds
	DiskID string `json:"diskId"`           // sda / nvme0n1
	Model  string `json:"model,omitempty"`  // disk model for readability
	Action string `json:"action"`           // sleep / offline / wake / remount
	OK     bool   `json:"ok"`               // success or failure
	Detail string `json:"detail,omitempty"` // human-readable outcome / error
}

// ChainHop is one step in a disk's storage chain.
type ChainHop struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // disk/part/raid1/lvm/btrfs/ext4/vfat
	Label      string `json:"label,omitempty"`
	Size       int64  `json:"size,omitempty"`
	UUID       string `json:"uuid,omitempty"`
	MountPoint string `json:"mountPoint,omitempty"`
	IsRaid     bool   `json:"isRaid,omitempty"`     // node is an md RAID device (fstype linux_raid_member or type raid*)
	IsPhysical bool   `json:"isPhysical,omitempty"` // node is a whole-disk or partition (vs md/lvm virtual)
}

// StorageNode is one node in the full storage topology tree.
type StorageNode struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Label      string         `json:"label,omitempty"`
	Size       int64          `json:"size,omitempty"`
	FsType     string         `json:"fsType,omitempty"`
	MountPoint string         `json:"mountPoint,omitempty"`
	Used       int64          `json:"used,omitempty"`
	UsePercent int            `json:"usePercent"`       // -1 unknown
	DiskID     string         `json:"diskId,omitempty"` // set when this node is a physical disk
	Health     HealthGrade    `json:"health,omitempty"` // physical disk health
	Children   []*StorageNode `json:"children,omitempty"`
}

// SelfTestEntry is one SMART self-test log record (ATA) or NVMe self-test.
type SelfTestEntry struct {
	Type        string `json:"type"` // Short / Extended / Conveyance / Selective
	TypeCn      string `json:"typeCn,omitempty"`
	Status      string `json:"status"` // Completed / Aborted / Interrupted / In progress
	StatusCn    string `json:"statusCn,omitempty"`
	Passed      bool   `json:"passed"`
	LifetimeH   int64  `json:"lifetimeH"` // power-on hours when the test ran
	LBAFirstErr string `json:"lbaFirstErr,omitempty"`
}

// ATAErrorEntry is one ATA SMART error-log record.
type ATAErrorEntry struct {
	ErrorNum    int    `json:"errorNum"`
	LifetimeH   int64  `json:"lifetimeH"`
	Command     string `json:"command,omitempty"`
	Description string `json:"description,omitempty"`
}

// DevStatEntry is one Device Statistics page entry (e.g. temperature history,
// NAND writes, grown defects).
type DevStatEntry struct {
	Name    string `json:"name"`
	Cn      string `json:"cn,omitempty"`
	Value   string `json:"value"`
	Flagged bool   `json:"flagged,omitempty"` // DSN flag: value may be a problem
}

// Partition is one partition/filesystem on the disk.
type Partition struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	FsType     string `json:"fsType,omitempty"`
	Label      string `json:"label,omitempty"`
	UUID       string `json:"uuid,omitempty"`
	MountPoint string `json:"mountPoint,omitempty"`
	Used       int64  `json:"used"`
	UsePercent int    `json:"usePercent"` // -1 if unknown
}

// LinkInfo describes the negotiated link speed vs the device max (E).
type LinkInfo struct {
	Kind       string `json:"kind"`       // SATA / NVMe / USB
	Negotiated string `json:"negotiated"` // e.g. "6.0 Gb/s" / "PCIe 3.0 x4" / "USB 3.2"
	DeviceMax  string `json:"deviceMax"`  // max the device supports
	Downgraded bool   `json:"downgraded"` // negotiated < deviceMax (possible cable/port issue)
	Note       string `json:"note,omitempty"`
}

// NVMeHealth is the NVMe SMART / Health Information log (Identify), shown as
// the attribute table for NVMe disks.
type NVMeHealth struct {
	CriticalWarning      int   `json:"criticalWarning"`
	Temperature          int   `json:"temperature"`          // composite, Kelvin-converted to C by smartctl
	AvailableSpare       int   `json:"availableSpare"`       // percent
	AvailableSpareThresh int   `json:"availableSpareThresh"` // percent
	PercentageUsed       int   `json:"percentageUsed"`       // percent
	DataUnitsRead        int64 `json:"dataUnitsRead"`
	DataUnitsWritten     int64 `json:"dataUnitsWritten"`
	HostReadCommands     int64 `json:"hostReadCommands"`
	HostWriteCommands    int64 `json:"hostWriteCommands"`
	ControllerBusyTime   int64 `json:"controllerBusyTime"` // minutes
	PowerCycles          int64 `json:"powerCycles"`
	PowerOnHours         int64 `json:"powerOnHours"`
	UnsafeShutdowns      int64 `json:"unsafeShutdowns"`
	MediaErrors          int64 `json:"mediaErrors"`
	NumErrLogEntries     int64 `json:"numErrLogEntries"`
	WarningTempTime      int64 `json:"warningTempTime"`  // minutes
	CriticalTempTime     int64 `json:"criticalTempTime"` // minutes
}

// Summary is the per-disk row shown in the list view (lighter than Disk).
type Summary struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Model            string        `json:"model"`
	Serial           string        `json:"serial"`
	Capacity         int64         `json:"capacity"`
	UsedCapacity     int64         `json:"usedCapacity"`
	UsagePercent     int           `json:"usagePercent"`
	Interface        InterfaceKind `json:"interface"`
	Type             DeviceType    `json:"type"`
	Temperature      int           `json:"temperatureC"`
	Health           HealthGrade   `json:"health"`
	Bridge           string        `json:"bridge,omitempty"` // chip label
	PercentUsed      int           `json:"percentUsed"`
	SmartAvail       bool          `json:"smartAvail"`
	SmartDeferred    bool          `json:"smartDeferred"`
	Role             string        `json:"role,omitempty"`
	VolumePaths      []string      `json:"volumePaths,omitempty"`
	Controller       string        `json:"controller,omitempty"`
	PowerState       string        `json:"powerState,omitempty"`
	Hibernated       bool          `json:"hibernated,omitempty"`
	CanSleep         bool          `json:"canSleep"`
	CanOffline       bool          `json:"canOffline"`
	CanWake          bool          `json:"canWake"`
	NoSleepReason    string        `json:"noSleepReason,omitempty"`
	NoOfflineReason  string        `json:"noOfflineReason,omitempty"`
	NeedsRaidConfirm bool          `json:"needsRaidConfirm,omitempty"`
	// UserNote is the user's persistent label for this disk.
	UserNote string `json:"userNote,omitempty"`
}

// ToSummary projects a Disk to its list-row form.
func (d *Disk) ToSummary() Summary {
	s := Summary{
		ID:               d.ID,
		Name:             d.Name,
		Model:            d.Model,
		Serial:           d.Serial,
		Capacity:         d.Capacity,
		UsedCapacity:     d.UsedCapacity,
		UsagePercent:     d.UsagePercent,
		Interface:        d.Interface,
		Type:             d.Type,
		Temperature:      d.Temperature,
		Health:           d.Health,
		PercentUsed:      d.PercentUsed,
		SmartAvail:       d.SmartAvail,
		SmartDeferred:    d.SmartDeferred,
		Role:             d.Role,
		VolumePaths:      d.VolumePaths,
		Controller:       d.Controller,
		PowerState:       d.PowerState,
		Hibernated:       d.Hibernated,
		CanSleep:         d.CanSleep,
		CanOffline:       d.CanOffline,
		CanWake:          d.CanWake,
		NoSleepReason:    d.NoSleepReason,
		NoOfflineReason:  d.NoOfflineReason,
		NeedsRaidConfirm: d.NeedsRaidConfirm,
		UserNote:         d.UserNote,
	}
	if d.Bridge != nil {
		s.Bridge = d.Bridge.Chip
	}
	return s
}

// Overview is the payload for the list endpoint.
type Overview struct {
	Disks     []Summary `json:"disks"`
	Count     int       `json:"count"`
	Generated int64     `json:"generated"` // unix seconds
	Mock      bool      `json:"mock"`
	Error     string    `json:"error,omitempty"`
}
