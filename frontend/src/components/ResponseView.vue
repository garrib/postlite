<script setup lang="ts">
// 响应查看器：状态、耗时、大小、响应头、Body（Pretty / Raw / 复制 / 下载走平台桥）。
import { computed, ref } from 'vue'
import { highlightJson } from '../lib/highlightJson'
import { copyText, filenameFor, platformDownload } from '../lib/platform'
import type { ApiResponse } from '../lib/types'

const props = defineProps<{ resp: ApiResponse | null }>()

const view = ref<'body' | 'headers'>('body')
const mode = ref<'pretty' | 'raw'>('pretty')
const toast = ref('')
let toastTimer: ReturnType<typeof setTimeout> | undefined

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

// 合法 JSON 且 Pretty 模式 → 语法高亮 HTML（highlightJson 已先转义，v-html 安全）。
const prettyHtml = computed(() => {
  const r = props.resp
  if (!r || !r.isText || mode.value !== 'pretty') return null
  try {
    return highlightJson(JSON.stringify(JSON.parse(r.bodyText), null, 2))
  } catch {
    return null // 非法 JSON 回退纯文本
  }
})

const sizeLabel = computed(() => {
  const n = props.resp?.sizeBytes ?? 0
  if (n >= 1024 * 1024) return `${(n / 1024 / 1024).toFixed(2)} MB`
  if (n >= 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${n} B`
})

function showToast(msg: string) {
  toast.value = msg
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => (toast.value = ''), 1200)
}

async function copyBody() {
  if (!props.resp) return
  const ok = await copyText(prettyBody.value ?? '')
  showToast(ok ? '已复制' : '复制失败')
}

function download() {
  const r = props.resp
  if (!r) return
  let ct = ''
  for (const h of r.headers) {
    if (h.name.toLowerCase() === 'content-type') {
      ct = h.value
      break
    }
  }
  platformDownload(filenameFor(ct), r.bodyBase64)
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
      <pre v-if="prettyHtml !== null" class="resp-body" v-html="prettyHtml"></pre>
      <pre v-else-if="resp.isText" class="resp-body">{{ prettyBody }}</pre>
      <div v-else class="resp-empty">
        二进制内容（{{ sizeLabel }}），点右上「下载」保存。
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
