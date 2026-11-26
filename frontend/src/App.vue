<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import BodyEditor from './components/BodyEditor.vue'
import HistoryPanel from './components/HistoryPanel.vue'
import KvEditor from './components/KvEditor.vue'
import MethodSelect from './components/MethodSelect.vue'
import ResponseView from './components/ResponseView.vue'
import { ApiError, clearHistory, deleteHistory, listHistory, sendRequest } from './lib/api'
import { toCurl } from './lib/curlExport'
import { copyText, isNativeApp, onNativeSetOptions, postOptions } from './lib/platform'
import { parseCurl } from './lib/curlParse'
import { buildUrl, parseUrl, type ParamRow } from './lib/params'
import { useClickOutside } from './lib/useClickOutside'
import {
  emptyRequest,
  type ApiResponse,
  type Body,
  type Header,
  type HistoryRecord,
} from './lib/types'

// ———— 表单状态（唯一真源 = envelope；params 行为派生态） ————

const env = ref(emptyRequest())
const paramRows = ref<ParamRow[]>([])
const paramBase = ref('')
const paramHash = ref('')

const sending = ref(false)
const resp = ref<ApiResponse | null>(null)
const sendError = ref('')

// 左上角「设置」下拉：仅浏览器（CLI）形态显示；.app 内用系统菜单栏「设置…⌘,」原生面板。
const nativeApp = isNativeApp()
const settingsOpen = ref(false)
const settingsRoot = ref<HTMLElement | null>(null)
useClickOutside(settingsRoot, () => (settingsOpen.value = false))

// 原生面板 → Web：Swift evaluateJavaScript 调这里。
onNativeSetOptions((o) => {
  env.value.options = { ...env.value.options, ...o }
})
// Web → 原生面板：选项变更即推（初始挂载后也推一次）。
watch(
  () => env.value.options,
  (o) => {
    if (nativeApp) postOptions(o)
  },
  { deep: true },
)

const tab = ref<'params' | 'headers' | 'body'>('body')

// URL 与参数行双向同步（test-cases.md T1/T2）。
function reparseUrl() {
  const p = parseUrl(env.value.url)
  paramBase.value = p.base
  paramHash.value = p.hash
  paramRows.value = p.rows
}
// KvEditor 行（name/value）↔ ParamRow（key/value）转换。
const kvParamRows = computed(() => paramRows.value.map((r) => ({ name: r.key, value: r.value, enabled: r.enabled })))
function onParamKv(rows: Array<{ name: string; value: string; enabled: boolean }>) {
  const next: ParamRow[] = rows.map((r) => ({ key: r.name, value: r.value, enabled: r.enabled }))
  paramRows.value = next
  env.value.url = buildUrl(paramBase.value, next, paramHash.value)
}
function onUrlChange() {
  reparseUrl()
}

function onHeaders(rows: Header[]) {
  env.value.headers = rows.map((r) => ({ ...r }))
}
function onBody(b: Body) {
  env.value.body = b
}

// 发送（design.md §3.4 状态机：sending → done/error）。
async function send() {
  if (sending.value) return
  if (env.value.url.trim() === '') {
    toast('请填写 URL')
    return
  }
  sending.value = true
  sendError.value = ''
  resp.value = null
  try {
    resp.value = await sendRequest(env.value)
    void refreshHistory()
  } catch (e) {
    if (e instanceof ApiError) {
      sendError.value = `【${e.code}】${e.message}`
    } else {
      sendError.value = String(e)
    }
  } finally {
    sending.value = false
  }
}

// ———— cURL 导入 / 导出 ————

const showImport = ref(false)
const importText = ref('')
const importWarnings = ref<string[]>([])

function openImport() {
  importText.value = toCurl(env.value)
  importWarnings.value = []
  showImport.value = true
}

function applyImport() {
  const parsed = parseCurl(importText.value)
  const { warnings, ...parsedEnv } = parsed
  env.value = { ...parsedEnv, headers: parsedEnv.headers.map((h) => ({ ...h })), body: { ...parsedEnv.body } }
  reparseUrl()
  importWarnings.value = warnings
  if (warnings.length === 0) showImport.value = false
}

function closeImport() {
  showImport.value = false
}

async function copyCurl() {
  const ok = await copyText(toCurl(env.value))
  toast(ok ? '已复制 cURL 命令' : '复制失败')
}

// ———— 历史 ————

const history = ref<HistoryRecord[]>([])

async function refreshHistory() {
  try {
    history.value = await listHistory()
  } catch (e) {
    if (e instanceof ApiError) toast(`历史加载失败：${e.message}`)
  }
}

function loadHistory(rec: HistoryRecord) {
  env.value = {
    method: rec.request.method,
    url: rec.request.url,
    headers: rec.request.headers.map((h) => ({ ...h })),
    body: { ...rec.request.body },
    options: { ...rec.request.options },
  }
  reparseUrl()
  resp.value = null
  sendError.value = ''
  toast('已回填该历史请求')
}

async function onDeleteHistory(id: string) {
  try {
    await deleteHistory(id)
    history.value = history.value.filter((r) => r.id !== id)
  } catch {
    toast('删除失败')
  }
}

async function onClearHistory() {
  if (!window.confirm('清空全部历史（7 天内）？')) return
  try {
    await clearHistory()
    history.value = []
  } catch {
    toast('清空失败')
  }
}

// ———— 杂项 ————

const toastMsg = ref('')
let toastTimer: ReturnType<typeof setTimeout> | undefined
function toast(msg: string) {
  toastMsg.value = msg
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => (toastMsg.value = ''), 1600)
}

function onKeySend(ev: KeyboardEvent) {
  if ((ev.metaKey || ev.ctrlKey) && ev.key === 'Enter') {
    ev.preventDefault()
    void send()
  }
}

const enabledHeaderCount = computed(() => env.value.headers.filter((h) => h.enabled && h.name).length)
const enabledParamCount = computed(() => paramRows.value.filter((r) => r.enabled && r.key).length)

onMounted(() => {
  reparseUrl()
  if (nativeApp) postOptions(env.value.options)
  void refreshHistory()
})
</script>

<template>
  <div class="app-shell" @keydown="onKeySend">
    <div class="topbar">
      <div class="topbar-left">
        <div v-if="!nativeApp" ref="settingsRoot" class="settings-root">
          <button type="button" class="ghost settings-btn" @click="settingsOpen = !settingsOpen">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="3" />
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
            </svg>
            <span>设置</span>
            <span class="caret" :class="{ open: settingsOpen }">▾</span>
          </button>
          <div v-if="settingsOpen" class="settings-menu">
            <div class="settings-title">发送选项</div>
            <label class="settings-row" title="跟随 3xx 重定向（curl -L）">
              <input type="checkbox" v-model="env.options.followRedirects" />
              <span>跟随重定向</span>
            </label>
            <label class="settings-row" title="跳过 TLS 证书校验（curl -k）">
              <input type="checkbox" v-model="env.options.insecureSkipVerify" />
              <span>跳过证书校验</span>
            </label>
            <div class="settings-row settings-timeout">
              <span>超时 ms</span>
              <input type="number" min="0" max="300000" step="100" v-model.number="env.options.timeoutMs" />
            </div>
            <div class="settings-hint">超时 0 = 不限（服务端硬顶 300s）</div>
          </div>
        </div>
        <div class="logo"><span class="brand-post">Post</span><span class="brand-lite">lite</span></div>
      </div>
      <div class="spacer"></div>
      <div class="version-tag mono">v0.2.1</div>
    </div>

    <div class="url-bar">
      <MethodSelect v-model="env.method" />
      <input
        class="url"
        v-model="env.url"
        type="text"
        placeholder="https://api.example.com/v1/anything"
        spellcheck="false"
        @change="onUrlChange"
      />
      <button class="primary" :disabled="sending" @click="send()">{{ sending ? '发送中…' : '发送 ⌘↵' }}</button>
      <button class="ghost" @click="openImport">导入 cURL</button>
      <button class="ghost" @click="copyCurl">复制 cURL</button>
    </div>

    <div class="main-split">
      <HistoryPanel
        :items="history"
        @load="loadHistory"
        @delete="onDeleteHistory"
        @clear="onClearHistory"
      />

      <div class="workbench">
        <div class="request-pane">
          <div class="tabs">
            <div class="tab" :class="{ active: tab === 'params' }" @click="tab = 'params'">
              Params<span v-if="enabledParamCount" class="count">{{ enabledParamCount }}</span>
            </div>
            <div class="tab" :class="{ active: tab === 'headers' }" @click="tab = 'headers'">
              Headers<span v-if="enabledHeaderCount" class="count">{{ enabledHeaderCount }}</span>
            </div>
            <div class="tab" :class="{ active: tab === 'body' }" @click="tab = 'body'">Body</div>
          </div>
          <div class="pane-body">
            <KvEditor
              v-if="tab === 'params'"
              :rows="kvParamRows"
              key-placeholder="参数名"
              value-placeholder="值"
              @update:rows="onParamKv"
            />
            <KvEditor
              v-else-if="tab === 'headers'"
              :rows="env.headers"
              key-placeholder="Header 名"
              value-placeholder="值"
              @update:rows="onHeaders"
            />
            <BodyEditor v-else :body="env.body" @update:body="onBody" />
          </div>
        </div>

        <div class="response-pane">
          <div v-if="sending" class="resp-empty">发送中…</div>
          <div v-else-if="sendError" class="error-box">{{ sendError }}</div>
          <ResponseView v-else :resp="resp" />
        </div>
      </div>
    </div>

    <div v-if="showImport" class="modal-mask" @click.self="closeImport">
      <div class="modal">
        <h3>粘贴 cURL 命令（支持 bash / cmd / PowerShell 复制出来的格式）</h3>
        <textarea
          v-model="importText"
          spellcheck="false"
          placeholder="curl 'https://api.example.com/x' -H 'Accept: application/json' --data-raw '{...}'"
        ></textarea>
        <div v-if="importWarnings.length" class="warn-list">
          <div v-for="(w, i) in importWarnings" :key="i">⚠ {{ w }}</div>
        </div>
        <div class="actions">
          <button class="ghost" @click="closeImport">取消</button>
          <button class="primary" @click="applyImport">解析并应用</button>
        </div>
      </div>
    </div>

    <div v-if="toastMsg" class="toast">{{ toastMsg }}</div>
  </div>
</template>
