<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import {
  api, fmtBytes, fmtAgo, fmtClock,
  type Overview, type Summary, type PowerEvent,
} from "../api";
import PowerConfirm from "../components/PowerConfirm.vue";
import PowerBadge from "../components/PowerBadge.vue";
import DiskTypeIcon from "../components/DiskTypeIcon.vue";
import SkeletonCards from "../components/SkeletonCards.vue";
import {
  IconWarn, IconPowerOff, IconWake, IconLog,
} from "../icons";

const data = ref<Overview | null>(null);
const loading = ref(true);
const refreshing = ref(false);
const fetchError = ref<string | null>(null);
const toast = ref<{ ok: boolean; msg: string } | null>(null);
const events = ref<PowerEvent[]>([]);
const showLog = ref(true);
const clearingLog = ref(false);

async function load(isRefresh = false) {
  if (isRefresh) refreshing.value = true;
  else loading.value = true;
  fetchError.value = null;
  try {
    const [ov, lg] = await Promise.all([
      isRefresh ? api.refresh() : api.overview(),
      api.powerLog(100).catch(() => ({ events: [], count: 0 })),
    ]);
    data.value = ov;
    events.value = lg.events ?? [];
  } catch (e: any) {
    fetchError.value = e?.message || "无法连接后端服务";
  } finally {
    loading.value = false;
    refreshing.value = false;
  }
}
onMounted(() => load());

// Sort same as overview: by volume number then id.
function volKey(d: Summary): number {
  let best = Number.MAX_SAFE_INTEGER;
  for (const v of d.volumePaths ?? []) {
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

// Modal state.
const modal = ref<{ disk: Summary; action: "offline" | "wake" } | null>(null);
function ask(d: Summary, action: "offline" | "wake") {
  modal.value = { disk: d, action };
}
async function onDone(ok: boolean, message: string) {
  // Keep the modal open on failure so the user sees the error; close on ok.
  if (ok) {
    modal.value = null;
    toast.value = { ok, msg: message };
    setTimeout(() => (toast.value = null), 5000);
    await load(); // power operation already patched the cached overview
  }
}

// Sub-line under the power badge: offline duration / last-offline time.
function stateSub(d: Summary): string {
  if (d.hibernated && d.offlineAt) return `下线于 ${fmtClock(d.offlineAt)}（${fmtAgo(d.offlineAt)}）`;
  return "";
}

const actionMeta: Record<string, { text: string; cls: string }> = {
  sleep: { text: "休眠", cls: "sleep" },
  offline: { text: "下线", cls: "off" },
  wake: { text: "唤醒", cls: "on" },
  remount: { text: "补挂载", cls: "idle" },
  step: { text: "步骤", cls: "step" },
};
function evAction(a: string) {
  return actionMeta[a] ?? { text: a, cls: "unk" };
}

async function clearLog() {
  if (clearingLog.value) return;
  clearingLog.value = true;
  try {
    await api.clearPowerLog();
    events.value = [];
    toast.value = { ok: true, msg: "操作日志已清空" };
    setTimeout(() => (toast.value = null), 4000);
  } catch (e: any) {
    toast.value = { ok: false, msg: e?.message || "清空失败" };
    setTimeout(() => (toast.value = null), 5000);
  } finally {
    clearingLog.value = false;
  }
}
</script>

<template>
  <div>
    <div class="pw-banner">
      <span class="pw-bico"><IconWarn :size="15" /></span>
      <div>
        <strong>深度下线</strong>会把硬盘在内核层隔离（卸载卷 + 停转 + device offline），
        之后飞牛后台与文件管理器都无法唤醒它，只能在此手动唤醒。适合冷备盘。<br>
        <span class="pw-note2">注意：<strong>固态硬盘（SSD / NVMe）不能休眠</strong>，无机械结构无需停转；<strong>仅 SATA 机械硬盘（HDD）</strong>可进行深度下线/休眠。</span>
      </div>
    </div>

    <div v-if="toast" class="pw-toast" :class="toast.ok ? 'ok' : 'err'">{{ toast.msg }}</div>

    <SkeletonCards v-if="loading" kind="power" />

    <div v-else-if="fetchError" class="state-box">
      <div class="icon"><IconWarn :size="36" /></div>
      <h3>无法加载</h3>
      <p>{{ fetchError }}</p>
      <button class="btn primary" @click="load()">重试</button>
    </div>

    <template v-else>
      <div class="pw-table">
        <div class="pw-row pw-head">
          <div class="c-disk">硬盘</div>
          <div class="c-vol">卷</div>
          <div class="c-cap">容量</div>
          <div class="c-state">电源状态</div>
          <div class="c-ops">操作</div>
        </div>
        <div v-for="d in disks" :key="d.id" class="pw-row" :class="{ off: d.hibernated }">
          <div class="c-disk">
            <span class="pw-ico"><DiskTypeIcon :type="d.type" :size="17" /></span>
            <div>
              <div class="pw-model">{{ d.model || "未知型号" }}</div>
              <div v-if="d.userNote" class="pw-note-tag" :title="d.userNote">
                <span class="tag note">{{ d.userNote }}</span>
              </div>
              <div class="pw-ser">{{ d.name }} · {{ d.serial || "—" }}</div>
            </div>
          </div>
          <div class="c-vol">
            <span v-if="(d.volumePaths || []).length" class="pw-vol">{{ (d.volumePaths || [])[0] }}</span>
            <span v-else class="pw-none">—</span>
          </div>
          <div class="c-cap">{{ fmtBytes(d.capacity) }}</div>
          <div class="c-state">
            <PowerBadge :power-state="d.powerState" :hibernated="d.hibernated" />
            <span v-if="stateSub(d)" class="pw-sub">{{ stateSub(d) }}</span>
          </div>
          <div class="c-ops">
            <template v-if="!d.hibernated">
              <button
                class="btn sm danger" :disabled="!d.canOffline"
                :title="d.canOffline ? '卸载卷 → 停转 → device offline，飞牛无法唤醒' : d.noOfflineReason"
                @click="ask(d, 'offline')"
              ><IconPowerOff :size="13" /> 深度下线</button>
            </template>
            <button v-else class="btn sm primary" title="恢复 running 并重扫总线；之后需在飞牛硬盘设置里点「挂载硬盘」" @click="ask(d, 'wake')">
              <IconWake :size="13" /> 唤醒
            </button>
          </div>
        </div>
      </div>

      <!-- event log -->
      <div class="pw-log">
        <button class="pw-loghead" @click="showLog = !showLog">
          <IconLog :size="14" />
          <span>操作日志</span>
          <span class="cnt" v-if="events.length">{{ events.length }}</span>
          <span class="log-tools">
            <button
              class="log-clear" title="清空全部操作日志" :disabled="clearingLog || !events.length"
              @click.stop="clearLog"
            >{{ clearingLog ? "清空中…" : "清空" }}</button>
            <span class="caret" :class="{ open: showLog }">▾</span>
          </span>
        </button>
        <div v-if="showLog" class="pw-logbody">
          <div v-if="!events.length" class="pw-logempty">暂无操作记录</div>
          <div v-for="(e, i) in events" :key="i" class="pw-logrow">
            <span class="lt">{{ fmtClock(e.time) }}</span>
            <span class="la" :class="evAction(e.action).cls">{{ evAction(e.action).text }}</span>
            <span class="ld">
              <b>{{ e.diskId }}</b>
              <span v-if="e.model" class="lm"> {{ e.model }}</span>
            </span>
            <span class="ls" :class="e.ok ? 'ok' : 'fail'">{{ e.ok ? "成功" : "失败" }}</span>
            <span class="lx">{{ e.detail || "—" }}</span>
          </div>
        </div>
      </div>
    </template>

    <PowerConfirm
      v-if="modal"
      :disk="modal.disk"
      :action="modal.action"
      @close="modal = null"
      @done="onDone"
    />
  </div>
</template>

<style scoped>
.pw-banner {
  display: flex; gap: 10px; align-items: flex-start;
  background: rgba(232,168,62,.08); border: 1px solid rgba(232,168,62,.28);
  color: var(--ink-2); font-size: 12.5px; line-height: 1.6;
  border-radius: 10px; padding: 12px 14px; margin-bottom: 16px;
}
.pw-bico { color: var(--warn); flex: none; margin-top: 2px; display: inline-grid; place-items: center; }
.pw-bico :deep(svg) { width: 15px !important; height: 15px !important; }
.pw-banner strong { color: var(--warn); }
.pw-note2 { color: var(--ink-3); font-size: 12px; }
.pw-note2 strong { color: var(--acc); }
.pw-toast {
  position: sticky; top: 10px; z-index: 30; margin-bottom: 14px;
  padding: 11px 14px; border-radius: 9px; font-size: 13px; font-weight: 600;
}
.pw-toast.ok { background: rgba(67,201,117,.13); border: 1px solid rgba(67,201,117,.4); color: var(--good); }
.pw-toast.err { background: rgba(236,91,82,.13); border: 1px solid rgba(236,91,82,.4); color: var(--crit); }

.pw-table { background: linear-gradient(180deg, var(--panel-2), var(--panel)); border: 1px solid var(--line); border-radius: 12px; overflow: hidden; }
.pw-row {
  display: grid; grid-template-columns: minmax(0,2.2fr) minmax(0,1fr) 90px 110px minmax(0,1.7fr);
  gap: 12px; align-items: center; padding: 13px 16px; border-top: 1px solid var(--line-soft);
}
.pw-row.pw-head { border-top: none; background: var(--panel-3); font-size: 11px; font-weight: 700; letter-spacing: .6px; color: var(--ink-3); text-transform: uppercase; padding: 9px 16px; }
.pw-row.off { opacity: .62; }
.c-disk { display: flex; align-items: center; gap: 10px; min-width: 0; }
.pw-ico { width: 32px; height: 32px; border-radius: 8px; flex: none; display: grid; place-items: center; background: var(--panel-3); border: 1px solid var(--line); color: var(--acc); }
.pw-model { font-weight: 700; color: var(--ink); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pw-note-tag { margin: 3px 0 1px; }
.pw-note-tag .tag.note { font-size: 10.5px; padding: 2px 7px; line-height: 1.4; white-space: normal; }
.pw-ser { font-family: var(--font-mono); font-size: 11.5px; color: var(--ink-3); }
.c-vol .pw-vol { font-family: var(--font-mono); font-size: 12.5px; font-weight: 700; color: var(--fair); }
.pw-none { color: var(--ink-3); }
.c-cap { font-family: var(--font-mono); font-size: 12.5px; color: var(--ink-2); }
.c-state { display: flex; flex-direction: column; gap: 4px; align-items: flex-start; }
.pw-sub { font-size: 11px; color: var(--ink-3); font-family: var(--font-mono); }

/* event log */
.pw-log { margin-top: 18px; background: linear-gradient(180deg, var(--panel-2), var(--panel)); border: 1px solid var(--line); border-radius: 12px; overflow: hidden; }
.pw-loghead {
  width: 100%; display: flex; align-items: center; gap: 8px;
  background: var(--panel-3); border: none; cursor: pointer;
  padding: 11px 16px; color: var(--ink-2); font-size: 13px; font-weight: 700; font-family: var(--font-sans);
}
.pw-loghead:hover { color: var(--ink); }
.pw-loghead svg { color: var(--acc); width: 14px !important; height: 14px !important; flex: none; }
.pw-loghead .cnt { background: var(--acc-deep); color: var(--acc); font-size: 11px; font-weight: 700; padding: 1px 8px; border-radius: 999px; }
.log-tools { margin-left: auto; display: flex; align-items: center; gap: 10px; }
.log-clear {
  background: transparent; border: 1px solid var(--line); border-radius: 6px;
  color: var(--ink-3); font-size: 11.5px; font-weight: 600; padding: 3px 10px;
  font-family: var(--font-sans); cursor: pointer;
}
.log-clear:hover:not(:disabled) { color: var(--crit); border-color: rgba(236,91,82,.45); background: rgba(236,91,82,.08); }
.log-clear:disabled { opacity: .45; cursor: not-allowed; }
.pw-loghead .caret { margin-left: 0; color: var(--ink-3); transition: transform .18s; }
.pw-loghead .caret.open { transform: rotate(180deg); }
.pw-logbody { max-height: 340px; overflow-y: auto; }
.pw-logempty { padding: 22px; text-align: center; color: var(--ink-3); font-size: 12.5px; }
.pw-logrow {
  display: grid; grid-template-columns: 88px 60px minmax(0,1.3fr) 50px minmax(0,2fr);
  gap: 8px; align-items: start; padding: 8px 16px; border-top: 1px solid var(--line-soft);
  font-size: 12.5px; min-width: 0;
}
.pw-logrow > span { min-width: 0; }
.pw-logrow .lt { font-family: var(--font-mono); color: var(--ink-3); font-size: 11.5px; }
.pw-logrow .la { font-weight: 700; font-size: 11.5px; text-align: center; padding: 2px 0; border-radius: 5px; border: 1px solid; }
.pw-logrow .la.sleep { color: var(--acc); border-color: rgba(63,193,212,.35); background: rgba(63,193,212,.1); }
.pw-logrow .la.off { color: var(--ink-3); border-color: var(--line); background: var(--panel-3); }
.pw-logrow .la.on { color: var(--good); border-color: rgba(67,201,117,.35); background: rgba(67,201,117,.1); }
.pw-logrow .la.idle { color: var(--fair); border-color: rgba(147,204,82,.32); background: rgba(147,204,82,.08); }
.pw-logrow .la.step { color: var(--ink-3); border-color: var(--line); background: transparent; border-style: dashed; }
.pw-logrow .la.unk { color: var(--unknown); border-color: var(--line); background: var(--panel-3); }
.pw-logrow .ld b { color: var(--ink); font-family: var(--font-mono); font-weight: 700; }
.pw-logrow .lm { color: var(--ink-3); font-size: 11.5px; }
.pw-logrow .ls { font-weight: 700; font-size: 11.5px; }
.pw-logrow .ls.ok { color: var(--good); }
.pw-logrow .ls.fail { color: var(--crit); }
.pw-logrow .lx { color: var(--ink-2); font-size: 12px; white-space: normal; word-break: break-all; overflow-wrap: anywhere; }
@media (max-width: 860px) {
  .pw-logrow { grid-template-columns: 78px 54px minmax(0,1fr) 46px; }
  .pw-logrow .lx { display: none; }
}
.c-ops { display: flex; gap: 8px; flex-wrap: wrap; }
.btn.sm { padding: 5px 11px; font-size: 12px; border-radius: 7px; }
.btn.sm.danger { color: var(--crit); border-color: rgba(236,91,82,.4); background: rgba(236,91,82,.1); }
.btn.sm.danger:hover:not(:disabled) { background: rgba(236,91,82,.18); border-color: var(--crit); }
.btn.sm.primary {
  background: #4fe0b0; border-color: #4fe0b0; color: #053326; font-weight: 700;
  box-shadow: 0 0 10px rgba(79,224,176,.35);
}
.btn.sm.primary:hover:not(:disabled) { background: #6bf0c2; box-shadow: 0 0 14px rgba(79,224,176,.5); }
@media (max-width: 860px) {
  .pw-row { grid-template-columns: minmax(0,1.6fr) 90px 100px; }
  .c-vol, .c-cap { display: none; }
  .pw-row.pw-head .c-vol, .pw-row.pw-head .c-cap { display: none; }
}
</style>
