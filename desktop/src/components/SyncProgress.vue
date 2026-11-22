<script setup lang="ts">
import type { Progress } from "../commands";

defineProps<{ progress: Progress }>();
</script>

<template>
  <div>
    <div class="progress" role="progressbar" :aria-valuenow="progress.percentage" aria-valuemin="0" aria-valuemax="100">
      <span :style="{ width: progress.percentage + '%' }"></span>
    </div>
    <p class="progress-label">{{ progress.stage }}: {{ progress.current }} — {{ progress.percentage }}%</p>
  </div>
</template>
