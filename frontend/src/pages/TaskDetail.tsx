import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { errMsg, get, post } from '../api'
import { ftime } from '../fmt'
import { Crumb, Empty, Callout, Section, StatusBadge } from '../components/ui'
import type { Config, MutationResult, NextRun, RunRecord, Task } from '../types'

interface TaskDetailData extends Task {
  nexts: NextRun[]
}

export default function TaskDetail() {
  const { name } = useParams()
  const [task, setTask] = useState<TaskDetailData | null>(null)
  const [cfg, setCfg] = useState<Config | null>(null)
  const [runs, setRuns] = useState<RunRecord[]>([])
  const [flash, setCallout] = useState('')
  const [err, setErr] = useState('')

  const load = () => {
    get<Config>('/api/config')
      .then((c) => {
        setCfg(c)
        const t = c.tasks.find((x) => x.name === name)
        if (!t) {
          setErr('任务不存在')
          return
        }
        setTask((prev) => ({ ...(prev ?? { nexts: [] }), ...t }))
      })
      .catch((e) => setErr(errMsg(e)))
    get<{ nexts: NextRun[] }>('/api/overview')
      .then((ov) => setTask((prev) => (prev ? { ...prev, nexts: ov.nexts.filter((n) => n.task === name) } : prev)))
      .catch(() => null)
    get<{ items: RunRecord[] }>(`/api/runs?task=${encodeURIComponent(name ?? '')}&limit=20`)
      .then((r) => setRuns(r.items))
      .catch(() => null)
  }
  useEffect(load, [name])

  const runNow = async () => {
    setErr('')
    try {
      await post<MutationResult>(`/api/tasks/${encodeURIComponent(name ?? '')}/run`)
      setCallout('已触发,后台执行中;稍后刷新查看进度')
      setTimeout(load, 1500)
    } catch (e) {
      setErr(errMsg(e))
    }
  }

  if (err && !task) return <Callout err={err} />
  if (!task || !cfg) return null
  const topic = cfg.topics.find((t) => t.name === task.topic)
  const targetObjs = task.targets
    .map((n) => cfg.targets.find((t) => t.name === n))
    .filter((t): t is NonNullable<typeof t> => Boolean(t))

  return (
    <div>
      <Crumb items={[{ to: '/config', label: '配置' }, { to: '/config/tasks', label: '任务' }, { label: task.name }]} />
      <h1 className="text-[19px] font-semibold my-2">任务:{task.name}</h1>
      <p className="text-[13px]">
        主题:<span className="mono">{task.topic}</span>
        {targetObjs.length > 0 && (
          <> · 推送目标:{targetObjs.map((t, i) => (
            <span key={t.name}>{i > 0 && ', '}<span className="mono">{t.name}({t.to})</span></span>
          ))}</>
        )}
      </p>
      {topic && (
        <details className="card p-3 my-2">
          <summary className="cursor-pointer text-[13px] text-slate-500">主题 prompt</summary>
          <pre className="mt-2 whitespace-pre-wrap break-words text-[13px]">{topic.prompt}</pre>
        </details>
      )}

      <div className="flex items-center gap-3 my-3 flex-wrap">
        <button className="btn" onClick={runNow}>▶ 立即运行</button>
        <span className="text-xs text-slate-500">后台执行,刷新本页查看进度;运行中重复触发会被拒绝</span>
      </div>
      <Callout msg={flash} err={err} />

      <Section title="调度">
        {task.nexts.length > 0 ? (
          <div className="card overflow-x-auto"><table className="tbl">
            <thead><tr><th>cron</th><th>下次触发</th></tr></thead>
            <tbody>
              {task.nexts.map((n, i) => (
                <tr key={i}><td><span className="mono">{n.schedule}</span></td><td>{ftime(n.next)}</td></tr>
              ))}
            </tbody>
          </table></div>
        ) : <Empty>无(serve 启动后显示)</Empty>}
      </Section>

      <Section title="最近运行">
        {runs.length > 0 ? (
          <div className="card overflow-x-auto"><table className="tbl">
            <thead><tr><th>#</th><th>触发</th><th>状态</th><th>开始</th><th>报告 / 错误</th></tr></thead>
            <tbody>
              {runs.map((r) => (
                <tr key={r.id}>
                  <td className="text-slate-500">{r.id}</td>
                  <td className="text-slate-500">{r.trigger}</td>
                  <td><StatusBadge status={r.status} /></td>
                  <td className="text-slate-500">{ftime(r.started_at)}</td>
                  <td>
                    {r.subject ? (
                      <Link className="text-blue-600" to={`/runs/${r.id}`}>{r.subject}</Link>
                    ) : r.error ? (
                      <Link className="text-slate-500" to={`/runs/${r.id}`}>{r.error.slice(0, 60)}</Link>
                    ) : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table></div>
        ) : <Empty>暂无</Empty>}
      </Section>

      <Section title="邮件产物">
        <Link className="text-blue-600 text-[13px]" to={`/emails?task=${encodeURIComponent(task.name)}`}>查看该任务的产物 →</Link>
      </Section>
    </div>
  )
}
