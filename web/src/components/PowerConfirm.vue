<script setup lang="ts">
import { ref, computed, watch, nextTick } from "vue";
import { api, type Summary } from "../api";
import { IconPowerOff, IconWake, IconWarn } from "../icons";

// Confirmation modal for the deep-offline / wake power actions. The parent
// passes the target disk and which action to arm; we emit done(ok, message)
// after the API call resolves.
const props = defineProps<{
  disk: Summary;
  action: "offline" | "wake";
}>();
const emit = defineEmits<{
  (e: "close"): void;
  (e: "done", ok: boolean, message: string): void;
}>();

const busy = ref(false);
const error = ref<string | null>(null);
const raidAck = ref(false);
const needRaid = computed(() => props.action === "offline" && !!props.disk.needsRaidConfirm);
// IO confirmation: shown when the backend rejects offline with 正在读写 (409).
const ioBusy = ref(false);
const ioAck = ref(false);

// Live step log streamed from the backend while the op runs.
const steps = ref<string[]>([]);
const logBox = ref<HTMLElement | null>(null);

watch(
  () => [props.disk?.id, props.action],
  () => {
    busy.value = false;
    error.value = null;
    raidAck.value = false;
    ioBusy.value = false;
    ioAck.value = false;
    steps.value = [];
  },
);

async function pushStep(_step: string, detail: string) {
  steps.value.push(detail);
  await nextTick();
  logBox.value?.scrollTo({ top: logBox.value.scrollHeight, behavior: "smooth" });
}

const meta = computed(() => {
  switch (props.action) {
    case "offline":
      return {
        title: "深度下线硬盘",
        icon: "off",
        confirm: "深度下线",
        cls: "danger",
      };
    default:
      return {
        title: "唤醒硬盘",
        icon: "wake",
        confirm: "唤醒",
        cls: "primary",
      };
  }
});

const vols = computed(() => props.disk.volumePaths ?? []);

async function run() {
  if (busy.value) return;
  if (needRaid.value && !raidAck.value) return;
  if (ioBusy.value && !ioAck.value) return;
  busy.value = true;
  error.value = null;
  steps.value = [];
  try {
    let res;
    if (props.action === "offline") {
      res = await api.offlineStream(
        props.disk.id,
        raidAck.value,
        ioAck.value,
        pushStep,
      );
    }
    else res = await api.wakeStream(props.disk.id, pushStep);
    emit("done", true, res.message);
  } catch (e: any) {
    const msg = e?.message || "操作失败";
    // 正在读写：offer a force-offline confirmation instead of a hard error.
    if (/正在读写|IO 计数/.test(msg) && props.action === "offline") {
      ioBusy.value = true;
      error.value = null;
    } else {
      error.value = msg;
      emit("done", false, msg);
    }
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="pc-mask" @click.self="!busy && emit('close')">
    <div class="pc-box" role="dialog" aria-modal="true">
      <div class="pc-head">
        <span class="pc-ico" :class="meta.cls">
          <IconPowerOff v-if="meta.icon === 'off'" :size="18" />
          <IconWake v-else :size="18" />
        </span>
        <div class="pc-title">{{ meta.title }}</div>
        <button class="pc-x" :disabled="busy" @click="emit('close')" aria-label="关闭">×</button>
      </div>

      <div class="pc-body">
        <div class="pc-disk">
          <span class="pc-model">{{ disk.model || disk.id }}</span>
          <span class="pc-ser">{{ disk.serial || "—" }}</span>
        </div>

        <template v-if="action === 'offline'">
          <p>将按以下顺序执行内核级隔离，之后<strong>飞牛系统和文件管理器都无法再唤醒此盘</strong>，只能在本面板手动唤醒：</p>
          <ol class="pc-steps">
            <li>同步数据（sync）</li>
            <li>懒卸载卷（umount -l，先 sync 刷盘）：<code v-for="v in vols" :key="v" class="pc-vol">{{ v }}</code><span v-if="!vols.length" class="muted">（无挂载卷）</span></li>
            <li>停转（hdparm -y）</li>
            <li>内核标记 device offline</li>
          </ol>
          <div class="pc-warn">
            <span class="pc-wico"><IconWarn :size="15" /></span>
            <div>
              卸载后<strong>对应存储空间将从飞牛界面消失</strong>，引用它的共享、应用、Docker 会无法访问，直到你在本面板唤醒。
            </div>
          </div>
          <div v-if="needRaid" class="pc-raid">
            <span class="pc-wico"><IconWarn :size="15" /></span>
            <div>
              <strong>高风险：此盘是 md RAID 阵列成员。</strong>下线会让阵列<strong>降级</strong>；若这是单盘 RAID1（镜像只有它一个成员），阵列会直接<strong>离线</strong>，重新上线时若未自动重组，需重启或手动 <code>mdadm --assemble</code> 恢复。操作不当有数据丢失风险。
              <label class="pc-ack">
                <input type="checkbox" v-model="raidAck" />
                我已了解阵列降级风险，仍要下线
              </label>
            </div>
          </div>

          <div v-if="ioBusy" class="pc-raid">
            <span class="pc-wico"><IconWarn :size="15" /></span>
            <div>
              <strong>检测到该盘正在读写。</strong>此刻强制下线（懒卸载）可能丢失尚未落盘的数据，导致文件不完整或文件系统需修复。建议等读写停止后再下线。
              <label class="pc-ack">
                <input type="checkbox" v-model="ioAck" />
                我确认数据风险，仍要强制下线
              </label>
            </div>
          </div>
        </template>

        <template v-else>
          <p>写入内核 <code>running</code> + 重扫总线，等待盘体起旋后刷新数据。</p>
          <p class="muted">机械盘起旋约需 5-15 秒，期间请勿重复操作。</p>
          <div class="pc-warn">
            <span class="pc-wico"><IconWarn :size="15" /></span>
            <div>唤醒后硬盘仅在内核层上线。<strong>要正常访问数据，还需到飞牛「硬盘设置 / 存储管理」里点击「挂载硬盘」</strong>，挂载对应存储空间后才能访问。</div>
          </div>
        </template>

        <div v-if="error" class="pc-err">{{ error }}</div>

        <div v-if="busy || steps.length" ref="logBox" class="pc-log">
          <div v-for="(s, i) in steps" :key="i" class="pc-logline">
            <span class="pc-logck">✓</span>{{ s }}
          </div>
          <div v-if="busy" class="pc-logline run">
            <span class="pc-spin2"></span>执行中…
          </div>
        </div>
      </div>

      <div class="pc-foot">
        <button class="btn" :disabled="busy" @click="emit('close')">取消</button>
        <button
          class="btn"
          :class="meta.cls === 'danger' ? 'danger' : 'primary'"
          :disabled="busy || (needRaid && !raidAck) || (ioBusy && !ioAck)"
          @click="run"
        >
          <span v-if="busy" class="pc-spin"></span>
          {{ busy ? "执行中…" : meta.confirm }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pc-mask {
  position: fixed; inset: 0; z-index: 80;
  background: rgba(5, 8, 12, .66); backdrop-filter: blur(3px);
  display: grid; place-items: center; padding: 20px;
}
.pc-box {
  width: min(520px, 100%); background: linear-gradient(180deg, var(--panel-2), var(--panel));
  border: 1px solid var(--line); border-radius: 14px; box-shadow: 0 24px 60px rgba(0,0,0,.5);
  overflow: hidden;
}
.pc-head { display: flex; align-items: center; gap: 10px; padding: 14px 16px; border-bottom: 1px solid var(--line-soft); }
.pc-ico { width: 34px; height: 34px; border-radius: 9px; display: grid; place-items: center; flex: none; }
.pc-ico.primary { color: var(--acc); background: rgba(63,193,212,.12); border: 1px solid rgba(63,193,212,.3); }
.pc-ico.danger { color: var(--crit); background: rgba(236,91,82,.12); border: 1px solid rgba(236,91,82,.35); }
.pc-title { font-size: 15px; font-weight: 800; letter-spacing: .2px; }
.pc-x { margin-left: auto; background: none; border: none; color: var(--ink-3); font-size: 22px; line-height: 1; cursor: pointer; padding: 2px 6px; border-radius: 6px; }
.pc-x:hover { color: var(--ink); background: var(--panel-3); }
.pc-x:disabled { opacity: .4; cursor: not-allowed; }
.pc-body { padding: 16px; font-size: 13px; color: var(--ink-2); }
.pc-body p { margin: 8px 0; }
.pc-body code { font-family: var(--font-mono); background: var(--panel-3); padding: 1px 6px; border-radius: 5px; font-size: 12px; color: var(--acc); }
.muted { color: var(--ink-3); }
.pc-disk { display: flex; flex-direction: column; gap: 2px; background: var(--panel-3); border: 1px solid var(--line); border-radius: 9px; padding: 10px 12px; margin-bottom: 12px; }
.pc-model { font-weight: 700; color: var(--ink); }
.pc-ser { font-family: var(--font-mono); font-size: 12px; color: var(--ink-3); }
.pc-steps { margin: 8px 0; padding-left: 20px; }
.pc-steps li { margin: 4px 0; }
.pc-vol { margin-right: 6px; }
.pc-warn, .pc-raid { display: flex; gap: 10px; padding: 10px 12px; border-radius: 9px; margin-top: 12px; font-size: 12.5px; line-height: 1.55; }
.pc-warn { color: var(--warn); background: rgba(232,168,62,.1); border: 1px solid rgba(232,168,62,.3); }
.pc-raid { color: var(--crit); background: rgba(236,91,82,.1); border: 1px solid rgba(236,91,82,.35); }
.pc-wico { flex: none; margin-top: 2px; display: inline-grid; place-items: center; }
.pc-wico :deep(svg) { width: 15px !important; height: 15px !important; }
.pc-warn strong, .pc-raid strong { color: inherit; }
.pc-ack { display: flex; align-items: center; gap: 8px; margin-top: 10px; font-weight: 700; color: var(--ink); cursor: pointer; user-select: none; }
.pc-ack input { width: 15px; height: 15px; accent-color: var(--crit); cursor: pointer; }
.pc-err { margin-top: 12px; padding: 9px 12px; border-radius: 8px; background: rgba(236,91,82,.12); border: 1px solid rgba(236,91,82,.35); color: var(--crit); font-size: 12.5px; }
.pc-log {
  margin-top: 12px; max-height: 180px; overflow-y: auto;
  background: var(--panel-3); border: 1px solid var(--line); border-radius: 8px;
  padding: 9px 12px; font-family: var(--font-mono); font-size: 12px; color: var(--ink-2);
}
.pc-logline { display: flex; align-items: center; gap: 8px; padding: 2px 0; }
.pc-logck { color: var(--good); font-weight: 700; flex: none; }
.pc-logline.run { color: var(--acc); }
.pc-spin2 { width: 11px; height: 11px; border: 2px solid rgba(63,193,212,.25); border-top-color: var(--acc); border-radius: 50%; animation: pcrot .7s linear infinite; flex: none; }
.pc-foot { display: flex; justify-content: flex-end; gap: 10px; padding: 14px 16px; border-top: 1px solid var(--line-soft); }
.btn.danger { background: var(--crit); border-color: var(--crit); color: #3d0a06; font-weight: 700; }
.btn.danger:hover:not(:disabled) { background: #f2706a; }
.pc-spin { display: inline-block; width: 13px; height: 13px; border: 2px solid rgba(0,0,0,.25); border-top-color: currentColor; border-radius: 50%; animation: pcrot .7s linear infinite; vertical-align: -2px; }
@keyframes pcrot { to { transform: rotate(360deg); } }
</style>
