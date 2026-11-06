import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { get, errMsg } from '../api'
import { ftime, dur } from '../fmt'
import { Crumb, Callout, StatusBadge } from '../components/ui'
import type { RunRecord } from '../types'

export default function RunDetail() {
  const { id } = useParams()
  const [r, setR] = useState<RunRecord | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    get<RunRecord>(`/api/runs/${id}`).then(setR).catch((e) => setErr(errMsg(e)))
  }, [id])

  if (err) return <Callout err={err} />
  if (!r) return null
  return (
    <div>
      <Crumb items={[{ to: '/', label: '概览' }, { to: `/tasks/${r.task}`, label: r.task }, { label: `运行 #${r.id}` }]} />
      <h1 className="text-[19px] font-semibold my-2 flex items-center gap-3">
        运行 #{r.id} <StatusBadge status={r.status} />
      </h1>
      <p className="text-xs text-slate-500">
        任务 <Link className="text-blue-600" to={`/tasks/${r.task}`}>{r.task}</Link> · 触发 {r.trigger} · 开始 {ftime(r.started_at)}
        {r.finished_at && <> · 结束 {ftime(r.finished_at)} · 耗时 {dur(r.duration)}</>}
      </p>
      {r.subject && <h2 className="text-[15px] mt-4">{r.subject}</h2>}
      {r.error && <div className="flash flash-err">{r.error}</div>}
      {r.body && (
        <pre className="mt-3 bg-slate-900 dark:bg-black text-slate-200 rounded-xl p-4 overflow-auto max-h-[70vh] whitespace-pre-wrap break-words text-[13px] leading-relaxed">
          {r.body}
        </pre>
      )}
      <p className="mt-3"><Link className="text-blue-600" to="/emails">查看邮件产物 →</Link></p>
    </div>
  )
}
