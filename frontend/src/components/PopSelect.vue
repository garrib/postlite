<script setup lang="ts">
// 通用自绘下拉（design.md §4.3）：按钮态当前项 + caret，列表逐项可着色 + 当前勾选。
// 点击外部/Esc 关闭，↑↓ + Enter 键盘可达。方法选择器与 form-data 类型下拉共用。
import { computed, ref } from 'vue'
import { useClickOutside } from '../lib/useClickOutside'
import type { PopOption } from '../lib/types'

const props = defineProps<{
  modelValue: string
  options: PopOption[]
}>()

const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const open = ref(false)
const active = ref(0)
const root = ref<HTMLElement | null>(null)
useClickOutside(root, () => (open.value = false))

const current = computed(() => props.options.find((o) => o.value === props.modelValue))

function toggle() {
  if (!open.value) active.value = Math.max(props.options.findIndex((o) => o.value === props.modelValue), 0)
  open.value = !open.value
}

function pick(o: PopOption) {
  emit('update:modelValue', o.value)
  open.value = false
}

function onKeydown(ev: KeyboardEvent) {
  if (ev.key === 'Escape') {
    open.value = false
    return
  }
  if (!open.value) {
    if (ev.key === 'Enter' || ev.key === ' ') {
      ev.preventDefault()
      toggle()
    }
    return
  }
  if (ev.key === 'ArrowDown') {
    ev.preventDefault()
    active.value = Math.min(active.value + 1, props.options.length - 1)
  } else if (ev.key === 'ArrowUp') {
    ev.preventDefault()
    active.value = Math.max(active.value - 1, 0)
  } else if (ev.key === 'Enter') {
    ev.preventDefault()
    pick(props.options[active.value])
  }
}
</script>

<template>
  <div ref="root" class="pop-root" tabindex="-1" @keydown="onKeydown">
    <button type="button" class="pop-btn" :class="current?.cls" @click="toggle">
      <span class="mono">{{ current?.label ?? modelValue }}</span>
      <span class="caret" :class="{ open }">▾</span>
    </button>
    <div v-if="open" class="pop-menu" role="listbox">
      <div
        v-for="(o, i) in options"
        :key="o.value"
        class="pop-item"
        :class="[o.cls, { active: i === active, current: o.value === modelValue }]"
        role="option"
        :aria-selected="o.value === modelValue"
        @mouseenter="active = i"
        @click="pick(o)"
      >
        <span class="mono">{{ o.label }}</span>
        <span v-if="o.value === modelValue" class="check">✓</span>
      </div>
    </div>
  </div>
</template>
