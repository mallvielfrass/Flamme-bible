<script setup>
import { onMounted, ref } from 'vue'
import { loadTwemoji } from '../../emoji'
import MdNode from './MdNode.vue'

const props = defineProps({
  node: { type: Object, required: true },
})

const src = ref('')

onMounted(async () => {
  if (props.node.type !== 'emoji') return
  src.value = await loadTwemoji(props.node.cps)
})
</script>

<template>
  <template v-if="node.type === 'text'">{{ node.text }}</template>
  <img v-else-if="node.type === 'emoji' && src" class="twemoji" :src="src" :alt="node.text" />
  <template v-else-if="node.type === 'emoji'">{{ node.text }}</template>
  <br v-else-if="node.type === 'br'" />
  <strong v-else-if="node.type === 'strong'"><MdNode v-for="(child, index) in node.children" :key="index" :node="child" /></strong>
  <em v-else-if="node.type === 'em'"><MdNode v-for="(child, index) in node.children" :key="index" :node="child" /></em>
  <s v-else-if="node.type === 's'"><MdNode v-for="(child, index) in node.children" :key="index" :node="child" /></s>
  <code v-else-if="node.type === 'code'">{{ node.text }}</code>
  <pre v-else-if="node.type === 'pre'"><code>{{ node.text }}</code></pre>
  <a v-else-if="node.type === 'a'" :href="node.href" rel="noopener noreferrer" target="_blank"><MdNode v-for="(child, index) in node.children" :key="index" :node="child" /></a>
  <p v-else-if="node.type === 'p'"><MdNode v-for="(child, index) in node.children" :key="index" :node="child" /></p>
  <ul v-else-if="node.type === 'ul'"><MdNode v-for="(child, index) in node.children" :key="index" :node="child" /></ul>
  <ol v-else-if="node.type === 'ol'"><MdNode v-for="(child, index) in node.children" :key="index" :node="child" /></ol>
  <li v-else-if="node.type === 'li'"><MdNode v-for="(child, index) in node.children" :key="index" :node="child" /></li>
  <blockquote v-else-if="node.type === 'quote'"><MdNode v-for="(child, index) in node.children" :key="index" :node="child" /></blockquote>
  <component :is="`h${node.level || 2}`" v-else-if="node.type === 'h'"><MdNode v-for="(child, index) in node.children" :key="index" :node="child" /></component>
  <template v-else><MdNode v-for="(child, index) in node.children || []" :key="index" :node="child" /></template>
</template>
