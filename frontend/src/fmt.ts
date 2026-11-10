// 展示格式化:时间 / 耗时 / 大小。

export function ftime(s?: string): string {
  if (!s) return '—'
  const d = new Date(s)
  if (isNaN(+d)) return '—'
  return d.toLocaleString('zh-CN', { hour12: false })
}

export function dur(ns?: number): string {
  if (!ns && ns !== 0) return '—'
  const sec = Math.round(ns / 1e9)
  if (sec < 60) return `${sec}s`
  if (sec < 3600) return `${Math.floor(sec / 60)}m${sec % 60}s`
  return `${Math.floor(sec / 3600)}h${Math.floor((sec % 3600) / 60)}m`
}

export function human(n?: number): string {
  if (n === undefined || n === null) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export function uptime(sec: number): string {
  const s = Math.floor(sec)
  if (s < 60) return `${s} 秒`
  if (s < 3600) return `${Math.floor(s / 60)} 分 ${s % 60} 秒`
  return `${Math.floor(s / 3600)} 时 ${Math.floor((s % 3600) / 60)} 分`
}

// triggerLabel 把运行记录的触发方式映射为中文展示。
export function triggerLabel(t?: string): string {
  if (t === 'schedule') return '定时'
  if (t === 'manual') return '手动'
  return t ?? '—'
}
