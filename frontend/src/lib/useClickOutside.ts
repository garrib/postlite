// 点击容器外部时触发回调（下拉菜单通用）。
import { onBeforeUnmount, onMounted, type Ref } from 'vue'

export function useClickOutside(root: Ref<HTMLElement | null>, cb: () => void) {
  const handler = (ev: MouseEvent) => {
    const el = root.value
    if (el && !el.contains(ev.target as Node)) cb()
  }
  onMounted(() => document.addEventListener('mousedown', handler))
  onBeforeUnmount(() => document.removeEventListener('mousedown', handler))
}
