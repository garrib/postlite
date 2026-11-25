<script setup lang="ts">
// 响应查看器：状态、耗时、大小、响应头、Body（Pretty / Raw / 下载）。
import { computed, ref } from 'vue'
import type { ApiResponse } from '../lib/types'

const props = defineProps<{ resp: ApiResponse | null }>()

const view = ref<'body' | 'headers'>('body')
const mode = ref<'pretty' | 'raw'>('pretty')

const statusClass = computed(() => {
  const s = props.resp?.status ?? 0
  if (s >= 200 && s < 300) return 's2'
  if (s >= 300 && s < 400) return 's3'
  if (s >= 400 && s < 500) return 's4'
  if (s >= 500) return 's5'
  return ''
})

const prettyBody = computed(() => {
  const r = props.resp
  if (!r || !r.isText) return null
  if (mode.value === 'raw') return r.bodyText
  try {
    return JSON.stringify(JSON.parse(r.bodyText), null, 2)
  } catch {
    return r.bodyText
  }
})

const sizeLabel = computed(() => {
  const n = props.resp?.sizeBytes ?? 0
  if (n >= 1024 * 1024) return `${(n / 1024 / 1024).toFixed(2)} MB`
  if (n >= 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${n} B`
})

function copyBody() {
  if (!props.resp) return
  const text = prettyBody.value ?? ''
  void navigator.clipboard.writeText(text)
  emitToast()
}

function download() {
  const r = props.resp
  if (!r) return
  const bin = atob(r.bodyBase64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  const blob = new Blob([bytes])
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = 'response.bin'
  a.click()
  URL.revokeObjectURL(a.href)
}

const toast = ref('')
function emitToast() {
  toast.value = '已复制'
  setTimeout(() => (toast.value = ''), 1200)
}
</script>

<template>
  <div v-if="!resp" class="resp-empty">发送请求后这里显示响应</div>
  <template v-else>
    <div class="resp-meta">
      <span class="status-pill" :class="statusClass">{{ resp.status }} {{ resp.statusText }}</span>
      <span>{{ resp.timeMs }} ms</span>
      <span>{{ sizeLabel }}<span v-if="resp.truncated" title="响应超过 20MB，已截断展示">（已截断）</span></span>
      <span v-if="!resp.isText" style="color: var(--warn)">二进制响应</span>
      <span style="flex: 1"></span>
      <button class="ghost" @click="copyBody">复制</button>
      <button class="ghost" @click="download">下载</button>
    </div>
    <div class="tabs">
      <div class="tab" :class="{ active: view === 'body' }" @click="view = 'body'">Body</div>
      <div class="tab" :class="{ active: view === 'headers' }" @click="view = 'headers'">
        Headers<span class="count">{{ resp.headers.length }}</span>
      </div>
    </div>
    <template v-if="view === 'body'">
      <div v-if="resp.isText" class="resp-meta" style="border-bottom: none; padding-bottom: 0">
        <div class="tab" :class="{ active: mode === 'pretty' }" @click="mode = 'pretty'">Pretty</div>
        <div class="tab" :class="{ active: mode === 'raw' }" @click="mode = 'raw'">Raw</div>
      </div>
      <pre v-if="resp.isText" class="resp-body">{{ prettyBody }}</pre>
      <div v-else class="resp-empty">
        二进制内容（{{ sizeLabel }}），点右上「下载」保存；预览用 hex 见后续版本。
      </div>
    </template>
    <div v-else class="pane-body">
      <table class="kv-table">
        <tbody>
          <tr v-for="(h, i) in resp.headers" :key="i">
            <td class="mono" style="width: 30%; color: var(--text-dim)">{{ h.name }}</td>
            <td class="mono" style="word-break: break-all">{{ h.value }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </template>
  <div v-if="toast" class="toast">{{ toast }}</div>
</template>
