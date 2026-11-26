<script setup lang="ts">
// 方法选择器（自绘下拉）：按钮态方法色粗体 + caret，列表逐项方法色 + 当前勾选。
// 键盘：Enter/Space 开合、↑↓ 移动、Enter 选中、Esc 关闭（design.md §4.3）。
import { computed, ref } from 'vue'
import { useClickOutside } from '../lib/useClickOutside'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']

const open = ref(false)
const active = ref(0) // 键盘高亮索引
const root = ref<HTMLElement | null>(null)
useClickOutside(root, () => close())

function close() {
  open.value = false
}

function toggle() {
  if (!open.value) active.value = METHODS.indexOf(props.modelValue)
  open.value = !open.value
}

function pick(m: string) {
  emit('update:modelValue', m)
  close()
}

function onKeydown(ev: KeyboardEvent) {
  if (ev.key === 'Escape') {
    close()
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
    active.value = Math.min(active.value + 1, METHODS.length - 1)
  } else if (ev.key === 'ArrowUp') {
    ev.preventDefault()
    active.value = Math.max(active.value - 1, 0)
  } else if (ev.key === 'Enter') {
    ev.preventDefault()
    pick(METHODS[active.value])
  }
}

const btnCls = computed(() => 'msel-' + props.modelValue.toLowerCase())
</script>

<template>
  <div ref="root" class="method-root" tabindex="-1" @keydown="onKeydown">
    <button type="button" class="method-btn" :class="btnCls" @click="toggle">
      <span class="mono method-label">{{ modelValue }}</span>
      <span class="caret" :class="{ open }">▾</span>
    </button>
    <div v-if="open" class="method-menu" role="listbox">
      <div
        v-for="(m, i) in METHODS"
        :key="m"
        class="method-item"
        :class="['msel-' + m.toLowerCase(), { active: i === active, current: m === modelValue }]"
        role="option"
        :aria-selected="m === modelValue"
        @mouseenter="active = i"
        @click="pick(m)"
      >
        <span class="mono">{{ m }}</span>
        <span v-if="m === modelValue" class="check">✓</span>
      </div>
    </div>
  </div>
</template>
