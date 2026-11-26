<script setup lang="ts">
// 方法选择器 = PopSelect + 方法配色（design.md §4.3）；按钮宽度自适应，GET/POST 紧凑。
import PopSelect from './PopSelect.vue'
import type { PopOption } from '../lib/types'

defineProps<{ modelValue: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
const OPTIONS: PopOption[] = METHODS.map((m) => ({ value: m, label: m, cls: 'msel-' + m.toLowerCase() }))
</script>

<template>
  <PopSelect
    :model-value="modelValue"
    :options="OPTIONS"
    @update:model-value="emit('update:modelValue', $event)"
  />
</template>
