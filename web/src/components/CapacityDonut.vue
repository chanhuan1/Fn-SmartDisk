<script setup lang="ts">
import { computed } from "vue";
import { fmtBytes } from "../api";

// Donut showing used vs free capacity with the usage % in the center.
const props = defineProps<{ used: number; total: number }>();

const R = 44;
const C = 2 * Math.PI * R;

const pct = computed(() => {
  if (!props.total || props.total <= 0 || props.used < 0) return -1;
  return Math.min(100, Math.round((props.used / props.total) * 100));
});

const color = computed(() => {
  if (pct.value < 0) return "var(--unknown)";
  if (pct.value >= 90) return "var(--crit)";
  if (pct.value >= 75) return "var(--warn)";
  return "var(--acc)";
});

const dash = computed(() => {
  const p = Math.max(0, pct.value);
  return `${(p / 100) * C} ${C}`;
});
</script>

<template>
  <div class="donut" :title="`已用 ${fmtBytes(used)} / 共 ${fmtBytes(total)}`">
    <svg viewBox="0 0 110 110" class="donut-svg" role="img" :aria-label="`容量使用率 ${pct < 0 ? '未知' : pct + '%'}`">
      <circle cx="55" cy="55" :r="R" class="donut-track" />
      <circle
        v-if="pct >= 0"
        cx="55" cy="55" :r="R"
        class="donut-arc"
        :style="{ stroke: color, strokeDasharray: dash }"
        transform="rotate(-90 55 55)"
      />
    </svg>
    <div class="donut-center">
      <div class="donut-pct" :style="{ color }">{{ pct < 0 ? "—" : pct + "%" }}</div>
      <div class="donut-sub" v-if="pct >= 0">已用 {{ fmtBytes(used) }}</div>
    </div>
  </div>
</template>

<style scoped>
.donut { position: relative; width: 108px; height: 108px; }
.donut-svg { width: 100%; height: 100%; display: block; }
.donut-track { fill: none; stroke: var(--panel-3); stroke-width: 11; }
.donut-arc {
  fill: none; stroke-width: 11; stroke-linecap: round;
  transition: stroke-dasharray .5s ease, stroke .3s;
}
.donut-center {
  position: absolute; inset: 0; display: flex; flex-direction: column;
  align-items: center; justify-content: center; gap: 1px; text-align: center;
}
.donut-pct { font-family: var(--font-mono); font-size: 19px; font-weight: 700; line-height: 1; }
.donut-sub { font-size: 10px; color: var(--ink-3); max-width: 84px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
