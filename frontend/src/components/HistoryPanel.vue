<script setup lang="ts">
// 历史侧栏（Postman 风，design.md §4.3）：仅 方法+URL，按日分组（今天/昨天/M月D日），
// 不展示状态码与响应预览；删除按钮 hover 出现。
import { computed, ref } from 'vue'
import type { HistoryRecord } from '../lib/types'

const props = defineProps<{ items: HistoryRecord[] }>()
const emit = defineEmits<{
  (e: 'load', rec: HistoryRecord): void
  (e: 'delete', id: string): void
  (e: 'clear'): void
}>()

const q = ref('')

const filtered = computed(() => {
  const kw = q.value.trim().toLowerCase()
  if (kw === '') return props.items
  return props.items.filter(
    (r) => r.request.url.toLowerCase().includes(kw) || r.request.method.toLowerCase().includes(kw),
  )
})

// 按日分组（items 已按 createdAt 降序，分组顺序即组内顺序）。
interface DayGroup {
  label: string
  items: HistoryRecord[]
}
const groups = computed<DayGroup[]>(() => {
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const day = 24 * 3600 * 1000
  const out: DayGroup[] = []
  let cur: DayGroup | null = null
  let curKey = ''
  for (const r of filtered.value) {
    const d = new Date(r.createdAt)
    const start = new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
    const diffDays = Math.round((today - start) / day)
    const yearTag = d.getFullYear() !== now.getFullYear() ? `${d.getFullYear()}年` : ''
    const label =
      diffDays === 0 ? '今天' : diffDays === 1 ? '昨天' : `${yearTag}${d.getMonth() + 1}月${d.getDate()}日`
    const key = `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
    if (key !== curKey) {
      cur = { label, items: [] }
      curKey = key
      out.push(cur)
    }
    cur!.items.push(r)
  }
  return out
})
</script>

<template>
  <div class="history-panel">
    <div class="history-head">
      <div class="hh-input">
        <input v-model="q" type="text" placeholder="检索历史（URL / 方法）" />
        <button class="hh-clear" title="清空全部历史" @click="emit('clear')">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="3 6 5 6 21 6" />
            <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
            <path d="M10 11v6" />
            <path d="M14 11v6" />
            <path d="M9 6V4a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2" />
          </svg>
        </button>
      </div>
    </div>
    <div class="history-list">
      <template v-for="g in groups" :key="g.label">
        <div class="hg-label">{{ g.label }}</div>
        <div v-for="rec in g.items" :key="rec.id" class="history-item" @click="emit('load', rec)">
          <span class="m" :class="'m-' + rec.request.method.toLowerCase()">{{ rec.request.method }}</span>
          <span class="u" :title="rec.request.url">{{ rec.request.url }}</span>
          <button class="hd row-del ghost" title="删除" @click.stop="emit('delete', rec.id)">✕</button>
        </div>
      </template>
      <div v-if="filtered.length === 0" class="resp-empty" style="padding: 30px 10px; font-size: 12px">
        {{ items.length === 0 ? '最近 7 天的请求会出现在这里' : '无匹配记录' }}
      </div>
    </div>
  </div>
</template>
