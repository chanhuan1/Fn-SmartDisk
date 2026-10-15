<script setup lang="ts">
import { computed } from "vue";
import { tempThresholds, type DeviceType, type TempHistory } from "../api";

// Visual min–current–max temperature band using the SCT on-disk history.
// Scale runs from 0 to the media's critical temp so warn/crit markers line up
// with the same thresholds used across the app.
const props = defineProps<{ history: TempHistory; type: DeviceType }>();

const th = computed(() => tempThresholds(props.type));

const scale = computed(() =>
  Math.max(th.value.crit, props.history.max, props.history.current, 1),
);

function pos(t: number): number {
  return Math.max(0, Math.min(100, (t / scale.value) * 100));
}

const warnPos = computed(() => pos(th.value.warn));
const critPos = computed(() => pos(th.value.crit));
const hasMin = computed(() => props.history.min > 0);
const hasMax = computed(() => props.history.max > 0);
const curColor = computed(() => {
  const t = props.history.current;
  if (t >= th.value.crit) return "var(--crit)";
  if (t >= th.value.warn) return "var(--warn)";
  return "var(--good)";
});
</script>

<template>
  <div class="trb">
    <div
      class="trb-bar"
      role="img"
      :aria-label="`温度：当前 ${history.current || '-'}°C，历史 ${hasMin ? history.min : '-'} 至 ${hasMax ? history.max : '-'}°C`"
    >
      <span class="trb-tick warn" :style="{ left: warnPos + '%' }" :title="`警戒 ${th.warn}°C`"></span>
      <span class="trb-tick crit" :style="{ left: critPos + '%' }" :title="`临界 ${th.crit}°C`"></span>
      <span v-if="hasMin" class="trb-mk min" :style="{ left: pos(history.min) + '%' }" :title="`历史最低 ${history.min}°C`"></span>
      <span v-if="history.current > 0" class="trb-mk cur" :style="{ left: pos(history.current) + '%', background: curColor }" :title="`当前 ${history.current}°C`"></span>
      <span v-if="hasMax" class="trb-mk max" :style="{ left: pos(history.max) + '%' }" :title="`历史最高 ${history.max}°C`"></span>
    </div>
    <div class="trb-labels">
      <span class="min" v-if="hasMin">最低 {{ history.min }}°C</span>
      <span class="cur" v-if="history.current > 0" :style="{ color: curColor }">当前 {{ history.current }}°C</span>
      <span class="max" v-if="hasMax">最高 {{ history.max }}°C</span>
    </div>
  </div>
</template>

<style scoped>
.trb { padding: 4px 2px 0; }
.trb-bar {
  position: relative; height: 10px; border-radius: 5px;
  background: linear-gradient(90deg, rgba(63,193,212,.28), rgba(67,201,117,.22) 45%, rgba(232,168,62,.26) 72%, rgba(236,91,82,.3));
  border: 1px solid var(--line-soft);
}
.trb-tick { position: absolute; top: -3px; bottom: -3px; width: 2px; border-radius: 1px; }
.trb-tick.warn { background: rgba(232,168,62,.75); }
.trb-tick.crit { background: rgba(236,91,82,.85); }
.trb-mk {
  position: absolute; top: 50%; width: 10px; height: 10px; border-radius: 50%;
  transform: translate(-50%, -50%); border: 2px solid var(--bg-1);
}
.trb-mk.min { background: var(--acc); }
.trb-mk.cur { width: 13px; height: 13px; box-shadow: 0 0 8px currentColor; z-index: 2; }
.trb-mk.max { background: var(--warn); }
.trb-labels { display: flex; justify-content: space-between; margin-top: 7px; font-size: 11.5px; font-family: var(--font-mono); }
.trb-labels .min { color: var(--acc); }
.trb-labels .max { color: var(--warn); }
.trb-labels:empty { display: none; }
</style>
