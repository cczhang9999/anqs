<script setup lang="ts">
import { propTypes } from '@/utils/propTypes'
import { computed } from 'vue'
import { hasComponentPermission } from '@/directives/permission/hasPermi'

const props = defineProps({
  permission: propTypes.string.def()
})

const hasPermission = computed(() => {
  return !props.permission || hasComponentPermission(props.permission)
})
</script>

<template>
  <template v-if="hasPermission">
    <slot></slot>
  </template>
</template>
