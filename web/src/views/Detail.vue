<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import {
  api, fmtBytes, fmtHours, fmtPercent, fmtAgo, fmtTimeHMS,
  tempThresholds, tempColorOf, attrDesc,
  type Disk, type Summary, type SmartAttr,
} from "../api";
import HealthBadge from "../components/HealthBadge.vue";
import PowerConfirm from "../components/PowerConfirm.vue";
import PowerBadge from "../components/PowerBadge.vue";
import TempRangeBar from "../components/TempRangeBar.vue";
import SkeletonCards from "../components/SkeletonCards.vue";
import SmartReadConfirm from "../components/SmartReadConfirm.vue";
import {
  IconBack, IconRefresh, IconWarn, IconDisk, IconPowerOff, IconWake,
  IconSearch, IconCheck, IconInfo,
} from "../icons";

const props = defineProps<{ id: string }>();
const emit = defineEmits<{ (e: "back"): void }>();

const disk = ref<Disk | null>(null);
const noteEditing = ref(false);
const noteDraft = ref("");
const noteSaving = ref(false);
const noteError = ref("");
const loading = ref(true);
const error = ref<string | null>(null);
const confirmSmartRead = ref(false);
const smartReadBusy = ref(false);
const smartReadError = ref("");
const now = ref(Math.floor(Date.now() / 1000));
let ageTimer = 0;

async function load(forceRefresh = false) {
  loading.value = true;
  error.value = null;
  try {
    if (forceRefresh) {
      // 只刷新这一块盘（不打扰其他休眠盘）。
      disk.value = await api.refreshDisk(props.id);
    } else {
      // 进入详情读缓存（不唤醒休眠盘）。
      disk.value = await api.disk(props.id);
    }
  } catch (e: any) {
    error.value = e?.message || "无法读取磁盘详情";
  } finally {
    loading.value = false;
  }
}
function requestSmartRefresh() {
	if (disk.value?.smartDeferred) {
		smartReadError.value = "";
		confirmSmartRead.value = true;
		return;
	}
	void load(true);
}

async function readDeferredSmart() {
	if (smartReadBusy.value || !disk.value) return;
	smartReadBusy.value = true;
	smartReadError.value = "";
	try {
		disk.value = await api.refreshDisk(props.id);
		confirmSmartRead.value = false;
	} catch (e: any) {
		smartReadError.value = e?.message || "读取 SMART 失败，可重试或先查看基础信息";
	} finally {
		smartReadBusy.value = false;
	}
}

onMounted(() => {
	void load();
	ageTimer = window.setInterval(() => (now.value = Math.floor(Date.now() / 1000)), 60_000);
});
onUnmounted(() => window.clearInterval(ageTimer));

const d = computed(() => disk.value);

function startEditNote() {
  noteDraft.value = d.value?.userNote ?? "";
  noteError.value = "";
  noteEditing.value = true;
}
function cancelEditNote() {
  noteEditing.value = false;
  noteDraft.value = "";
  noteError.value = "";
}
async function saveNote() {
  if (!d.value || noteSaving.value) return;
  noteSaving.value = true;
  noteError.value = "";
  try {
    disk.value = await api.setNote(d.value.id, noteDraft.value.trim());
    noteEditing.value = false;
  } catch (e: any) {
    noteError.value = e?.message || "保存失败";
  } finally {
    noteSaving.value = false;
  }
}

const lifeColor = computed(() => {
  if (!d.value || d.value.percentUsed < 0) return "var(--acc)";
  const p = d.value.percentUsed;
  if (p >= 95) return "var(--crit)";
  if (p >= 80) return "var(--warn)";
  return "var(--good)";
});

// Classify each health note into ok / info / warn / crit for the risk list.
type NoteLevel = "ok" | "info" | "warn" | "crit";
interface NoteItem { text: string; level: NoteLevel }

const noteItems = computed<NoteItem[]>(() => {
  const health = d.value?.health;
  return (d.value?.healthNotes ?? []).map((text) => {
    let level: NoteLevel = "info";
    if (/各项指标正常/.test(text)) level = "ok";
    else if (/无法读取|不可读|未知/.test(text)) level = "info";
    else if (health === "危") level = "crit";
    else level = "warn";
    return { text, level };
  });
});

const showAttrs = computed(() => (d.value?.attrs?.length ?? 0) > 0);

// --- SMART attribute filter (keyword + only-abnormal) ---
const attrFilter = ref("");
const attrOnlyAbnormal = ref(false);

const filteredAttrs = computed<SmartAttr[]>(() => {
  const all = d.value?.attrs ?? [];
  const q = attrFilter.value.trim().toLowerCase();
  return all.filter((a) => {
    if (attrOnlyAbnormal.value && !(a.critical || a.whenFailed)) return false;
    if (!q) return true;
    return (
      String(a.id).includes(q) ||
      (a.name || "").toLowerCase().includes(q) ||
      (a.cn || "").toLowerCase().includes(q)
    );
  });
});

// --- progress-bar helpers for metric cards ---

function clampPct(v: number): number {
  return Math.max(0, Math.min(100, v));
}

// Temperature bar: scale 0..crit by media type, colored by unified thresholds.
const tempInfo = computed(() => {
  if (!d.value || !d.value.temperatureC) return null;
  const { warn, crit } = tempThresholds(d.value.type);
  const t = d.value.temperatureC;
  const pct = clampPct((t / crit) * 100);
  return { pct, color: tempColorOf(t, d.value.type), warn, crit };
});

// Power-on-hours bar: reference against a 5-year 24/7 span for context.
const pohInfo = computed(() => {
  if (!d.value || !d.value.powerOnHours) return null;
  const ref = 5 * 365 * 24;
  return { pct: clampPct((d.value.powerOnHours / ref) * 100), ref };
});

// Power-cycles bar: reference against 10k cycles.
const cycleInfo = computed(() => {
  if (!d.value || !d.value.powerCycles) return null;
  const ref = 10000;
  return { pct: clampPct((d.value.powerCycles / ref) * 100), ref };
});

// Unsafe-shutdown ratio bar (NVMe): share of power cycles that were unsafe.
const unsafeInfo = computed(() => {
  if (!d.value || d.value.type !== "NVMe") return null;
  const us = d.value.unsafeShutdowns ?? 0;
  if (us <= 0) return null;
  const pc = d.value.powerCycles || us;
  const pct = clampPct((us / pc) * 100);
  const color = pct > 30 ? "var(--warn)" : "var(--acc)";
  return { pct, color, us, pc };
});

// Write-amount basis note (how totalBytesWritten was derived).
const writeBasis = computed(() => {
  if (!d.value) return "";
  return d.value.type === "NVMe"
    ? "按 NVMe data_units_written × 512000 字节计算"
    : "按 SMART Total_LBAs_Written × 512 字节计算";
});

// Available-spare bar (NVMe): spare % vs threshold.
const spareInfo = computed(() => {
  const nv = d.value?.nvme;
  if (!nv) return null;
  const pct = clampPct(nv.availableSpare);
  const color = nv.availableSpare <= nv.availableSpareThresh ? "var(--crit)" : "var(--good)";
  return { pct, color };
});

// NVMe SMART table rows.
const nvmeRows = computed(() => {
  const nv = d.value?.nvme;
  if (!nv) return [] as { k: string; v: string; warn?: boolean }[];
  const num = (n: number) => (n || n === 0 ? n.toLocaleString() : "-");
  return [
    { k: "严重警告 (Critical Warning)", v: String(nv.criticalWarning), warn: nv.criticalWarning !== 0 },
    { k: "可用备用块 (Available Spare)", v: `${nv.availableSpare}%（阈值 ${nv.availableSpareThresh}%）`, warn: nv.availableSpare <= nv.availableSpareThresh },
    { k: "已用寿命百分比 (Percentage Used)", v: `${nv.percentageUsed}%`, warn: nv.percentageUsed >= 95 },
    { k: "读取数据单元 (Data Units Read)", v: `${num(nv.dataUnitsRead)}（约 ${fmtBytes(nv.dataUnitsRead * 512000)}）` },
    { k: "写入数据单元 (Data Units Written)", v: `${num(nv.dataUnitsWritten)}（约 ${fmtBytes(nv.dataUnitsWritten * 512000)}）` },
    { k: "主机读命令 (Host Read Commands)", v: num(nv.hostReadCommands) },
    { k: "主机写命令 (Host Write Commands)", v: num(nv.hostWriteCommands) },
    { k: "控制器繁忙时间 (Controller Busy)", v: `${num(nv.controllerBusyTime)} 分钟` },
    { k: "通电次数 (Power Cycles)", v: num(nv.powerCycles) },
    { k: "通电时间 (Power On Hours)", v: `${num(nv.powerOnHours)} 小时` },
    { k: "异常断电 (Unsafe Shutdowns)", v: num(nv.unsafeShutdowns), warn: nv.unsafeShutdowns > 0 && nv.powerCycles > 0 && nv.unsafeShutdowns / nv.powerCycles > 0.3 },
    { k: "介质错误 (Media Errors)", v: num(nv.mediaErrors), warn: nv.mediaErrors > 0 },
    { k: "错误日志条目 (Error Log Entries)", v: num(nv.numErrLogEntries), warn: nv.numErrLogEntries > 0 },
    { k: "警告温度时间 (Warning Temp Time)", v: `${num(nv.warningTempTime)} 分钟`, warn: nv.warningTempTime > 0 },
    { k: "临界温度时间 (Critical Temp Time)", v: `${num(nv.criticalTempTime)} 分钟`, warn: nv.criticalTempTime > 0 },
  ];
});

const basicRows = computed(() => {
  if (!d.value) return [] as { k: string; v: string }[];
  const x = d.value;
  const rows = [
    { k: "型号", v: x.model || "未知" },
    { k: "序列号", v: x.serial || "未知" },
    { k: "固件版本", v: x.firmware || "未知" },
    { k: "容量", v: fmtBytes(x.capacity) },
    { k: "接口类型", v: x.interface },
    { k: "介质类型", v: x.type === "NVMe" ? "SSD" : x.type },
  ];
  if (x.formFactor) rows.push({ k: "外形规格", v: x.formFactor });
  if (x.rotationRate && x.rotationRate > 0) rows.push({ k: "转速", v: `${x.rotationRate} RPM` });
  if (x.logicalSectorSize || x.physicalSectorSize) {
    rows.push({ k: "扇区大小", v: `逻辑 ${x.logicalSectorSize || "-"} B / 物理 ${x.physicalSectorSize || "-"} B` });
  }
  if (x.wwn) rows.push({ k: "WWN / EUI-64", v: x.wwn });
  if (x.controller) rows.push({ k: "主控", v: x.controller });
  if (x.powerState) rows.push({ k: "电源状态", v: powerStateText(x.powerState) });
  if (x.apmLevel) rows.push({ k: "APM 电源管理", v: `${x.apmLevel}` });
  if (x.cacheSizeBytes) rows.push({ k: "缓存大小", v: fmtBytes(x.cacheSizeBytes) });
  if (x.ncqDepth) rows.push({ k: "NCQ 深度", v: String(x.ncqDepth) });
  if (x.type === "SSD" || x.type === "NVMe") {
    rows.push({ k: "TRIM 支持", v: x.trimSupported ? "支持" : "不支持/未知" });
  }
  rows.push(
    { k: "生产日期", v: x.manufactureDate || "未知" },
    { k: "SMART 支持", v: x.smartSupported ? (x.smartEnabled ? "已启用" : "已禁用") : "不支持" },
    { k: "SMART 整体状态", v: x.smartAvail ? (x.smartOk ? "通过 (PASSED)" : "未通过 (FAILING)") : "不可读" },
  );
  return rows;
});

const showSelfTests = computed(() => (d.value?.selfTests?.length ?? 0) > 0);
const showErrorLog = computed(() => (d.value?.ataErrorLog?.length ?? 0) > 0);
const showDevStats = computed(() => (d.value?.devStats?.length ?? 0) > 0);
const showPartitions = computed(() => (d.value?.partitions?.length ?? 0) > 0);

const showTempHistory = computed(() => {
  const t = d.value?.tempHistory;
  return t && (t.max > 0 || t.min > 0 || t.current > 0);
});

const showPhy = computed(() => (d.value?.sataPhy?.length ?? 0) > 0);

const phyHasErrors = computed(() =>
  (d.value?.sataPhy ?? []).some((e) => e.value > 0),
);

const showBtrfs = computed(() => {
  const b = d.value?.btrfsErrs;
  return b != null;
});

const btrfsTotal = computed(() => {
  const b = d.value?.btrfsErrs;
  if (!b) return 0;
  return b.writeIOErrs + b.readIOErrs + b.flushIOErrs + b.corruptionErrs + b.generationErrs;
});

function powerStateText(s?: string): string {
  switch (s) {
    case "active": return "运行中";
    case "idle": return "空闲";
    case "standby": return "休眠";
    case "offline": return "已下线";
    default: return "未知";
  }
}

// --- power control ---
const pwModal = ref<"offline" | "wake" | null>(null);
const pwToast = ref<{ ok: boolean; msg: string } | null>(null);

// Offline button tooltip: spell out the blast radius of the action.
const offlineTip = computed(() => {
  const x = d.value;
  if (!x) return "";
  if (!x.canOffline) return x.noOfflineReason || "不可下线";
  if (x.needsRaidConfirm) return "内核级隔离：卸载卷 → 停转 → device offline（此盘是 RAID 成员，需二次确认）";
  return "内核级隔离：卸载卷 → 停转 → device offline，飞牛无法再唤醒";
});

// PowerConfirm needs a Summary shape; project the detail disk onto it.
const pwSummary = computed<Summary | null>(() => {
  const x = d.value;
  if (!x) return null;
  return {
    id: x.id, name: x.name, model: x.model, serial: x.serial,
    capacity: x.capacity, usedCapacity: x.usedCapacity, usagePercent: x.usagePercent,
    interface: x.interface, type: x.type, temperatureC: x.temperatureC,
    health: x.health, percentUsed: x.percentUsed, smartAvail: x.smartAvail,
    smartDeferred: x.smartDeferred,
    volumePaths: x.volumePaths, powerState: x.powerState, hibernated: x.hibernated,
    canSleep: x.canSleep, canOffline: x.canOffline, canWake: x.canWake,
    noSleepReason: x.noSleepReason, noOfflineReason: x.noOfflineReason,
    needsRaidConfirm: x.needsRaidConfirm,
  };
});
async function onPwDone(ok: boolean, message: string) {
  // PowerConfirm keeps itself open on failure; on success we close + reload.
  if (ok) {
    pwModal.value = null;
    pwToast.value = { ok, msg: message };
    setTimeout(() => (pwToast.value = null), 5000);
    await load(); // power operation already patched the cached disk state
  }
}

const collectedText = computed(() => {
  const t = d.value?.collectedAt;
  if (!t) return "";
  return `${fmtAgo(t)} · ${fmtTimeHMS(t)}`;
});
const dataIsStale = computed(() => {
	const t = d.value?.collectedAt;
	return !!t && now.value - t > 24 * 60 * 60;
});
</script>

<template>
  <div>
    <!-- loading -->
    <SkeletonCards v-if="loading" kind="detail" />

    <!-- error -->
    <div v-else-if="error || !d" class="state-box">
      <div class="icon"><IconWarn :size="34" /></div>
      <h3>无法加载磁盘详情</h3>
      <p>{{ error || "未找到该磁盘" }}</p>
      <button class="btn" @click="emit('back')"><IconBack :size="15" /> 返回列表</button>
      <button class="btn primary" style="margin-left:10px" @click="() => load()">重试</button>
    </div>

    <template v-else>
      <!-- header -->
      <div class="detail-head">
        <button class="btn ghost back" @click="emit('back')"><IconBack :size="15" /> 返回</button>
        <div class="detail-title">
          <h1>
            {{ d.model || "未知型号" }}
            <HealthBadge :grade="d.health" />
            <PowerBadge v-if="d.powerState || d.hibernated" :power-state="d.powerState" :hibernated="d.hibernated" />
          </h1>
          <div class="detail-meta">
            <span>设备 <b>{{ d.name }}</b></span>
            <span>序列号 <b>{{ d.serial || "—" }}</b></span>
            <span>接口 <b>{{ d.interface }}</b></span>
            <span v-if="d.bridge">桥接 <b>{{ d.bridge.chip }}</b></span>
            <span v-if="collectedText" class="gen-time" :class="{ stale: dataIsStale }" title="上次采集时间">上次采集 {{ collectedText }}</span>
            <span v-if="dataIsStale" class="stale-label">数据已超过 24 小时</span>
          </div>
        </div>
        <div class="detail-ops">
          <template v-if="!d.hibernated">
            <button
              class="btn danger" :disabled="!d.canOffline"
              :title="offlineTip"
              @click="pwModal = 'offline'"
            ><IconPowerOff :size="14" /> 深度下线</button>
          </template>
          <button v-else class="btn primary" title="让设备恢复 running 并重扫总线；之后需在飞牛硬盘设置里点「挂载硬盘」" @click="pwModal = 'wake'"><IconWake :size="14" /> 唤醒</button>
          <button class="btn" title="只重新采集这一块盘（可能唤醒休眠中的磁盘）" @click="requestSmartRefresh"><IconRefresh :size="15" /> 刷新</button>
        </div>
      </div>

      <div v-if="d.smartDeferred" class="deferred-banner">
        <IconWarn :size="15" />
        <span>为避免唤醒休眠盘，SMART 数据暂未读取；当前显示的是基础信息。</span>
        <button class="btn sm" @click="confirmSmartRead = true">读取 SMART</button>
      </div>

      <div v-if="pwToast" class="pw-note" :class="pwToast.ok ? 'ok' : 'err'">{{ pwToast.msg }}</div>

      <!-- metric cards: usage intensity group -->
      <div class="metric-group">使用强度</div>
      <div class="cards">
        <div class="metric">
          <div class="k">容量</div>
          <div class="v" style="font-size:19px">{{ fmtBytes(d.capacity) }}</div>
          <div class="lifebar" v-if="d.usagePercent >= 0">
            <span :style="{ width: d.usagePercent + '%', background: d.usagePercent >= 90 ? 'var(--crit)' : d.usagePercent >= 75 ? 'var(--warn)' : 'var(--acc)' }"></span>
          </div>
          <div class="foot" v-if="d.usagePercent >= 0">已用 {{ fmtBytes(d.usedCapacity) }}（{{ d.usagePercent }}%）</div>
        </div>
        <div class="metric">
          <div class="k">当前温度</div>
          <div class="v">{{ d.temperatureC || "-" }}<span class="unit" v-if="d.temperatureC">°C</span></div>
          <div class="lifebar" v-if="tempInfo">
            <span :style="{ width: tempInfo.pct + '%', background: tempInfo.color }"></span>
          </div>
          <div class="foot" v-if="tempInfo">警戒线 {{ tempInfo.warn }}°C / 临界 {{ tempInfo.crit }}°C</div>
        </div>
        <div class="metric">
          <div class="k">通电时间</div>
          <div class="v" style="font-size:19px">{{ fmtHours(d.powerOnHours) }}</div>
          <div class="lifebar" v-if="pohInfo">
            <span :style="{ width: pohInfo.pct + '%', background: 'var(--acc)' }"></span>
          </div>
          <div class="foot" v-if="d.powerOnHours > 0">共 {{ d.powerOnHours.toLocaleString() }} 小时<span v-if="d.usageYears > 0"> · 约 {{ d.usageYears.toFixed(1) }} 年</span></div>
        </div>
        <div class="metric">
          <div class="k">通电次数</div>
          <div class="v">{{ d.powerCycles || "-" }}</div>
          <div class="lifebar" v-if="cycleInfo">
            <span :style="{ width: cycleInfo.pct + '%', background: 'var(--acc)' }"></span>
          </div>
          <div class="foot" v-if="cycleInfo">参考上限 10000 次</div>
        </div>
        <div class="metric" v-if="d.loadCycleCount && d.loadCycleCount > 0">
          <div class="k">磁头加载/卸载</div>
          <div class="v" style="font-size:19px">{{ d.loadCycleCount.toLocaleString() }}</div>
          <div class="foot" :style="{ color: d.loadCycleCount > 300000 ? 'var(--warn)' : 'var(--ink-3)' }">
            {{ d.loadCycleCount > 300000 ? "次数偏高，注意磁头磨损" : "次" }}
          </div>
        </div>
      </div>

      <!-- metric cards: wear / lifespan group -->
      <template v-if="d.percentUsed >= 0 || spareInfo || d.totalBytesWritten > 0 || unsafeInfo">
        <div class="metric-group">寿命与磨损</div>
        <div class="cards">
          <div class="metric" v-if="d.percentUsed >= 0">
          <div class="k">剩余写入寿命</div>
          <div class="v">{{ fmtPercent(d.percentLifeRemain) }}</div>
          <div class="lifebar">
            <span :style="{ width: Math.min(d.percentUsed, 100) + '%', background: lifeColor }"></span>
          </div>
          <div class="foot">已用 {{ fmtPercent(d.percentUsed) }}</div>
          <div class="foot" v-if="d.lifeSource" style="color:var(--ink-3)">{{ d.lifeSource }}</div>
          </div>
          <div class="metric" v-if="spareInfo">
          <div class="k">可用备用块</div>
          <div class="v">{{ d.nvme!.availableSpare }}<span class="unit">%</span></div>
          <div class="lifebar">
            <span :style="{ width: spareInfo.pct + '%', background: spareInfo.color }"></span>
          </div>
          <div class="foot">阈值 {{ d.nvme!.availableSpareThresh }}%</div>
          </div>
          <div class="metric" v-if="d.totalBytesWritten > 0">
          <div class="k">累计写入量</div>
          <div class="v" style="font-size:19px">{{ fmtBytes(d.totalBytesWritten) }}</div>
          <div class="foot">{{ writeBasis }}</div>
          </div>
          <div class="metric" v-if="unsafeInfo">
          <div class="k">异常断电</div>
          <div class="v">{{ unsafeInfo.us }}</div>
          <div class="lifebar">
            <span :style="{ width: unsafeInfo.pct + '%', background: unsafeInfo.color }"></span>
          </div>
          <div class="foot">占通电周期 {{ unsafeInfo.pct.toFixed(1) }}%</div>
          </div>
        </div>
      </template>

      <!-- health notes -->
      <div class="section" v-if="noteItems.length">
        <div class="section-h">
          <h3>健康评估</h3>
          <HealthBadge :grade="d.health" />
        </div>
        <div class="section-b">
          <ul class="notes">
            <li v-for="(n, i) in noteItems" :key="i" :class="'note-' + n.level">
              <span class="n-ico">
                <IconCheck v-if="n.level === 'ok'" :size="13" />
                <IconWarn v-else-if="n.level === 'warn' || n.level === 'crit'" :size="13" />
                <IconInfo v-else :size="13" />
              </span>
              <span>{{ n.text }}</span>
            </li>
          </ul>
        </div>
      </div>

      <!-- basic info -->
      <div class="section">
        <div class="section-h">
          <h3>基本信息</h3>
          <button v-if="!noteEditing" class="btn sm" @click="startEditNote" title="添加或修改本盘备注（按序列号持久化，重启/换槽不丢失）">
            {{ d.userNote ? "编辑备注" : "添加备注" }}
          </button>
        </div>
        <div v-if="noteEditing" class="section-b">
          <div class="note-editor">
            <textarea
              v-model="noteDraft" rows="2" maxlength="200"
              placeholder="例如：2025-10 购入，冷备仓库盘；或：柜内 3 号仓位"
            ></textarea>
            <div class="note-actions">
              <span class="note-count" :class="{ over: noteDraft.length >= 200 }">{{ noteDraft.length }}/200</span>
              <button class="btn sm" :disabled="noteSaving" @click="cancelEditNote">取消</button>
              <button class="btn sm primary" :disabled="noteSaving" @click="saveNote">
                {{ noteSaving ? "保存中…" : "保存" }}
              </button>
            </div>
            <div v-if="noteError" class="note-err">{{ noteError }}</div>
          </div>
        </div>
        <div v-else-if="d.userNote" class="section-b">
          <div class="user-note">
            <IconInfo :size="13" />
            <span>{{ d.userNote }}</span>
          </div>
        </div>
        <div class="section-b pad0">
          <table class="attrs">
            <tbody>
              <tr v-for="r in basicRows" :key="r.k">
                <td class="name" style="width:180px">{{ r.k }}</td>
                <td>{{ r.v }}</td>
              </tr>
              <template v-if="d.bridge">
                <tr>
                  <td class="name">USB 桥接芯片</td>
                  <td>{{ d.bridge.chip || "未知" }}</td>
                </tr>
                <tr v-if="d.bridge.vendorId">
                  <td class="name">桥接 VID:PID</td>
                  <td>{{ d.bridge.vendorId }}:{{ d.bridge.productId }}</td>
                </tr>
                <tr v-if="d.bridge.passthrough">
                  <td class="name">透传协议</td>
                  <td>{{ d.bridge.passthrough }}</td>
                </tr>
              </template>
              <template v-if="d.link">
                <tr>
                  <td class="name">链路速率</td>
                  <td>
                    当前 {{ d.link.negotiated || "-" }} / 设备最大 {{ d.link.deviceMax || "-" }}
                    <span v-if="d.link.downgraded" class="tag" style="color:var(--warn);border-color:rgba(230,162,60,.4);background:rgba(230,162,60,.1);margin-left:6px">降速</span>
                  </td>
                </tr>
                <tr v-if="d.link.note">
                  <td class="name">链路说明</td>
                  <td style="color:var(--ink-3)">{{ d.link.note }}</td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </div>

      <!-- storage chain -->
      <div class="section" v-if="d.chain && d.chain.length">
        <div class="section-h">
          <h3>存储链</h3>
          <span class="hint" style="font-size:12px;color:var(--ink-3)">{{ d.role }}</span>
        </div>
        <div class="section-b">
          <div class="chain">
            <div class="chain-node disk">
              <div class="chain-name">{{ d.id }}</div>
              <div class="chain-type">物理盘</div>
            </div>
            <template v-for="(h, i) in d.chain" :key="i">
              <div class="chain-arrow">→</div>
              <div class="chain-node">
                <div class="chain-name">{{ h.name }}</div>
                <div class="chain-type">{{ h.type }}<span v-if="h.label"> · {{ h.label }}</span></div>
                <div class="chain-sub" v-if="h.mountPoint">{{ h.mountPoint }}</div>
                <div class="chain-sub" v-else-if="h.size">{{ fmtBytes(h.size) }}</div>
              </div>
            </template>
          </div>
        </div>
      </div>

      <!-- D: partitions -->
      <div class="section" v-if="showPartitions">
        <div class="section-h">
          <h3>分区 / 文件系统</h3>
          <span class="hint" style="font-size:12px;color:var(--ink-3)">共 {{ d.partitions!.length }} 个分区</span>
        </div>
        <div class="section-b pad0" style="overflow-x:auto">
          <table class="attrs">
            <thead>
              <tr><th>分区</th><th>大小</th><th>文件系统</th><th>挂载点</th><th>已用</th><th>使用率</th></tr>
            </thead>
            <tbody>
              <tr v-for="p in d.partitions" :key="p.name">
                <td class="name">{{ p.name }}</td>
                <td>{{ fmtBytes(p.size) }}</td>
                <td>{{ p.fsType || "-" }}<span v-if="p.label" style="color:var(--ink-3)"> ({{ p.label }})</span></td>
                <td>{{ p.mountPoint || "-" }}</td>
                <td>{{ p.used > 0 ? fmtBytes(p.used) : "-" }}</td>
                <td>
                  <template v-if="p.usePercent >= 0">
                    <span :style="{ color: p.usePercent >= 90 ? 'var(--crit)' : p.usePercent >= 75 ? 'var(--warn)' : 'inherit' }">{{ p.usePercent }}%</span>
                    <div class="minibar" style="margin-top:4px">
                      <span :style="{ width: Math.min(p.usePercent, 100) + '%', background: p.usePercent >= 90 ? 'var(--crit)' : p.usePercent >= 75 ? 'var(--warn)' : 'var(--acc)' }"></span>
                    </div>
                  </template>
                  <span v-else>-</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- SMART attributes -->
      <div class="section" v-if="showAttrs">
        <div class="section-h">
          <h3>SMART 属性</h3>
          <div class="attr-tools">
            <label class="attr-search">
              <IconSearch :size="13" />
              <input v-model="attrFilter" type="text" placeholder="筛选 ID / 名称" />
            </label>
            <label class="attr-abn">
              <input type="checkbox" v-model="attrOnlyAbnormal" />
              只看异常
            </label>
            <span class="hint" style="font-size:12px;color:var(--ink-3)">
              {{ filteredAttrs.length }} / {{ d.attrs!.length }} 项
            </span>
          </div>
        </div>
        <div class="section-b pad0" style="overflow-x:auto">
          <table class="attrs">
            <thead>
              <tr>
                <th>ID</th><th>属性(中文)</th><th>当前值</th><th>最差值</th><th>阈值</th><th>原始值</th><th>状态</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="a in filteredAttrs" :key="a.id" :class="{ critical: a.critical || a.whenFailed }">
                <td>{{ a.id }}</td>
                <td class="name" :title="attrDesc(a.id)">
                  {{ a.cn || a.name }}<span v-if="a.important" class="star" title="关键健康指标">*</span>
                  <div v-if="a.cn" class="en-sub">{{ a.name }}</div>
                </td>
                <td>{{ a.value }}</td>
                <td>{{ a.worst }}</td>
                <td>{{ a.thresh }}</td>
                <td>{{ a.raw || a.rawValue }}</td>
                <td>{{ a.whenFailed ? a.whenFailed : (a.critical ? "关注" : "正常") }}</td>
              </tr>
            </tbody>
          </table>
          <div v-if="!filteredAttrs.length" class="attr-empty">没有匹配的属性</div>
        </div>
      </div>

      <!-- NVMe SMART / Health log -->
      <div class="section" v-if="d.type === 'NVMe' && nvmeRows.length">
        <div class="section-h">
          <h3>NVMe SMART / 健康信息日志</h3>
          <span class="hint" style="font-size:12px;color:var(--ink-3)">共 {{ nvmeRows.length }} 项</span>
        </div>
        <div class="section-b pad0" style="overflow-x:auto">
          <table class="attrs">
            <thead>
              <tr><th style="width:46%">项目</th><th>值</th></tr>
            </thead>
            <tbody>
              <tr v-for="r in nvmeRows" :key="r.k" :class="{ critical: r.warn }">
                <td class="name">{{ r.k }}</td>
                <td>{{ r.v }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- no SMART -->
      <div v-else-if="!d.smartAvail" class="section">
        <div class="section-b">
          <div class="state-box" style="padding:30px 20px">
            <div class="icon"><IconDisk :size="30" /></div>
            <h3>无法读取 SMART 数据</h3>
            <p>该设备可能是通过不支持透传的 USB 桥接连接，或 SMART 被禁用。基本信息仍可显示。</p>
          </div>
        </div>
      </div>

      <!-- SCT temperature history -->
      <div class="section" v-if="showTempHistory">
        <div class="section-h"><h3>温度历史（盘内记录）</h3></div>
        <div class="section-b">
          <TempRangeBar :history="d.tempHistory!" :type="d.type" />
          <table class="attrs" v-if="d.tempHistory!.overTempMin > 0 || d.tempHistory!.underTempMin > 0" style="margin-top:10px">
            <tbody>
              <tr v-if="d.tempHistory!.overTempMin > 0">
                <td class="name" style="width:220px">超温累计时间</td>
                <td style="color:var(--warn)">{{ d.tempHistory!.overTempMin }} 分钟</td>
              </tr>
              <tr v-if="d.tempHistory!.underTempMin > 0">
                <td class="name">低温累计时间</td>
                <td>{{ d.tempHistory!.underTempMin }} 分钟</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- SATA PHY link errors -->
      <div class="section" v-if="showPhy">
        <div class="section-h">
          <h3>SATA 链路错误计数</h3>
          <span class="hint" style="font-size:12px" :style="{ color: phyHasErrors ? 'var(--warn)' : 'var(--good)' }">
            {{ phyHasErrors ? "存在错误，检查线缆/背板" : "链路正常" }}
          </span>
        </div>
        <div class="section-b pad0" style="overflow-x:auto">
          <table class="attrs">
            <thead><tr><th>事件</th><th>中文</th><th>计数</th></tr></thead>
            <tbody>
              <tr v-for="(e, i) in d.sataPhy" :key="i" :class="{ critical: e.value > 0 }">
                <td>{{ e.name }}</td>
                <td class="cn">{{ e.cn || "-" }}</td>
                <td>{{ e.value }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- btrfs pool errors -->
      <div class="section" v-if="showBtrfs">
        <div class="section-h">
          <h3>btrfs 存储池错误计数</h3>
          <span class="hint" style="font-size:12px" :style="{ color: btrfsTotal > 0 ? 'var(--warn)' : 'var(--good)' }">
            卷 {{ d.btrfsMount }} · {{ btrfsTotal > 0 ? "存在错误" : "无错误" }}
          </span>
        </div>
        <div class="section-b pad0" style="overflow-x:auto">
          <table class="attrs">
            <thead><tr><th>类型</th><th>说明</th><th>计数</th></tr></thead>
            <tbody>
              <tr :class="{ critical: d.btrfsErrs!.writeIOErrs > 0 }">
                <td>write_io_errs</td><td class="cn">写入 IO 错误</td><td>{{ d.btrfsErrs!.writeIOErrs }}</td>
              </tr>
              <tr :class="{ critical: d.btrfsErrs!.readIOErrs > 0 }">
                <td>read_io_errs</td><td class="cn">读取 IO 错误</td><td>{{ d.btrfsErrs!.readIOErrs }}</td>
              </tr>
              <tr :class="{ critical: d.btrfsErrs!.flushIOErrs > 0 }">
                <td>flush_io_errs</td><td class="cn">刷写 IO 错误</td><td>{{ d.btrfsErrs!.flushIOErrs }}</td>
              </tr>
              <tr :class="{ critical: d.btrfsErrs!.corruptionErrs > 0 }">
                <td>corruption_errs</td><td class="cn">校验和/数据损坏错误</td><td>{{ d.btrfsErrs!.corruptionErrs }}</td>
              </tr>
              <tr :class="{ critical: d.btrfsErrs!.generationErrs > 0 }">
                <td>generation_errs</td><td class="cn">代际校验错误</td><td>{{ d.btrfsErrs!.generationErrs }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- A: self-test history -->
      <div class="section" v-if="showSelfTests">
        <div class="section-h">
          <h3>自检历史</h3>
          <span class="hint" style="font-size:12px;color:var(--ink-3)">共 {{ d.selfTests!.length }} 次</span>
        </div>
        <div class="section-b pad0" style="overflow-x:auto">
          <table class="attrs">
            <thead>
              <tr><th>类型</th><th>结果</th><th>运行时(通电小时)</th><th>首个错误 LBA</th></tr>
            </thead>
            <tbody>
              <tr v-for="(t, i) in d.selfTests" :key="i">
                <td class="name">{{ t.typeCn || t.type }}<span v-if="t.typeCn" class="en-sub"> {{ t.type }}</span></td>
                <td :style="{ color: t.passed ? 'var(--good)' : 'var(--warn)' }">{{ t.statusCn || t.status }}</td>
                <td>{{ t.lifetimeH }}</td>
                <td>{{ t.lbaFirstErr || "-" }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- A: ATA error log -->
      <div class="section" v-if="showErrorLog">
        <div class="section-h">
          <h3>SMART 错误日志</h3>
          <span class="hint" style="font-size:12px;color:var(--warn)">共 {{ d.ataErrorLog!.length }} 条</span>
        </div>
        <div class="section-b pad0" style="overflow-x:auto">
          <table class="attrs">
            <thead>
              <tr><th>#</th><th>描述</th><th>发生时(通电小时)</th></tr>
            </thead>
            <tbody>
              <tr v-for="e in d.ataErrorLog" :key="e.errorNum" class="critical">
                <td>{{ e.errorNum }}</td>
                <td class="name">{{ e.description || "-" }}</td>
                <td>{{ e.lifetimeH }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- A: device statistics -->
      <div class="section" v-if="showDevStats">
        <div class="section-h">
          <h3>设备统计</h3>
          <span class="hint" style="font-size:12px;color:var(--ink-3)">共 {{ d.devStats!.length }} 项</span>
        </div>
        <div class="section-b pad0" style="overflow-x:auto">
          <table class="attrs">
            <thead>
              <tr><th style="width:60%">项目</th><th>值</th></tr>
            </thead>
            <tbody>
              <tr v-for="(s, i) in d.devStats" :key="i" :class="{ critical: s.flagged }">
                <td class="name">{{ s.cn || s.name }}<span v-if="s.cn" class="en-sub"> {{ s.name }}</span></td>
                <td>{{ s.value }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="foot-note">SMART 采集只读 · 电源控制按确认执行</div>
    </template>

    <PowerConfirm
      v-if="pwModal && pwSummary"
      :disk="pwSummary"
      :action="pwModal"
      @close="pwModal = null"
      @done="onPwDone"
    />
    <SmartReadConfirm
        v-if="confirmSmartRead && d && d.smartDeferred"
        :disks="d ? [d] : []"
        :busy="smartReadBusy"
        :progress="smartReadBusy ? `正在读取：${d.id}` : ''"
        :error="smartReadError"
        @accept="readDeferredSmart"
        @skip="confirmSmartRead = false"
    />
  </div>
</template>

<style scoped>
.user-note {
  display: flex; gap: 8px; align-items: flex-start;
  color: var(--ink-2); font-size: 13px; line-height: 1.6;
}
.user-note svg { color: var(--acc); flex: none; margin-top: 3px; }
.note-editor { display: flex; flex-direction: column; gap: 8px; }
.note-editor textarea {
  width: 100%; box-sizing: border-box; resize: vertical; min-height: 56px;
  background: var(--panel-3); border: 1px solid var(--line); border-radius: 8px;
  color: var(--ink); font-family: var(--font-sans); font-size: 13px; line-height: 1.6;
  padding: 9px 11px;
}
.note-editor textarea:focus { outline: none; border-color: var(--acc); }
.note-actions { display: flex; align-items: center; gap: 8px; justify-content: flex-end; }
.note-count { margin-right: auto; color: var(--ink-3); font-size: 11.5px; font-family: var(--font-mono); }
.note-count.over { color: var(--crit); }
.note-err { color: var(--crit); font-size: 12px; }
.btn.sm { padding: 4px 10px; font-size: 12px; border-radius: 7px; }
.btn.sm.primary { background: #4fe0b0; border-color: #4fe0b0; color: #053326; font-weight: 700; }
.btn.sm.primary:hover:not(:disabled) { background: #6bf0c2; }
.detail-ops { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.btn.danger { color: var(--crit); border-color: rgba(236,91,82,.4); background: rgba(236,91,82,.1); }
.btn.danger:hover:not(:disabled) { background: rgba(236,91,82,.18); border-color: var(--crit); }
.pw-note { margin-bottom: 16px; padding: 11px 14px; border-radius: 9px; font-size: 13px; font-weight: 600; }
.pw-note.ok { background: rgba(67,201,117,.12); border: 1px solid rgba(67,201,117,.4); color: var(--good); }
.pw-note.err { background: rgba(236,91,82,.12); border: 1px solid rgba(236,91,82,.4); color: var(--crit); }
</style>
