import type { ReactNode } from 'react'
import { IconAlert, IconCheck, IconChevron } from './icons'

type Tone = 'ok' | 'bad' | 'warn' | 'accent'

const toneText: Record<Tone, string> = {
  ok: 'text-ok',
  bad: 'text-bad',
  warn: 'text-warn',
  accent: 'text-accent',
}

/** 页头:标题 + 描述 + 右侧动作区。 */
export function PageHead({
  title,
  desc,
  actions,
}: {
  title: ReactNode
  desc?: ReactNode
  actions?: ReactNode
}) {
  return (
    <div className="page-head">
      <div className="min-w-0">
        <h1 className="page-title">{title}</h1>
        {desc && <p className="page-desc">{desc}</p>}
      </div>
      {actions && <div className="ml-auto flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  )
}

/** 仪表带:概览页的统计主角,数字直接落在面板上,细线分格。 */
export function StatBand({ items }: { items: { key: ReactNode; value: ReactNode; tone?: Tone }[] }) {
  return (
    <div className="statband">
      {items.map((it, i) => (
        <div className="statband-cell" key={i}>
          <div className="statband-key">{it.key}</div>
          <div className={`statband-val ${it.tone ? toneText[it.tone] : ''}`}>{it.value}</div>
        </div>
      ))}
    </div>
  )
}

/** 统计卡(次级场景)。 */
export function Stat({
  value,
  label,
  tone,
}: {
  value: ReactNode
  label: ReactNode
  tone?: Tone
}) {
  return (
    <div className="stat">
      <div className={`stat-value ${tone ? toneText[tone] : ''}`}>{value}</div>
      <div className="stat-label">{label}</div>
    </div>
  )
}

/** 状态徽章:状态点 + 文字。 */
export function StatusBadge({ status }: { status: string }) {
  const map: Record<string, { cls: string; text: string }> = {
    success: { cls: 'badge badge-success', text: '成功' },
    failed: { cls: 'badge badge-failed', text: '失败' },
    running: { cls: 'badge badge-running', text: '运行中' },
  }
  const m = map[status] ?? { cls: 'badge', text: status }
  return (
    <span className={m.cls}>
      <span className="dot" />
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
      {err ? <IconAlert className="mt-0.5 shrink-0" /> : <IconCheck className="mt-0.5 shrink-0" />}
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
    <section className="mt-8">
      <div className="mb-3 flex items-baseline gap-3">
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
          {i > 0 && <IconChevron width={12} height={12} className="text-ghost" />}
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
