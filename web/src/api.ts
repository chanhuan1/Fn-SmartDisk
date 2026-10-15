// API client + shared types mirroring the backend model.

// Detect the fnOS unified-gateway prefix at runtime. When the panel is served
// through the gateway, the app lives under /app/<prefix>; on a bare port it
// lives at the root. We derive the API base from the current location so the
// same build works in both modes.
function detectBase(): string {
  const m = window.location.pathname.match(/^(\/app\/[^/]+)/);
  return m ? m[1] : "";
}
export const API_BASE = detectBase();

export type InterfaceKind = "SATA" | "NVMe" | "USB" | "SCSI" | "未知";
export type DeviceType = "SSD" | "HDD" | "NVMe" | "未知";
export type HealthGrade = "优" | "良" | "警" | "危" | "未知";

export interface BridgeInfo {
  vendorId?: string;
  productId?: string;
  chip?: string;
  passthrough?: string;
}

export interface SelfTestEntry {
  type: string;
  typeCn?: string;
  status: string;
  statusCn?: string;
  passed: boolean;
  lifetimeH: number;
  lbaFirstErr?: string;
}

export interface ATAErrorEntry {
  errorNum: number;
  lifetimeH: number;
  command?: string;
  description?: string;
}

export interface DevStatEntry {
  name: string;
  cn?: string;
  value: string;
  flagged?: boolean;
}

export interface Partition {
  name: string;
  size: number;
  fsType?: string;
  label?: string;
  uuid?: string;
  mountPoint?: string;
  used: number;
  usePercent: number;
}

export interface LinkInfo {
  kind: string;
  negotiated: string;
  deviceMax: string;
  downgraded: boolean;
  note?: string;
}

export interface TempHistory {
  current: number;
  max: number;
  min: number;
  overTempMin: number;
  underTempMin: number;
}

export interface SATAPhyEvent {
  name: string;
  cn?: string;
  value: number;
}

export interface BtrfsErrors {
  writeIOErrs: number;
  readIOErrs: number;
  flushIOErrs: number;
  corruptionErrs: number;
  generationErrs: number;
}

export interface ChainHop {
  name: string;
  type: string;
  label?: string;
  size?: number;
  mountPoint?: string;
}

export interface StorageNode {
  name: string;
  type: string;
  label?: string;
  size?: number;
  fsType?: string;
  mountPoint?: string;
  used?: number;
  usePercent: number;
  diskId?: string;
  health?: HealthGrade;
  children?: StorageNode[];
}

export interface SmartAttr {
  id: number;
  name: string;
  cn?: string;
  important?: boolean;
  value: number;
  worst: number;
  thresh: number;
  raw: string;
  rawValue: number;
  whenFailed: string;
  critical?: boolean;
}

export interface NVMeHealth {
  criticalWarning: number;
  temperature: number;
  availableSpare: number;
  availableSpareThresh: number;
  percentageUsed: number;
  dataUnitsRead: number;
  dataUnitsWritten: number;
  hostReadCommands: number;
  hostWriteCommands: number;
  controllerBusyTime: number;
  powerCycles: number;
  powerOnHours: number;
  unsafeShutdowns: number;
  mediaErrors: number;
  numErrLogEntries: number;
  warningTempTime: number;
  criticalTempTime: number;
}

export interface Disk {
  id: string;
  name: string;
  model: string;
  serial: string;
  firmware: string;
  capacity: number;
  usedCapacity: number;
  usagePercent: number;
  interface: InterfaceKind;
  type: DeviceType;
  rotational: boolean;
  temperatureC: number;
  powerOnHours: number;
  powerCycles: number;
  unsafeShutdowns?: number;
  percentUsed: number;
  percentLifeRemain: number;
  totalBytesWritten: number;
  lifeSource?: string;
  vendor?: string;
  controller?: string;
  smartSupported: boolean;
  smartEnabled: boolean;
  smartOk: boolean;
  smartAvail: boolean;
  smartDeferred: boolean;
  attrs?: SmartAttr[];
  nvme?: NVMeHealth;
  bridge?: BridgeInfo;
  manufactureDate: string;
  health: HealthGrade;
  healthNotes?: string[];
  collectedAt?: number;
  usageYears: number;
  physicalSectorSize?: number;
  logicalSectorSize?: number;
  rotationRate?: number;
  formFactor?: string;
  trimSupported?: boolean;
  selfTests?: SelfTestEntry[];
  ataErrorLog?: ATAErrorEntry[];
  devStats?: DevStatEntry[];
  partitions?: Partition[];
  link?: LinkInfo;
  role?: string;
  roles?: string[];
  volumePaths?: string[];
  chain?: ChainHop[];
  wwn?: string;
  cacheSizeBytes?: number;
  ncqDepth?: number;
  tempHistory?: TempHistory;
  loadCycleCount?: number;
  sataPhy?: SATAPhyEvent[];
  btrfsErrs?: BtrfsErrors;
  btrfsMount?: string;
  powerState?: string;
  apmLevel?: number;
  hibernated?: boolean;
  offlineAt?: number;
  canSleep: boolean;
  canOffline: boolean;
  canWake: boolean;
  noSleepReason?: string;
  noOfflineReason?: string;
  needsRaidConfirm?: boolean;
  userNote?: string;
}

export interface Summary {
  id: string;
  name: string;
  model: string;
  serial: string;
  capacity: number;
  usedCapacity: number;
  usagePercent: number;
  interface: InterfaceKind;
  type: DeviceType;
  temperatureC: number;
  health: HealthGrade;
  bridge?: string;
  percentUsed: number;
  smartAvail: boolean;
  smartDeferred: boolean;
  role?: string;
  volumePaths?: string[];
  controller?: string;
  powerState?: string;
  hibernated?: boolean;
  offlineAt?: number;
  canSleep: boolean;
  canOffline: boolean;
  canWake: boolean;
  noSleepReason?: string;
  noOfflineReason?: string;
  needsRaidConfirm?: boolean;
  userNote?: string;
}

export interface Overview {
  disks: Summary[];
  count: number;
  generated: number;
  mock: boolean;
  error?: string;
}

async function get<T>(url: string): Promise<T> {
  let res: Response;
  try {
    res = await fetch(url, { headers: { Accept: "application/json" } });
  } catch (e: any) {
    throw new Error(netErrorText(e));
  }
  if (!res.ok) {
    let msg = httpErrorText(res.status);
    try {
      const j = await res.json();
      if (j && j.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  return (await res.json()) as T;
}

async function post<T>(url: string): Promise<T> {
  let res: Response;
  try {
    res = await fetch(url, { method: "POST", headers: { Accept: "application/json" } });
  } catch (e: any) {
    throw new Error(netErrorText(e));
  }
  if (!res.ok) {
    let msg = httpErrorText(res.status);
    try {
      const j = await res.json();
      if (j && j.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  return (await res.json()) as T;
}

async function postJSON<T>(url: string, body?: unknown): Promise<T> {
  let res: Response;
  try {
    res = await fetch(url, {
      method: "POST",
      headers: { Accept: "application/json", "Content-Type": "application/json" },
      body: body === undefined ? "{}" : JSON.stringify(body),
    });
  } catch (e: any) {
    const err = new Error(netErrorText(e)) as Error & { status?: number };
    throw err;
  }
  const j = await res.json().catch(() => null);
  if (!res.ok) {
    const err = new Error(j?.error || httpErrorText(res.status)) as Error & { status?: number };
    err.status = res.status;
    throw err;
  }
  return j as T;
}

// Friendlier network/HTTP error text: fetch() rejects with TypeError on
// network failure/timeout; HTTP statuses get a Chinese hint.
function netErrorText(e: any): string {
  const raw = e?.message || "";
  if (/timeout|timed out|abort/i.test(raw)) {
    return "采集超时：磁盘数量较多或有磁盘正在唤醒，请稍后重试";
  }
  return "无法连接后端服务（网络中断或服务未启动）";
}

function httpErrorText(status: number): string {
  if (status === 504 || status === 408) {
    return `采集超时（HTTP ${status}）：磁盘数量较多或有磁盘正在唤醒，请稍后重试`;
  }
  if (status === 502 || status === 503) {
    return `后端服务暂不可用（HTTP ${status}），请稍后重试`;
  }
  return `HTTP ${status}`;
}

export interface PowerResult {
  ok: boolean;
  message: string;
  disk?: Disk;
}

// Streamed power op: posts with Accept: text/event-stream and invokes
// onStep(step, detail) for each live step, resolving with the final message.
export async function streamPower(
  url: string,
  body: unknown,
  onStep: (step: string, detail: string) => void,
): Promise<PowerResult> {
  const res = await fetch(url, {
    method: "POST",
    headers: {
      Accept: "text/event-stream",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body ?? {}),
  });
  if (!res.ok || !res.body) {
    const j = await res.json().catch(() => null);
    throw new Error(j?.error || `HTTP ${res.status}`);
  }
  const reader = res.body.getReader();
  const dec = new TextDecoder();
  let buf = "";
  let final: PowerResult | null = null;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    // SSE events are separated by a blank line.
    let idx;
    while ((idx = buf.indexOf("\n\n")) >= 0) {
      const raw = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      for (const line of raw.split("\n")) {
        if (!line.startsWith("data:")) continue;
        const payload = line.slice(5).trim();
        if (!payload) continue;
        try {
          const j = JSON.parse(payload);
          if (j.done) {
            if (!j.ok) throw new Error(j.error || "操作失败");
            final = { ok: true, message: j.message || "完成" };
          } else if (j.step) {
            onStep(j.step, j.detail || "");
          }
        } catch (e) {
          if (e instanceof SyntaxError) continue;
          throw e;
        }
      }
    }
  }
  return final ?? { ok: true, message: "完成" };
}

export interface PowerEvent {
  time: number;
  diskId: string;
  model?: string;
  action: string; // sleep / offline / wake / remount
  ok: boolean;
  detail?: string;
}

export const api = {
  overview: () => get<Overview>(`${API_BASE}/api/disks`),
  refresh: () => post<Overview>(`${API_BASE}/api/disks/refresh`),
  disk: (id: string) => get<Disk>(`${API_BASE}/api/disks/${encodeURIComponent(id)}`),
  refreshDisk: (id: string) => post<Disk>(`${API_BASE}/api/disks/${encodeURIComponent(id)}/refresh`),
  health: () => get<{ ok: boolean; time: number; version?: string }>(`${API_BASE}/api/health`),
  topology: () => get<{ tree: StorageNode[]; generated: number }>(`${API_BASE}/api/topology`),
  sleep: (id: string) =>
    postJSON<PowerResult>(`${API_BASE}/api/disks/${encodeURIComponent(id)}/sleep`),
  offline: (id: string, confirmRaid = false, confirmBusyIO = false) =>
    postJSON<PowerResult>(
      `${API_BASE}/api/disks/${encodeURIComponent(id)}/offline`,
      { confirmRaid, confirmBusyIO },
    ),
  wake: (id: string) =>
    postJSON<PowerResult>(`${API_BASE}/api/disks/${encodeURIComponent(id)}/wake`),
  offlineStream: (
    id: string,
    confirmRaid: boolean,
    confirmBusyIO: boolean,
    onStep: (s: string, d: string) => void,
  ) =>
    streamPower(
      `${API_BASE}/api/disks/${encodeURIComponent(id)}/offline`,
      { confirmRaid, confirmBusyIO },
      onStep,
    ),
  wakeStream: (id: string, onStep: (s: string, d: string) => void) =>
    streamPower(`${API_BASE}/api/disks/${encodeURIComponent(id)}/wake`, {}, onStep),
  powerLog: (limit = 100) =>
    get<{ events: PowerEvent[]; count: number }>(`${API_BASE}/api/power/log?limit=${limit}`),
  clearPowerLog: () => postJSON<{ ok: boolean }>(`${API_BASE}/api/power/log/clear`),
  setNote: (id: string, note: string) =>
    postJSON<Disk>(`${API_BASE}/api/disks/${encodeURIComponent(id)}/note`, { note }),
};

// Relative "x 前" / duration formatting for power events & offline duration.
export function fmtAgo(ts?: number): string {
  if (!ts) return "—";
  const s = Math.max(0, Math.floor(Date.now() / 1000) - ts);
  if (s < 60) return `${s} 秒前`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} 分钟前`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} 小时前`;
  const d = Math.floor(h / 24);
  return d < 30 ? `${d} 天前` : `${Math.floor(d / 30)} 个月前`;
}

export function fmtClock(ts?: number): string {
  if (!ts) return "—";
  const d = new Date(ts * 1000);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

// Absolute HH:MM:SS for "last collected" timestamps.
export function fmtTimeHMS(ts?: number): string {
  if (!ts) return "";
  const d = new Date(ts * 1000);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

// Power-state display helper shared by views. `icon` names a key in the
// view's icon renderer (bolt / moon / off / unknown).
export function powerStateMeta(
  s?: string,
  hibernated?: boolean,
): { text: string; cls: string; icon: "bolt" | "moon" | "off" | "unknown" } {
  if (hibernated || s === "offline") return { text: "已下线", cls: "off", icon: "off" };
  switch (s) {
    case "active": return { text: "运行中", cls: "on", icon: "bolt" };
    case "idle": return { text: "空闲", cls: "idle", icon: "moon" };
    case "standby": return { text: "休眠", cls: "sleep", icon: "moon" };
    default: return { text: "未知", cls: "unk", icon: "unknown" };
  }
}

// ---- display helpers ----

export function fmtBytes(n: number): string {
  if (!n || n <= 0) return "-";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let f = n;
  let i = 0;
  while (f >= 1024 && i < units.length - 1) {
    f /= 1024;
    i++;
  }
  if (i === 0) return `${n} ${units[i]}`;
  // 2 decimals, trailing zeros trimmed (3.60 TB -> 3.6 TB, 931.51 GB kept).
  return `${parseFloat(f.toFixed(2))} ${units[i]}`;
}

// ---- unified temperature thresholds (media-type aware) ----

export interface TempThresholds {
  warn: number;
  crit: number;
}

export function tempThresholds(type: DeviceType): TempThresholds {
  if (type === "NVMe") return { warn: 70, crit: 80 };
  if (type === "SSD") return { warn: 60, crit: 70 };
  return { warn: 50, crit: 60 }; // HDD & unknown: conservative
}

export function tempColorOf(t: number, type: DeviceType): string {
  if (!t || t <= 0) return "var(--unknown)";
  const { warn, crit } = tempThresholds(type);
  if (t >= crit) return "var(--crit)";
  if (t >= warn) return "var(--warn)";
  return "var(--good)";
}

export function fmtHours(h: number): string {
  if (!h || h <= 0) return "-";
  const days = Math.floor(h / 24);
  if (days < 1) return `${h} 小时`;
  if (days < 365) return `${days} 天`;
  const years = Math.floor(days / 365);
  const rem = days % 365;
  return rem > 0 ? `${years} 年 ${rem} 天` : `${years} 年`;
}

export function fmtPercent(p: number): string {
  return p >= 0 ? `${p}%` : "-";
}

export function healthClass(g: HealthGrade): string {
  switch (g) {
    case "优":
      return "good";
    case "良":
      return "fair";
    case "警":
      return "warn";
    case "危":
      return "crit";
    default:
      return "unknown";
  }
}

// ---- SMART attribute glossary (concise Chinese explanations for tooltips) ----

export const ATTR_DESC: Record<number, string> = {
  1: "读取数据时的硬件纠错率，原始值随厂商算法不同，一般只看趋势。",
  3: "主轴电机加速到额定转速所需时间，变长可能意味着轴承老化。",
  4: "主轴电机启动/停止累计次数。",
  5: "因坏块被替换到备用区的扇区总数，出现增长说明盘面开始劣化。",
  7: "磁头定位到目标磁道的错误率，持续增长需警惕机械故障。",
  9: "硬盘累计通电运行的小时数，用于折算使用年限。",
  10: "主轴未能一次启动成功而重试的次数，非零需关注。",
  11: "校准失败后重试的累计次数。",
  12: "硬盘完整通电/断电的累计次数。",
  22: "氦气盘密封状态，异常会导致氦气泄漏失效。",
  166: "SSD 当前等待处理的坏块数量。",
  167: "SSD 备用块池发生的失败总数。",
  169: "SSD 估算的剩余寿命百分比，越低越接近寿命终点。",
  171: "SSD 闪存编程（写入）失败累计次数。",
  172: "SSD 闪存擦除失败累计次数。",
  173: "SSD 闪存平均磨损程度（厂商算法）。",
  174: "未正常断电的累计次数，过多可能损伤固件/数据。",
  175: "闪存编程失败的芯片级计数。",
  177: "SSD 磨损均衡算法的累计计数。",
  180: "剩余可用的保留块总数，持续下降说明备用资源被消耗。",
  183: "SATA 接口协商速率降级的次数。",
  187: "上报给主机的不可校正错误次数。",
  188: "执行命令超时的累计次数。",
  190: "记录到的超出正常范围的温度差值。",
  191: "外部冲击/振动事件的累计计数。",
  192: "断电时磁头归位（紧急缩回）的累计次数。",
  193: "磁头加载/卸载循环累计次数，接近 30 万次需注意磨损。",
  194: "盘内温度传感器读数（°C）。",
  195: "硬件 ECC 纠正的比特错误累计。",
  196: "尝试把数据迁往备用扇区的操作次数。",
  197: "等待重映射的可疑扇区数，是即将出现坏道的强信号。",
  198: "离线扫描发现且无法校正的扇区数。",
  199: "SATA 接口数据传输校验错误次数，多为数据线/背板/供电问题。",
  200: "多区域错误率统计。",
  201: "检测到数据与校验不一致（软式坏道）的计数。",
  202: "厂商自定义的寿命消耗百分比。",
  206: "磁头飞高相关的厂商统计。",
  210: "厂商自定义（常见为启停/寿命相关）。",
  220: "磁头因冲击发生位置偏移的累计次数。",
  222: "磁头在数据区累计停留的时间。",
  223: "磁头加载/卸载重试的累计次数。",
  224: "磁头遇到的机械阻力累计值。",
  225: "累计写入的主机数据量（厂商单位，常见 1GB）。",
  226: "磁头加载区累计停留时间。",
  228: "固件因断电保护而缩回磁头的次数。",
  230: "磁头写入时的偏摆幅度统计。",
  231: "SSD 剩余寿命估算（部分厂商为温度，需按型号区分）。",
  232: "备用块池中已消耗的比例。",
  233: "SSD 磨损指示（部分厂商为 NAND 写入量，单位 GB）。",
  234: "厂商自定义（常见为 NAND 累计擦除/写入统计）。",
  240: "磁头定位的累计时间（厂商算法）。",
  241: "主机累计写入量（厂商单位：LBA 或 GB，按主控区分）。",
  242: "主机累计读取量（厂商单位：LBA 或 GB，按主控区分）。",
  250: "读取时的重试计数。",
  254: "自由落体传感器触发（跌落保护）的次数。",
};

// Concise explanation for a SMART attribute (tooltip); empty when unknown.
export function attrDesc(id: number): string {
  return ATTR_DESC[id] || "";
}
