import type { ReactNode } from 'react'
import { IconAlert, IconCheck, IconChevron } from './icons'

/** 统计卡片。 */
export function Stat({
  value,
  label,
  tone,
}: {
  value: ReactNode
  label: ReactNode
  tone?: 'ok' | 'bad' | 'warn' | 'brand'
}) {
  const color =
    tone === 'ok'
      ? 'text-emerald-600 dark:text-emerald-400'
      : tone === 'bad'
        ? 'text-red-600 dark:text-red-400'
        : tone === 'warn'
          ? 'text-amber-600 dark:text-amber-400'
          : tone === 'brand'
            ? 'text-[#4d6bfe]'
            : ''
  return (
    <div className="stat card-hover">
      <div className={`stat-value ${color}`}>{value}</div>
      <div className="stat-label">{label}</div>
    </div>
  )
}

/** 状态徽章。 */
export function StatusBadge({ status }: { status: string }) {
  const map: Record<string, { cls: string; text: string }> = {
    success: { cls: 'badge badge-success', text: '成功' },
    failed: { cls: 'badge badge-failed', text: '失败' },
    running: { cls: 'badge badge-running', text: '运行中' },
  }
  const m = map[status] ?? { cls: 'badge', text: status }
  return (
    <span className={m.cls}>
      {status === 'running' && <span className="dot" />}
      {m.text}
    </span>
  )
}

/** 空状态。 */
export function Empty({ children, action }: { children: ReactNode; action?: ReactNode }) {
  return (
    <div className="empty">
      <div>{children}</div>
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

/** 提示条(内联,非 toast)。 */
export function Callout({ msg, err }: { msg?: string; err?: string }) {
  if (!msg && !err) return null
  return (
    <div className={`callout${err ? ' callout-error' : ''}`}>
      {err ? <IconAlert className="shrink-0 mt-0.5" /> : <IconCheck className="shrink-0 mt-0.5" />}
      <span>{err ?? msg}</span>
    </div>
  )
}

/** 区块标题。 */
export function Section({
  title,
  right,
  children,
}: {
  title: ReactNode
  right?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="mt-7">
      <div className="flex items-center gap-3 mb-2.5">
        <h2 className="section-title">{title}</h2>
        <div className="ml-auto">{right}</div>
      </div>
      {children}
    </section>
  )
}

/** 面包屑。 */
export function Crumb({ items }: { items: { to?: string; label: string }[] }) {
  return (
    <div className="crumb">
      {items.map((it, i) => (
        <span key={i} className="flex items-center gap-1.5">
          {i > 0 && <IconChevron width={12} height={12} className="text-slate-300 dark:text-slate-700" />}
          {it.to ? <a href={it.to}>{it.label}</a> : <span>{it.label}</span>}
        </span>
      ))}
    </div>
  )
}

/** 表格容器(横向滚动 + 圆角边框)。 */
export function TableWrap({ children }: { children: ReactNode }) {
  return <div className="card overflow-x-auto">{children}</div>
}
