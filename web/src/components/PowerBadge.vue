<script setup lang="ts">
import { computed } from "vue";
import { powerStateMeta } from "../api";
import { IconBolt, IconMoon, IconPowerOff, IconQuestion } from "../icons";

// Unified power-state chip with a state icon (bolt / moon / off / ?).
// Used in overview cards, detail header and the power table.
const props = withDefaults(
  defineProps<{ powerState?: string; hibernated?: boolean; pulse?: boolean }>(),
  { pulse: false },
);

const meta = computed(() => powerStateMeta(props.powerState, props.hibernated));
</script>

<template>
  <span class="pwb" :class="'pwb-' + meta.cls" :title="'电源状态：' + meta.text">
    <IconBolt v-if="meta.icon === 'bolt'" :size="11" />
    <IconMoon v-else-if="meta.icon === 'moon'" :size="11" />
    <IconPowerOff v-else-if="meta.icon === 'off'" :size="11" />
    <IconQuestion v-else :size="11" />
    <span class="pwb-t">{{ meta.text }}</span>
    <span v-if="pulse && meta.cls === 'on'" class="pwb-pulse"></span>
  </span>
</template>

<style scoped>
.pwb {
  display: inline-flex; align-items: center; gap: 5px;
  font-size: 11px; font-weight: 700; padding: 2.5px 9px;
  border-radius: 999px; border: 1px solid; white-space: nowrap;
  line-height: 1.35;
}
.pwb svg { flex: none; }
.pwb.pwb-on { color: var(--good); border-color: rgba(67,201,117,.4); background: rgba(67,201,117,.12); }
.pwb.pwb-idle { color: var(--fair); border-color: rgba(147,204,82,.35); background: rgba(147,204,82,.1); }
.pwb.pwb-sleep { color: var(--acc); border-color: rgba(63,193,212,.4); background: rgba(63,193,212,.12); }
.pwb.pwb-off { color: var(--ink-3); border-color: var(--line); background: var(--panel-3); }
.pwb.pwb-unk { color: var(--unknown); border-color: var(--line); background: var(--panel-3); }
.pwb-pulse {
  width: 5px; height: 5px; border-radius: 50%; background: currentColor;
  box-shadow: 0 0 6px currentColor; animation: pwb-blink 1.8s ease-in-out infinite;
}
@keyframes pwb-blink { 0%,100% { opacity: 1; } 50% { opacity: .3; } }
</style>
