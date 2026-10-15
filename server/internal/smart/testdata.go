package smart

// Sample smartctl -a -j payloads used by tests. These mirror real outputs
// (trimmed to the fields we consume).

const sampleSATAHDD = `{
  "device": {"name": "/dev/sda", "info_name": "/dev/sda", "type": "scsi", "protocol": "SCSI"},
  "model_name": "WDC WD40EFRX-68N32N0",
  "serial_number": "WD-WCC7K3ABCDEF",
  "firmware_version": "82.00A82",
  "user_capacity": {"blocks": 7814037168, "bytes": 4000787030016},
  "rotation_rate": 5400,
  "smart_support": {"available": true, "enabled": true},
  "smart_status": {"passed": true},
  "temperature": {"current": 38},
  "power_on_time": {"hours": 21340},
  "power_cycle_count": 640,
  "ata_smart_attributes": {"table": [
    {"id": 1, "name": "Raw_Read_Error_Rate", "value": 200, "worst": 200, "thresh": 51, "when_failed": "", "raw": {"value": 0, "string": "0"}},
    {"id": 5, "name": "Reallocated_Sector_Ct", "value": 200, "worst": 200, "thresh": 140, "when_failed": "", "raw": {"value": 0, "string": "0"}},
    {"id": 9, "name": "Power_On_Hours", "value": 71, "worst": 71, "thresh": 0, "when_failed": "", "raw": {"value": 21340, "string": "21340"}},
    {"id": 12, "name": "Power_Cycle_Count", "value": 100, "worst": 100, "thresh": 0, "when_failed": "", "raw": {"value": 640, "string": "640"}},
    {"id": 194, "name": "Temperature_Celsius", "value": 109, "worst": 95, "thresh": 0, "when_failed": "", "raw": {"value": 38, "string": "38"}},
    {"id": 197, "name": "Current_Pending_Sector", "value": 200, "worst": 200, "thresh": 0, "when_failed": "", "raw": {"value": 0, "string": "0"}},
    {"id": 199, "name": "UDMA_CRC_Error_Count", "value": 200, "worst": 200, "thresh": 0, "when_failed": "", "raw": {"value": 0, "string": "0"}}
  ]}
}`

const sampleSATASSD = `{
  "device": {"name": "/dev/sdb", "info_name": "/dev/sdb", "type": "scsi", "protocol": "SCSI"},
  "model_name": "Samsung SSD 870 EVO 1TB",
  "serial_number": "S5Y2NJ0R123456A",
  "firmware_version": "SVT02B6Q",
  "user_capacity": {"blocks": 1953525168, "bytes": 1000204886016},
  "rotation_rate": 0,
  "smart_support": {"available": true, "enabled": true},
  "smart_status": {"passed": true},
  "temperature": {"current": 41},
  "power_on_time": {"hours": 8200},
  "power_cycle_count": 412,
  "ata_smart_attributes": {"table": [
    {"id": 5, "name": "Reallocated_Sector_Ct", "value": 100, "worst": 100, "thresh": 10, "when_failed": "", "raw": {"value": 0, "string": "0"}},
    {"id": 9, "name": "Power_On_Hours", "value": 96, "worst": 96, "thresh": 0, "when_failed": "", "raw": {"value": 8200, "string": "8200"}},
    {"id": 12, "name": "Power_Cycle_Count", "value": 96, "worst": 96, "thresh": 0, "when_failed": "", "raw": {"value": 412, "string": "412"}},
    {"id": 177, "name": "Wear_Leveling_Count", "value": 98, "worst": 98, "thresh": 0, "when_failed": "", "raw": {"value": 12, "string": "12"}},
    {"id": 194, "name": "Temperature_Celsius", "value": 59, "worst": 50, "thresh": 0, "when_failed": "", "raw": {"value": 41, "string": "41"}},
    {"id": 241, "name": "Total_LBAs_Written", "value": 99, "worst": 99, "thresh": 0, "when_failed": "", "raw": {"value": 42949672960, "string": "42949672960"}}
  ]}
}`

const sampleNVMe = `{
  "device": {"name": "/dev/nvme0", "info_name": "/dev/nvme0", "type": "nvme", "protocol": "NVMe"},
  "model_name": "Samsung SSD 980 PRO 2TB",
  "serial_number": "S69ENF0R987654B",
  "firmware_version": "5B2QGXA7",
  "nvme_total_capacity": {"bytes": 2000398934016},
  "smart_support": {"available": true, "enabled": true},
  "smart_status": {"passed": true},
  "nvme_smart_health_information_log": {
    "critical_warning": 0,
    "temperature": 45,
    "available_spare": 100,
    "available_spare_threshold": 10,
    "percentage_used": 7,
    "data_units_read": 52428800,
    "data_units_written": 83886080,
    "host_read_commands": 1234567890,
    "host_write_commands": 2345678901,
    "controller_busy_time": 4567,
    "power_on_hours": 5120,
    "power_cycles": 305,
    "unsafe_shutdowns": 12,
    "media_errors": 0,
    "num_err_log_entries": 3,
    "warning_temp_time": 0,
    "critical_temp_time": 0
  }
}`

const sampleFailingHDD = `{
  "device": {"name": "/dev/sdc", "info_name": "/dev/sdc", "type": "scsi", "protocol": "SCSI"},
  "model_name": "ST2000DM008-2FR102",
  "serial_number": "ZFL0ABCDE",
  "firmware_version": "0001",
  "user_capacity": {"blocks": 3907029168, "bytes": 2000398934016},
  "rotation_rate": 7200,
  "smart_support": {"available": true, "enabled": true},
  "smart_status": {"passed": false},
  "temperature": {"current": 47},
  "power_on_time": {"hours": 30100},
  "power_cycle_count": 1200,
  "ata_smart_attributes": {"table": [
    {"id": 5, "name": "Reallocated_Sector_Ct", "value": 120, "worst": 120, "thresh": 10, "when_failed": "", "raw": {"value": 88, "string": "88"}},
    {"id": 197, "name": "Current_Pending_Sector", "value": 100, "worst": 100, "thresh": 0, "when_failed": "", "raw": {"value": 16, "string": "16"}},
    {"id": 9, "name": "Power_On_Hours", "value": 60, "worst": 60, "thresh": 0, "when_failed": "", "raw": {"value": 30100, "string": "30100"}}
  ]}
}`
