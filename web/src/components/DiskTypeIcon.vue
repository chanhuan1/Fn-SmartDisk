<script setup lang="ts">
import type { DeviceType } from "../api";
import { IconDisk, IconHdd, IconSsd, IconNvme } from "../icons";

// Per-media disk glyph: HDD platter / SSD stick / NVMe M.2 / generic USB box.
const props = withDefaults(defineProps<{ type: DeviceType; size?: number }>(), { size: 18 });
</script>

<template>
  <IconHdd v-if="props.type === 'HDD'" :size="props.size" />
  <IconSsd v-else-if="props.type === 'SSD'" :size="props.size" />
  <IconNvme v-else-if="props.type === 'NVMe'" :size="props.size" />
  <IconDisk v-else :size="props.size" />
</template>
