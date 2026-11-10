import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { errMsg, get, post } from '../api'
import { ftime, triggerLabel } from '../fmt'
import { IconEdit, IconPlay } from '../components/icons'
import { Crumb, Empty, Callout, PageHead, Section, StatusBadge, TableWrap } from '../components/ui'
import type { Config, MutationResult, NextRun, RunRecord, Subscription } from '../types'

interface SubscriptionDetailData extends Subscription {
  next?: NextRun
}

export default function SubscriptionDetail() {
  const { topic } = useParams()
  const [sub, setSub] = useState<SubscriptionDetailData | null>(null)
  const [cfg, setCfg] = useState<Config | null>(null)
  const [runs, setRuns] = useState<RunRecord[]>([])
  const [flash, setCallout] = useState('')
  const [err, setErr] = useState('')

  const load = () => {
    get<Config>('/api/config')
      .then((c) => {
        setCfg(c)
        const t = c.subscriptions.find((x) => x.topic === topic)
        if (!t) {
          setErr('订阅不存在')
          return
        }
        setSub((prev) => ({ ...(prev ?? {}), ...t }) as SubscriptionDetailData)
      })
      .catch((e) => setErr(errMsg(e)))
    get<{ nexts: NextRun[] }>('/api/overview')
      .then((ov) => setSub((prev) => (prev ? { ...prev, next: ov.nexts.find((n) => n.topic === topic) } : prev)))
      .catch(() => null)
    get<{ items: RunRecord[] }>(`/api/runs?topic=${encodeURIComponent(topic ?? '')}&limit=20`)
      .then((r) => setRuns(r.items))
      .catch(() => null)
  }
  useEffect(load, [topic])

  const runNow = async () => {
    setErr('')
    try {
      await post<MutationResult>(`/api/subscriptions/${encodeURIComponent(topic ?? '')}/run`)
      setCallout('已触发,后台执行中;稍后刷新查看进度')
      setTimeout(load, 1500)
    } catch (e) {
      setErr(errMsg(e))
    }
  }

  if (err && !sub) return <Callout err={err} />
  if (!sub || !cfg) return null
  const t = cfg.topics.find((x) => x.name === sub.topic)
  const targetObjs = sub.targets
    .map((n) => cfg.targets.find((x) => x.name === n))
    .filter((x): x is NonNullable<typeof x> => Boolean(x))

  return (
    <div>
      <Crumb items={[{ to: '/subscriptions', label: '订阅管理' }, { label: sub.topic }]} />
      <PageHead
        title={`订阅 ${sub.topic}`}
        desc={
          <>
            cron <span className="mono">{sub.cron}</span>
            {targetObjs.length > 0 && (
              <>
                ，推送目标{' '}
                {targetObjs.map((t, i) => (
                  <span key={t.name}>
                    {i > 0 && '、'}
                    <span className="mono">{t.name}</span>
                  </span>
                ))}
              </>
            )}
          </>
        }
        actions={
          <>
            <Link className="btn btn-ghost" to={`/subscriptions?edit=${encodeURIComponent(sub.topic)}`}>
              <IconEdit width={13} height={13} /> 编辑
            </Link>
            <button className="btn" onClick={runNow}>
              <IconPlay width={13} height={13} /> 立即运行
            </button>
          </>
        }
      />
      <Callout msg={flash} err={err} />

      {t && (
        <details className="card mt-5 p-4">
          <summary className="cursor-pointer text-[13px] font-medium text-dim">主题 prompt</summary>
          <pre className="mt-3 whitespace-pre-wrap break-words font-mono text-[12.5px] leading-relaxed text-dim">
            {t.prompt}
          </pre>
        </details>
      )}

      <Section title="调度">
        {sub.next ? (
          <TableWrap>
            <table className="tbl">
              <thead>
                <tr>
                  <th>cron</th>
                  <th>下次触发</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td>
                    <span className="mono">{sub.next.cron}</span>
                  </td>
                  <td className="text-dim">{ftime(sub.next.next)}</td>
                </tr>
              </tbody>
            </table>
          </TableWrap>
        ) : (
          <Empty>无(serve 启动后显示)</Empty>
        )}
      </Section>

      <Section title="最近运行">
        {runs.length > 0 ? (
          <TableWrap>
            <table className="tbl">
              <thead>
                <tr>
                  <th>#</th>
                  <th>触发</th>
                  <th>状态</th>
                  <th>开始</th>
                  <th>报告 / 错误</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((r) => (
                  <tr key={r.id}>
                    <td className="font-mono text-[12px] text-ghost">{r.id}</td>
                    <td className="text-[12px] text-ghost">{triggerLabel(r.trigger)}</td>
                    <td>
                      <StatusBadge status={r.status} />
                    </td>
                    <td className="text-[12px] text-ghost">{ftime(r.started_at)}</td>
                    <td>
                      {r.subject ? (
                        <Link className="text-accent no-underline hover:underline" to={`/runs/${r.id}`}>
                          {r.subject}
                        </Link>
                      ) : r.error ? (
                        <Link className="text-[12px] text-ghost no-underline hover:underline" to={`/runs/${r.id}`}>
                          {r.error.slice(0, 60)}
                        </Link>
                      ) : (
                        '—'
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableWrap>
        ) : (
          <Empty>暂无</Empty>
        )}
      </Section>

      <Section title="产物">
        <Link className="text-[13px] text-accent no-underline hover:underline" to={`/artifacts?topic=${encodeURIComponent(sub.topic)}`}>
          查看该订阅的产物
        </Link>
      </Section>
    </div>
  )
}
