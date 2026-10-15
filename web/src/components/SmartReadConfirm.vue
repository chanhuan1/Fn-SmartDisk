<script setup lang="ts">
import type { Summary } from "../api";
import { IconWarn } from "../icons";

withDefaults(defineProps<{
	disks: Array<Pick<Summary, "id" | "model" | "serial">>;
  busy?: boolean;
  progress?: string;
  error?: string;
}>(), {
  busy: false,
  progress: "",
  error: "",
});

const emit = defineEmits<{
  (e: "accept"): void;
  (e: "skip"): void;
}>();
</script>

<template>
  <div class="sr-mask" @click.self="!busy && emit('skip')">
    <section class="sr-box" role="dialog" aria-modal="true" aria-labelledby="sr-title">
      <header class="sr-head">
        <span class="sr-icon"><IconWarn :size="18" /></span>
        <h2 id="sr-title">读取休眠磁盘 SMART 数据？</h2>
      </header>
      <div class="sr-body">
        <p>读取完整 SMART 数据可能会唤醒机械硬盘。你可以先查看基础信息，稍后再单独读取这些磁盘。</p>
        <ul class="sr-disks">
          <li v-for="disk in disks" :key="disk.id">
            <strong>{{ disk.model || disk.id }}</strong>
            <span>{{ disk.id }}<template v-if="disk.serial"> · {{ disk.serial }}</template></span>
          </li>
        </ul>
        <div class="sr-note">选择“只看基础信息”不会读取这些磁盘的 SMART 数据，也不会自动重试。</div>
        <div v-if="progress" class="sr-progress">{{ progress }}</div>
        <div v-if="error" class="sr-error">{{ error }}</div>
      </div>
      <footer class="sr-foot">
        <button class="btn" :disabled="busy" @click="emit('skip')">{{ busy ? "读取中…" : "只看基础信息" }}</button>
        <button class="btn primary" :disabled="busy || !disks.length" @click="emit('accept')">
          {{ busy ? "读取中…" : "继续读取（可能唤醒）" }}
        </button>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.sr-mask { position: fixed; inset: 0; z-index: 80; display: grid; place-items: center; padding: 20px; background: rgba(5,8,12,.66); backdrop-filter: blur(3px); }
.sr-box { width: min(520px, 100%); overflow: hidden; background: linear-gradient(180deg, var(--panel-2), var(--panel)); border: 1px solid var(--line); border-radius: 14px; box-shadow: 0 24px 60px rgba(0,0,0,.5); }
.sr-head { display: flex; align-items: center; gap: 10px; padding: 14px 16px; border-bottom: 1px solid var(--line-soft); }
.sr-icon { width: 34px; height: 34px; display: grid; place-items: center; flex: none; color: var(--warn); background: rgba(232,168,62,.12); border: 1px solid rgba(232,168,62,.32); border-radius: 9px; }
.sr-head h2 { margin: 0; font-size: 15px; font-weight: 800; }
.sr-body { padding: 16px; color: var(--ink-2); font-size: 13px; line-height: 1.55; }
.sr-body p { margin: 0 0 12px; }
.sr-disks { display: grid; gap: 7px; max-height: 170px; overflow-y: auto; margin: 0; padding: 0; list-style: none; }
.sr-disks li { display: flex; flex-direction: column; gap: 2px; padding: 9px 11px; background: var(--panel-3); border: 1px solid var(--line); border-radius: 8px; }
.sr-disks strong { color: var(--ink); overflow-wrap: anywhere; }
.sr-disks span { color: var(--ink-3); font: 11px var(--font-mono); }
.sr-note { margin-top: 12px; color: var(--ink-3); font-size: 12px; }
.sr-progress { margin-top: 12px; color: var(--acc); }
.sr-error { margin-top: 12px; padding: 9px 12px; color: var(--crit); background: rgba(236,91,82,.12); border: 1px solid rgba(236,91,82,.35); border-radius: 8px; }
.sr-foot { display: flex; justify-content: flex-end; gap: 10px; padding: 14px 16px; border-top: 1px solid var(--line-soft); }
@media (max-width: 480px) { .sr-foot { flex-direction: column-reverse; } .sr-foot .btn { justify-content: center; } }
</style>
