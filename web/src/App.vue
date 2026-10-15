<script setup lang="ts">
import { ref, onMounted } from "vue";
import { api } from "./api";
import Overview from "./views/Overview.vue";
import Detail from "./views/Detail.vue";
import Power from "./views/Power.vue";

// Live app version from the backend (/api/health), shown in the tab bar.
const version = ref("");
onMounted(async () => {
  try {
    const h = await api.health();
    if (h.version) version.value = h.version;
  } catch {
    /* 版本号拿不到就不显示，不影响主功能 */
  }
});

// Hash-based mini router: "#/disk/<id>" detail, "#/power" power control,
// otherwise overview.
const route = ref<string>(location.hash || "#/");

function onHash() {
  route.value = location.hash || "#/";
  window.scrollTo(0, 0);
}
onMounted(() => window.addEventListener("hashchange", onHash));

function go(id: string) {
  location.hash = `#/disk/${encodeURIComponent(id)}`;
}
function goHome() {
  location.hash = "#/";
}
function goPower() {
  location.hash = "#/power";
}

function diskId(): string | null {
  const m = route.value.match(/^#\/disk\/(.+)$/);
  return m ? decodeURIComponent(m[1]) : null;
}
function isPower(): boolean {
  return route.value === "#/power";
}

// Global safety net: any uncaught render/runtime error shows a recoverable
// fallback instead of a blank page inside the fnOS iframe.
const fatal = ref<string | null>(null);
onMounted(() => {
  window.addEventListener("error", (ev) => {
    fatal.value = ev?.message || "页面脚本错误";
  });
  window.addEventListener("unhandledrejection", (ev) => {
    fatal.value = (ev?.reason as Error)?.message || "请求处理失败";
  });
});
function recover() {
  fatal.value = null;
  location.reload();
}
</script>

<template>
  <div class="shell">
    <nav class="tabbar">
      <button class="tab" :class="{ on: !diskId() && !isPower() }" @click="goHome">磁盘总览</button>
      <button class="tab" :class="{ on: isPower() }" @click="goPower">电源控制</button>
      <span v-if="version" class="ver" :title="'磁盘高级管理 版本 ' + version">v{{ version }}</span>
    </nav>
    <div v-if="fatal" class="state-box fatal-box">
      <h3>页面出现异常</h3>
      <p>{{ fatal }}</p>
      <button class="btn primary" @click="recover">重新加载</button>
    </div>
    <template v-else>
      <Detail v-if="diskId()" :id="diskId()!" @back="goHome" />
      <Power v-else-if="isPower()" />
      <Overview v-else @open="go" />
    </template>
  </div>
</template>

<style scoped>
.tabbar {
  position: sticky; top: 0; z-index: 50;
  display: flex; gap: 6px; margin: -22px -24px 16px; padding: 10px 24px 0;
  border-bottom: 1px solid var(--line-soft);
  background: rgba(11, 15, 20, .82);
  backdrop-filter: blur(14px) saturate(1.15);
  -webkit-backdrop-filter: blur(14px) saturate(1.15);
}
.tab {
  background: none; border: none; border-bottom: 2px solid transparent;
  color: var(--ink-3); font-size: 13.5px; font-weight: 700; font-family: var(--font-sans);
  padding: 8px 14px 10px; cursor: pointer; letter-spacing: .3px;
}
.tab:hover { color: var(--ink); }
.tab.on { color: var(--acc); border-bottom-color: var(--acc); }
.ver {
  margin-left: auto; align-self: center;
  font-family: var(--font-mono); font-size: 11.5px; font-weight: 700;
  color: var(--ink-3); background: var(--panel-3); border: 1px solid var(--line);
  padding: 2px 9px; border-radius: 999px; letter-spacing: .3px;
}
.fatal-box { margin-top: 40px; }
@media (max-width: 860px) {
  .tabbar { margin: -16px -14px 14px; padding: 8px 14px 0; }
}
</style>
