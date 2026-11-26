<script setup lang="ts">
// 请求体编辑器：none / json / x-www-form-urlencoded / form-data / raw / binary（api.md §1）。
import { computed, ref } from 'vue'
import KvEditor from './KvEditor.vue'
import { fileToBase64 } from '../lib/api'
import type { Body, BodyField } from '../lib/types'

const props = defineProps<{ body: Body }>()
const emit = defineEmits<{ (e: 'update:body', v: Body): void }>()

const types = [
  { v: 'none', label: 'none' },
  { v: 'json', label: 'json' },
  { v: 'x-www-form-urlencoded', label: 'x-www-form-urlencoded' },
  { v: 'form-data', label: 'form-data' },
  { v: 'raw', label: 'raw' },
  { v: 'binary', label: 'binary' },
] as const

function patch(p: Partial<Body>) {
  emit('update:body', { ...props.body, ...p })
}

const jsonValid = computed(() => {
  if (props.body.type !== 'json' || props.body.text.trim() === '') return null
  try {
    JSON.parse(props.body.text)
    return true
  } catch {
    return false
  }
})

function formatJson() {
  try {
    patch({ text: JSON.stringify(JSON.parse(props.body.text), null, 2) })
  } catch {
    /* 不动：非法 JSON 保持原样 */
  }
}

const urlFields = computed(() => props.body.fields.filter((f) => f.type !== 'file'))
const formFields = computed(() => props.body.fields)

function setFields(fields: BodyField[]) {
  patch({ fields })
}

// KvEditor 产出纯 KV 行；回填成完整 BodyField（urlencoded 恒为文本字段）。
function onUrlRows(rows: Array<{ name: string; value: string; enabled: boolean }>) {
  setFields(
    rows.map((r) => ({
      name: r.name,
      value: r.value,
      enabled: r.enabled,
      type: 'text' as const,
      fileName: '',
      contentType: '',
      contentBase64: '',
    })),
  )
}

function setField(i: number, p: Partial<BodyField>) {
  setFields(props.body.fields.map((f, idx) => (idx === i ? { ...f, ...p } : f)))
}

function addField(type: 'text' | 'file') {
  setFields([
    ...props.body.fields,
    { name: '', value: '', enabled: true, type, fileName: '', contentType: '', contentBase64: '' },
  ])
}

function removeField(i: number) {
  setFields(props.body.fields.filter((_, idx) => idx !== i))
}

async function pickFile(i: number, ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (!file) return
  const b64 = await fileToBase64(file)
  setField(i, { contentBase64: b64, fileName: file.name, contentType: file.type || '' })
  ;(ev.target as HTMLInputElement).value = '' // 允许重复选择同一文件
}

// 清除文件内容（保留 file 类型的行）：回填时提示重新选择文件的场景。
function clearFile(i: number) {
  setField(i, { contentBase64: '', fileName: '', contentType: '' })
}

// binary body：单文件，存到 text（base64），fileName 不落信封（发送端只见 base64）。
const binaryName = ref('')
async function pickBinary(ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (!file) return
  binaryName.value = file.name
  patch({ text: await fileToBase64(file) })
}

function fieldSizeLabel(f: BodyField): string {
  if (f.contentBase64 === '') return ''
  const bytes = Math.floor((f.contentBase64.length * 3) / 4)
  return bytes > 1024 ? `${(bytes / 1024).toFixed(1)} KB` : `${bytes} B`
}
</script>

<template>
  <div class="body-editor">
    <div class="pill-tabs">
      <div
        v-for="t in types"
        :key="t.v"
        class="pill"
        :class="{ active: body.type === t.v }"
        @click="patch({ type: t.v })"
      >
        {{ t.label }}
      </div>
    </div>

    <div style="padding-top: 10px">
      <template v-if="body.type === 'none'">
        <div style="color: var(--text-dim)">该请求无 body。</div>
      </template>

      <template v-else-if="body.type === 'json' || body.type === 'raw'">
        <div v-if="body.type === 'json'" style="margin-bottom: 6px; display: flex; gap: 8px; align-items: center">
          <button @click="formatJson">格式化</button>
          <span v-if="jsonValid === false" style="color: var(--danger)">JSON 非法（仍可按原文发送）</span>
        </div>
        <textarea
          :value="body.text"
          placeholder="请求体文本"
          spellcheck="false"
          @input="patch({ text: ($event.target as HTMLTextAreaElement).value })"
        ></textarea>
        <div v-if="body.type === 'raw'" style="margin-top: 8px">
          <span style="color: var(--text-dim)">MIME：</span>
          <input class="mime-input" type="text" :value="body.mime" placeholder="缺省 text/plain" @input="patch({ mime: ($event.target as HTMLInputElement).value })" />
        </div>
      </template>

      <template v-else-if="body.type === 'x-www-form-urlencoded'">
        <KvEditor :rows="urlFields" key-placeholder="字段名" value-placeholder="值" @update:rows="onUrlRows" />
      </template>

      <template v-else-if="body.type === 'form-data'">
        <div class="fd-rows">
          <div v-for="(f, i) in formFields" :key="i" class="fd-row" :class="{ disabled: !f.enabled }">
            <input type="checkbox" :checked="f.enabled" title="启用/禁用此字段" @change="setField(i, { enabled: !f.enabled })" />
            <input class="fd-name" type="text" :value="f.name" placeholder="字段名" @input="setField(i, { name: ($event.target as HTMLInputElement).value })" />
            <select class="fd-type" :value="f.type" @change="setField(i, { type: ($event.target as HTMLSelectElement).value as 'text' | 'file' })">
              <option value="text">文本</option>
              <option value="file">文件</option>
            </select>
            <div class="fd-value">
              <template v-if="f.type === 'file'">
                <label v-if="!f.contentBase64" class="fd-file-btn">
                  选择文件
                  <input type="file" hidden @change="pickFile(i, $event)" />
                </label>
                <div v-else class="fd-file-chip">
                  <span class="fd-file-icon">📄</span>
                  <span class="fd-file-name" :title="f.fileName">{{ f.fileName }}</span>
                  <span class="fd-file-size mono">{{ fieldSizeLabel(f) }}</span>
                  <label class="fd-file-btn small">
                    换
                    <input type="file" hidden @change="pickFile(i, $event)" />
                  </label>
                  <button class="row-del" title="清除文件内容" @click="clearFile(i)">✕</button>
                </div>
              </template>
              <template v-else>
                <input type="text" :value="f.value" placeholder="值" @input="setField(i, { value: ($event.target as HTMLInputElement).value })" />
              </template>
            </div>
            <button class="row-del fd-del" title="删除此字段" @click="removeField(i)">✕</button>
          </div>
          <div v-if="formFields.length === 0" class="fd-empty">暂无字段——用下方按钮添加</div>
        </div>
        <div class="fd-actions">
          <button class="ghost" @click="addField('text')">＋ 文本字段</button>
          <button class="ghost" @click="addField('file')">＋ 文件字段</button>
        </div>
      </template>

      <template v-else-if="body.type === 'binary'">
        <div style="display: flex; gap: 10px; align-items: center">
          <input type="file" @change="pickBinary($event)" />
          <span v-if="binaryName" class="mono" style="font-size: 12px; color: var(--text-dim)">{{ binaryName }}（{{ Math.floor(body.text.length * 0.75) }} B）</span>
        </div>
        <div style="color: var(--text-dim); margin-top: 8px">二进制以 base64 存于信封 text 字段发送。</div>
      </template>
    </div>
  </div>
</template>
