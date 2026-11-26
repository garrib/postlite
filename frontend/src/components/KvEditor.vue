<script setup lang="ts">
// 通用 KV 行编辑器：headers / query 参数 / x-www-form-urlencoded 共用。
import { computed } from 'vue'

const props = defineProps<{
  rows: Array<{ name: string; value: string; enabled: boolean }>
  keyPlaceholder?: string
  valuePlaceholder?: string
  addable?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:rows', v: Array<{ name: string; value: string; enabled: boolean }>): void
}>()

// 空表时展示一行引导空行（design.md §4.3）：输入即建行，不做纯空白。
const displayRows = computed(() =>
  props.rows.length === 0 ? [{ name: '', value: '', enabled: true }] : props.rows,
)

function set(i: number, patch: Partial<{ name: string; value: string; enabled: boolean }>) {
  if (props.rows.length === 0) {
    emit('update:rows', [{ name: '', value: '', enabled: true, ...patch }])
    return
  }
  const next = props.rows.map((r, idx) => (idx === i ? { ...r, ...patch } : r))
  emit('update:rows', next)
}

function add() {
  emit('update:rows', [...props.rows, { name: '', value: '', enabled: true }])
}

function del(i: number) {
  if (props.rows.length === 0) return // 引导行不可删
  emit('update:rows', props.rows.filter((_, idx) => idx !== i))
}

defineExpose({ add })
const showAdd = computed(() => props.addable !== false)
</script>

<template>
  <table class="kv-table">
    <tbody>
      <tr v-for="(r, i) in displayRows" :key="i">
        <td style="width: 22px">
          <input type="checkbox" :checked="r.enabled" @change="set(i, { enabled: !r.enabled })" title="启用/禁用此行" />
        </td>
        <td style="width: 40%">
          <input type="text" :value="r.name" :placeholder="keyPlaceholder ?? '名称'" @input="set(i, { name: ($event.target as HTMLInputElement).value })" />
        </td>
        <td>
          <input type="text" :value="r.value" :placeholder="valuePlaceholder ?? '值'" @input="set(i, { value: ($event.target as HTMLInputElement).value })" />
        </td>
        <td style="width: 30px">
          <button class="row-del" title="删除" @click="del(i)">✕</button>
        </td>
      </tr>
    </tbody>
  </table>
  <button v-if="showAdd" class="add-row ghost" @click="add">+ 添加一行</button>
</template>
