<script setup lang="ts">
defineProps<{
  plans: {
    path: string;
    name: string;
    mojibake?: boolean;
    encoding_warning?: string;
    preview_before?: string;
    preview_after?: string;
  }[];
}>();
const emit = defineEmits<{ repair: [string] }>();
</script>

<template>
  <article v-for="plan in plans.filter((item) => item.mojibake)" :key="plan.path" class="conflict-card">
    <p class="error">{{ plan.encoding_warning }}</p>
    <p class="hint">{{ plan.name }}</p>
    <p class="hint">До</p>
    <p>{{ plan.preview_before }}</p>
    <p class="hint">После</p>
    <p>{{ plan.preview_after }}</p>
    <button class="btn" type="button" @click="emit('repair', plan.path)">Починить кодировку</button>
  </article>
</template>
