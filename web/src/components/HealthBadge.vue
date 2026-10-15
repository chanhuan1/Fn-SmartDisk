<script setup lang="ts">
import { computed } from "vue";
import { healthClass, type HealthGrade } from "../api";

const props = defineProps<{ grade: HealthGrade; small?: boolean }>();
const cls = computed(() => `badge ${healthClass(props.grade)}`);
const label = computed(() => {
  switch (props.grade) {
    case "优": return "健康";
    case "良": return "良好";
    case "警": return "警告";
    case "危": return "危险";
    default: return "未知";
  }
});
</script>

<template>
  <span :class="cls"><span class="dot"></span>{{ label }}</span>
</template>
