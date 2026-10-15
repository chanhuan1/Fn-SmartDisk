<script setup lang="ts">
// Skeleton loading placeholders matching the overview / power / detail
// layouts, so first paint feels fast instead of showing a bare spinner.
const props = withDefaults(defineProps<{ kind?: "overview" | "detail" | "power" }>(), {
  kind: "overview",
});
</script>

<template>
  <div v-if="props.kind === 'overview'" class="sk-wrap" aria-busy="true" aria-label="正在加载">
    <div class="sk-strip">
      <div v-for="i in 4" :key="'s' + i" class="sk sk-stat"></div>
    </div>
    <div class="sk-grid">
      <div v-for="i in 6" :key="'c' + i" class="sk sk-card">
        <div class="sk-line" style="width: 55%"></div>
        <div class="sk-line" style="width: 80%"></div>
        <div class="sk-line" style="width: 40%"></div>
      </div>
    </div>
  </div>

  <div v-else-if="props.kind === 'detail'" class="sk-wrap" aria-busy="true" aria-label="正在加载">
    <div class="sk sk-head"></div>
    <div class="sk-cards">
      <div v-for="i in 5" :key="'m' + i" class="sk sk-metric"></div>
    </div>
    <div class="sk sk-block" v-for="i in 3" :key="'b' + i"></div>
  </div>

  <div v-else class="sk-wrap" aria-busy="true" aria-label="正在加载">
    <div class="sk sk-block" style="height: 76px"></div>
    <div class="sk sk-row" v-for="i in 5" :key="'r' + i"></div>
  </div>
</template>

<style scoped>
.sk-wrap { display: block; }
.sk {
  position: relative; overflow: hidden;
  background: var(--panel-2); border: 1px solid var(--line-soft);
  border-radius: var(--radius);
}
.sk::after {
  content: ""; position: absolute; inset: 0;
  background: linear-gradient(100deg, transparent 20%, rgba(255,255,255,.045) 50%, transparent 80%);
  animation: sk-shine 1.5s linear infinite;
}
@keyframes sk-shine { from { transform: translateX(-100%); } to { transform: translateX(100%); } }

.sk-strip { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 14px; margin-bottom: 24px; }
.sk-stat { height: 84px; }
.sk-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 14px; }
.sk-card { height: 168px; padding: 16px; }
.sk-line { height: 12px; border-radius: 6px; background: var(--panel-3); margin: 14px 0; }

.sk-head { height: 64px; margin-bottom: 18px; }
.sk-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 13px; margin-bottom: 22px; }
.sk-metric { height: 96px; }
.sk-block { height: 220px; margin-bottom: 18px; }
.sk-row { height: 58px; margin-bottom: 10px; }
</style>
