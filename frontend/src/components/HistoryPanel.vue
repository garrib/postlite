<script setup lang="ts">
// 历史面板：平铺倒序、检索、回填、单删、清空（design.md §2.1）。
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

function fmtTime(ms: number): string {
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function statusOf(r: HistoryRecord): { label: string; cls: string } {
  if (r.error) return { label: '错误', cls: 'err' }
  const s = r.response?.status ?? 0
  if (s >= 200 && s < 300) return { label: String(s), cls: 's2' }
  if (s >= 300 && s < 400) return { label: String(s), cls: 's3' }
  if (s >= 400) return { label: String(s), cls: 's4' }
  return { label: s ? String(s) : '—', cls: 'err' }
}
</script>

<template>
  <div class="history-panel">
    <div class="history-head">
      <input v-model="q" type="text" placeholder="检索历史（URL / 方法）" />
      <button class="ghost" title="清空全部历史" @click="emit('clear')">清空</button>
    </div>
    <div class="history-list">
      <div v-for="rec in filtered" :key="rec.id" class="history-item" @click="emit('load', rec)">
        <div class="l1">
          <span class="m">{{ rec.request.method }}</span>
          <span class="st" :class="statusOf(rec).cls">{{ statusOf(rec).label }}</span>
          <span class="t" style="margin-left: 8px">{{ fmtTime(rec.createdAt) }}</span>
          <span style="flex: 1"></span>
          <button
            class="row-del ghost"
            title="删除"
            @click.stop="emit('delete', rec.id)"
            style="padding: 0 4px; font-size: 11px"
          >
            ✕
          </button>
        </div>
        <div class="u">{{ rec.request.url }}</div>
        <div v-if="rec.response?.previewText" class="t" style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
          预览：{{ rec.response.previewText.slice(0, 80) }}
        </div>
        <div v-else-if="rec.error" class="t" style="color: var(--danger)">{{ rec.error.slice(0, 80) }}</div>
      </div>
      <div v-if="filtered.length === 0" class="resp-empty" style="padding: 30px 10px; font-size: 12px">
        {{ items.length === 0 ? '最近 7 天的请求会出现在这里' : '无匹配记录' }}
      </div>
    </div>
  </div>
</template>
