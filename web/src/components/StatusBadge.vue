<script setup>
const props = defineProps({
  status: { type: String, default: '' },
  label: { type: String, default: '' },
})

const names = {
  pending: 'на проверке',
  approved: 'одобрено',
  rejected: 'отклонено',
  draft: 'черновик',
  live: 'у людей сейчас',
  released: 'опубликована',
  admin: 'админ',
  moderator: 'модератор',
  user: 'пользователь',
  blocked: 'заблокирован',
  hidden: 'скрыт',
  songbook: 'Сборник',
  module: 'Библия',
}

const tones = {
  pending: 'pending',
  approved: 'approved',
  rejected: 'rejected',
  draft: 'draft',
  live: 'live',
  released: 'approved',
  admin: 'admin',
  moderator: 'moderator',
  user: 'draft',
  blocked: 'rejected',
  hidden: 'draft',
  songbook: 'kind',
  module: 'kind',
}
</script>

<template>
  <span class="status-badge" :class="tones[props.status] || 'draft'">
    <i aria-hidden="true"></i>{{ props.label || names[props.status] || props.status }}
  </span>
</template>
