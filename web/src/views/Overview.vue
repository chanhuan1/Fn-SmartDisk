<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import {
  api, fmtBytes, fmtAgo, fmtTimeHMS, tempColorOf,
  type Overview, type Summary, type HealthGrade,
} from "../api";
import HealthBadge from "../components/HealthBadge.vue";
import DiskTypeIcon from "../components/DiskTypeIcon.vue";
import PowerBadge from "../components/PowerBadge.vue";
import SkeletonCards from "../components/SkeletonCards.vue";
import SmartReadConfirm from "../components/SmartReadConfirm.vue";
import {
  IconDisk, IconRefresh, IconWarn, IconCheck,
} from "../icons";

const emit = defineEmits<{ (e: "open", id: string): void }>();

const data = ref<Overview | null>(null);
const loading = ref(true);
const refreshing = ref(false);
const fetchError = ref<string | null>(null);
const confirmSmartRead = ref(false);
const smartReadBusy = ref(false);
const smartReadProgress = ref("");
const smartReadError = ref("");
const refreshAfterSmartSkip = ref(false);
const now = ref(Math.floor(Date.now() / 1000));
let ageTimer = 0;

async function load(isRefresh = false) {
  if (isRefresh) refreshing.value = true;
  else loading.value = true;
  fetchError.value = null;
  try {
    // 首次进入读缓存（不唤醒休眠盘）；点刷新才强制重新采集（可能唤醒）。
    data.value = isRefresh ? await api.refresh() : await api.overview();
    if (isRefresh) confirmSmartRead.value = false;
    else if (data.value.disks.some((disk) => disk.smartDeferred)) confirmSmartRead.value = true;
  } catch (e: any) {
    fetchError.value = e?.message || "无法连接后端服务";
  } finally {
    loading.value = false;
    refreshing.value = false;
  }
}
onMounted(() => {
  void load();
  ageTimer = window.setInterval(() => (now.value = Math.floor(Date.now() / 1000)), 60_000);
});
onUnmounted(() => window.clearInterval(ageTimer));

// Sort disks by volume number (vol1, vol2, ...); disks without a volume go last.
function volKey(d: Summary): number {
  const vols = d.volumePaths ?? [];
  let best = Number.MAX_SAFE_INTEGER;
  for (const v of vols) {
    const m = v.match(/\/vol(\d+)/);
    if (m) best = Math.min(best, parseInt(m[1], 10));
  }
  return best;
}

const disks = computed<Summary[]>(() => {
  const list = (data.value?.disks ?? []).slice();
  list.sort((a, b) => {
    const ka = volKey(a);
    const kb = volKey(b);
    if (ka !== kb) return ka - kb;
    return a.id.localeCompare(b.id, undefined, { numeric: true });
  });
  return list;
});

const stats = computed(() => {
  const list = disks.value;
  const total = list.length;
  const healthy = list.filter((d) => d.health === "优" || d.health === "良").length;
  const attention = list.filter((d) => d.health === "警" || d.health === "危").length;
  const totalCap = list.reduce((s, d) => s + (d.capacity || 0), 0);
  const totalUsed = list.reduce((s, d) => s + (d.usedCapacity || 0), 0);
  return { total, healthy, attention, totalCap, totalUsed };
});

const generatedRel = computed(() => {
  if (!data.value?.generated) return "";
  return fmtAgo(data.value.generated);
});
const generatedAbs = computed(() => {
  if (!data.value?.generated) return "";
  return fmtTimeHMS(data.value.generated);
});
const stale = computed(() => !!data.value?.generated && now.value - data.value.generated > 24 * 60 * 60);
const deferredDisks = computed(() => disks.value.filter((d) => d.smartDeferred));

// Inline note editing on overview cards. Editing state is per-disk so
// multiple cards can be edited independently without modal juggling.
const editingNote = ref<string | null>(null);
const noteDraft = ref("");
const noteSaving = ref(false);
const noteError = ref("");

function startNoteEdit(d: Summary) {
  editingNote.value = d.id;
  noteDraft.value = d.userNote ?? "";
  noteError.value = "";
}
function cancelNoteEdit() {
  editingNote.value = null;
  noteDraft.value = "";
  noteError.value = "";
}
async function saveNote(d: Summary) {
  if (noteSaving.value) return;
  noteSaving.value = true;
  noteError.value = "";
  try {
    const updated = await api.setNote(d.id, noteDraft.value.trim());
    if (data.value) {
      const list = data.value.disks;
      for (let i = 0; i < list.length; i++) {
        if (list[i].id === d.id) list[i] = { ...list[i], userNote: updated.userNote ?? "" };
      }
    }
    editingNote.value = null;
    noteDraft.value = "";
  } catch (e: any) {
    noteError.value = e?.message || "保存失败";
  } finally {
    noteSaving.value = false;
  }
}

function requestRefresh() {
  if (deferredDisks.value.length) {
    smartReadError.value = "";
    refreshAfterSmartSkip.value = true;
    confirmSmartRead.value = true;
    return;
  }
  refreshAfterSmartSkip.value = false;
  void load(true);
}

function skipSmartRead() {
  confirmSmartRead.value = false;
  if (refreshAfterSmartSkip.value) {
    refreshAfterSmartSkip.value = false;
    void load(true);
  }
}

async function readDeferredDisks() {
  if (smartReadBusy.value) return;
  const targets = deferredDisks.value.slice();
  if (!targets.length) {
    confirmSmartRead.value = false;
    return;
  }
  smartReadBusy.value = true;
  smartReadError.value = "";
  try {
    for (let i = 0; i < targets.length; i++) {
      const target = targets[i];
      smartReadProgress.value = `正在读取 ${i + 1}/${targets.length}：${target.id}`;
      await api.refreshDisk(target.id);
      data.value = await api.overview();
    }
    refreshAfterSmartSkip.value = false;
    confirmSmartRead.value = false;
  } catch (e: any) {
    smartReadError.value = e?.message || "读取 SMART 失败，可重试或先查看基础信息";
    data.value = await api.overview().catch(() => data.value);
  } finally {
    smartReadBusy.value = false;
    smartReadProgress.value = "";
  }
}

function open(d: Summary) {
  emit("open", d.id);
}

function railColor(g: HealthGrade): string {
  switch (g) {
    case "优": return "var(--good)";
    case "良": return "var(--fair)";
    case "警": return "var(--warn)";
    case "危": return "var(--crit)";
    default: return "var(--unknown)";
  }
}

function tagClass(d: Summary): string {
  if (d.interface === "USB") return "tag usb";
  if (d.interface === "NVMe") return "tag nvme";
  return "tag";
}

// NVMe 盘的 type 为 "NVMe"，与接口标签重复；介质标签统一显示 SSD。
function mediaTag(d: Summary): string {
  return d.type === "NVMe" ? "SSD" : d.type;
}

function usageColor(pct: number): string {
  if (pct >= 90) return "var(--crit)";
  if (pct >= 75) return "var(--warn)";
  return "var(--acc)";
}

function lifeColor(pctUsed: number): string {
  if (pctUsed >= 95) return "var(--crit)";
  if (pctUsed >= 80) return "var(--warn)";
  return "var(--good)";
}

function roleClass(role: string): string {
  if (role.includes("系统盘")) return "role-sys";
  if (role.includes("RAID")) return "role-raid";
  return "role-ind";
}

const capPct = computed(() => {
  if (!stats.value.totalCap || stats.value.totalCap <= 0 || stats.value.totalUsed < 0) return -1;
  return Math.min(100, Math.round((stats.value.totalUsed / stats.value.totalCap) * 100));
});
const capColor = computed(() => {
  const p = capPct.value;
  if (p < 0) return "var(--unknown)";
  if (p >= 90) return "var(--crit)";
  if (p >= 75) return "var(--warn)";
  return "var(--acc)";
});
</script>

<template>
  <div>
    <div class="topbar">
      <div class="brand">
        <div class="brand-logo"><IconDisk :size="20" /></div>
        <div>
          <div class="brand-name">磁盘高级管理</div>
          <div class="brand-sub">飞牛 fnOS 磁盘工具</div>
        </div>
      </div>
      <div class="spacer"></div>
      <span v-if="generatedAbs" class="gen-time" :class="{ stale }" :title="'上次采集时间 ' + generatedAbs">
        {{ generatedRel }} · {{ generatedAbs }}
      </span>
      <span v-if="stale" class="stale-label">数据已超过 24 小时</span>
      <button
        class="btn"
        :disabled="refreshing"
        :title="deferredDisks.length ? '先确认是否读取休眠盘 SMART' : '重新采集全部磁盘（可能唤醒休眠中的机械盘）'"
        @click="requestRefresh"
      >
        <IconRefresh :size="15" /> {{ refreshing ? "刷新中" : "刷新" }}
      </button>
    </div>

    <div v-if="data?.mock" class="mock-banner">
      <IconWarn :size="15" /> 当前为演示数据（开发模式）。部署到飞牛设备后显示真实磁盘信息。
    </div>

    <div v-if="deferredDisks.length && !confirmSmartRead" class="deferred-banner">
      <IconWarn :size="15" />
      <span>{{ deferredDisks.length }} 块机械硬盘的 SMART 数据暂缓读取，磁盘基础信息仍可查看。</span>
      <button class="btn sm" @click="confirmSmartRead = true">读取 SMART</button>
    </div>

    <SkeletonCards v-if="loading" kind="overview" />

    <div v-else-if="fetchError" class="state-box">
      <div class="icon"><IconWarn :size="36" /></div>
      <h3>无法加载磁盘数据</h3>
      <p>{{ fetchError }}</p>
      <button class="btn primary" @click="load()">重试</button>
    </div>

    <div v-else-if="data?.error" class="state-box">
      <div class="icon"><IconWarn :size="36" /></div>
      <h3>采集失败</h3>
      <p>{{ data.error }}</p>
      <button class="btn primary" @click="load()">重试</button>
    </div>

    <div v-else-if="disks.length === 0" class="state-box">
      <div class="icon"><IconDisk :size="36" /></div>
      <h3>未检测到磁盘</h3>
      <p>未发现可读取的 SATA / NVMe / USB 磁盘。请确认磁盘已正确连接并被系统识别。</p>
      <button class="btn" @click="load(true)"><IconRefresh :size="15" /> 重新扫描</button>
    </div>

    <template v-else>
      <div class="stat-strip">
        <span class="chip total" title="检测到的物理磁盘数量">
          <IconDisk :size="13" /> 共 <b>{{ stats.total }}</b> 块磁盘
        </span>
        <span class="chip ok" title="健康评级为优/良的磁盘数量">
          <IconCheck :size="13" /> <b>{{ stats.healthy }}</b> 块正常
        </span>
        <span class="chip warn" :class="{ zero: !stats.attention }" title="健康评级为警/危的磁盘数量">
          <IconWarn :size="13" /> <b>{{ stats.attention }}</b> 块需关注
        </span>
        <span class="cap-sep" aria-hidden="true"></span>
        <span class="chip cap" title="所有磁盘总容量与已用空间">
          <span class="cap-track"><span class="cap-fill" :style="{ width: capPct + '%', background: capColor }"></span></span>
          总容量 <b>{{ fmtBytes(stats.totalCap) }}</b>
          <span class="cap-used">已用 {{ fmtBytes(stats.totalUsed) }}（{{ capPct >= 0 ? capPct + '%' : '—' }}）</span>
        </span>
      </div>

      <div class="disklist-head">
        <h2>物理磁盘</h2>
        <span class="hint">休眠盘的 SMART 数据需确认后读取</span>
      </div>
      <div class="disk-grid">
        <div
          v-for="d in disks"
          :key="d.id"
          class="disk-card"
          :style="{ '--rail': railColor(d.health) }"
          @click="open(d)"
        >
          <div class="dc-head">
            <div class="disk-ico"><DiskTypeIcon :type="d.type" :size="20" /></div>
            <div class="dc-vol" v-if="(d.volumePaths || []).length">{{ (d.volumePaths || [])[0] }}</div>
            <div class="dc-vol none" v-else>{{ d.name }}</div>
            <PowerBadge
              v-if="d.powerState || d.hibernated"
              :power-state="d.powerState"
              :hibernated="d.hibernated"
              pulse
            />
            <HealthBadge :grade="d.health" />
          </div>
          <div class="dc-model-row">
            <span class="dc-model">{{ d.model || "未知型号" }}</span>
            <span
              v-if="d.userNote && editingNote !== d.id"
              class="tag note" :title="d.userNote + '（点击编辑）'"
              @click.stop="startNoteEdit(d)"
            >{{ d.userNote }}</span>
            <span v-else-if="editingNote !== d.id" class="tag note-add" title="添加备注" @click.stop="startNoteEdit(d)">＋ 备注</span>
          </div>
          <div v-if="editingNote === d.id" class="note-inline" @click.stop>
            <textarea
              v-model="noteDraft" rows="2" maxlength="200"
              placeholder="例如：2025-10 购入，冷备仓库盘"
              @keydown.enter.exact.prevent="saveNote(d)"
              @keydown.esc="cancelNoteEdit"
            ></textarea>
            <div class="note-inline-actions">
              <span class="note-count" :class="{ over: noteDraft.length >= 200 }">{{ noteDraft.length }}/200</span>
              <button class="btn sm" :disabled="noteSaving" @click.stop="cancelNoteEdit">取消</button>
              <button class="btn sm primary" :disabled="noteSaving" @click.stop="saveNote(d)">
                {{ noteSaving ? "保存中…" : "保存" }}
              </button>
            </div>
            <div v-if="noteError" class="note-err">{{ noteError }}</div>
          </div>
          <div class="dc-serial">{{ d.serial || "—" }}</div>
          <div class="disk-tags">
            <span :class="tagClass(d)">{{ d.interface }}</span>
            <span class="tag media" :class="d.type === 'HDD' ? 'media-hdd' : 'media-ssd'">{{ mediaTag(d) }}</span>
            <span class="tag ctrl" v-if="d.controller">{{ d.controller }}</span>
            <span class="tag role" v-if="d.role" :class="roleClass(d.role)">{{ d.role }}</span>
            <span class="tag bridge" v-if="d.bridge">{{ d.bridge }}</span>
            <span class="tag smart-deferred" v-if="d.smartDeferred">SMART 待读取</span>
          </div>
          <div class="dc-metrics">
            <div class="dm">
              <div class="dmk">容量</div>
              <div class="dmv">{{ fmtBytes(d.capacity) }}</div>
              <div class="minibar" v-if="d.usagePercent >= 0">
                <span :style="{ width: d.usagePercent + '%', background: usageColor(d.usagePercent) }"></span>
              </div>
            </div>
            <div class="dm">
              <div class="dmk">温度</div>
              <div class="dmv" :style="{ color: tempColorOf(d.temperatureC, d.type) }">{{ d.temperatureC || "-" }}<span class="u" v-if="d.temperatureC">°C</span></div>
            </div>
            <div class="dm" v-if="d.smartAvail && d.percentUsed >= 0">
              <div class="dmk">寿命已用</div>
              <div class="dmv">{{ d.percentUsed }}<span class="u">%</span></div>
              <div class="minibar">
                <span :style="{ width: d.percentUsed + '%', background: lifeColor(d.percentUsed) }"></span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="foot-note">磁盘高级管理 · 扩展飞牛 fnOS 磁盘信息 · 数据来自 smartctl / nvme</div>
    </template>
  </div>
  <SmartReadConfirm
    v-if="confirmSmartRead"
    :disks="deferredDisks"
    :busy="smartReadBusy"
    :progress="smartReadProgress"
    :error="smartReadError"
    @accept="readDeferredDisks"
    @skip="skipSmartRead"
  />
</template>

<style scoped>
.tag.note { cursor: pointer; }
.tag.note:hover { border-color: rgba(147,204,82,.55); background: rgba(147,204,82,.14); }
.tag.note-add { cursor: pointer; color: var(--ink-3); border-style: dashed; }
.tag.note-add:hover { color: var(--fair); border-color: rgba(147,204,82,.5); background: rgba(147,204,82,.08); }
.note-inline {
  display: flex; flex-direction: column; gap: 7px;
  margin-top: 2px;
}
.note-inline textarea {
  width: 100%; box-sizing: border-box; resize: vertical; min-height: 52px;
  background: var(--panel-3); border: 1px solid var(--line); border-radius: 8px;
  color: var(--ink); font-family: var(--font-sans); font-size: 13px; line-height: 1.6;
  padding: 8px 10px;
}
.note-inline textarea:focus { outline: none; border-color: var(--acc); }
.note-inline-actions { display: flex; align-items: center; gap: 8px; justify-content: flex-end; }
.note-count { margin-right: auto; color: var(--ink-3); font-size: 11px; font-family: var(--font-mono); }
.note-count.over { color: var(--crit); }
.note-err { color: var(--crit); font-size: 12px; }
.btn.sm { padding: 3px 9px; font-size: 11.5px; border-radius: 6px; }
.btn.sm.primary { background: #4fe0b0; border-color: #4fe0b0; color: #053326; font-weight: 700; }
.btn.sm.primary:hover:not(:disabled) { background: #6bf0c2; }
/* inline stat strip (no cards) */
.stat-strip {
  display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px;
  margin-bottom: 18px; padding: 0 2px;
}
.chip {
  display: inline-flex; align-items: center; gap: 6px;
  font-size: 12.5px; color: var(--ink-2); line-height: 1.4;
}
.chip b { font-family: var(--font-mono); font-weight: 700; color: var(--ink); }
.chip svg { flex: none; }
.chip.total svg { color: var(--acc); }
.chip.ok svg { color: var(--good); }
.chip.ok b { color: var(--good); }
.chip.warn svg { color: var(--warn); }
.chip.warn b { color: var(--warn); }
.chip.warn.zero svg, .chip.warn.zero b { color: var(--ink-3); }
.cap-sep { width: 1px; height: 16px; background: var(--line); margin: 0 4px; }
.chip.cap { gap: 8px; }
.cap-track {
  width: 88px; height: 6px; border-radius: 999px; overflow: hidden;
  background: var(--panel-3); border: 1px solid var(--line-soft); flex: none;
}
.cap-fill { display: block; height: 100%; border-radius: 999px; transition: width .4s ease; }
.cap-used { color: var(--ink-3); font-size: 11.5px; }
</style>
