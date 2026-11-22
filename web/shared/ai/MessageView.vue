<script setup>
import { computed } from 'vue'
import { markdownTree } from './markdown.js'
import MdNode from './MdNode.vue'

const props = defineProps({
  text: { type: String, default: '' },
})

const nodes = computed(() => markdownTree(props.text))
</script>

<template>
  <div class="ai-md">
    <MdNode v-for="(node, index) in nodes" :key="index" :node="node" />
  </div>
</template>
