import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { del, get, mutationMessage, post, put, errMsg } from '../../api'
import { Empty, Callout } from '../../components/ui'
import type { Config, ListResp, MutationResult, Task } from '../../types'

export default function Tasks() {
  const [items, setItems] = useState<Task[]>([])
  const [params, setParams] = useSearchParams()
  const editing = params.get('edit')
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')

  const load = useCallback(() => {
    get<ListResp<Task>>('/api/tasks').then((r) => setItems(r.items)).catch((e) => setErr(errMsg(e)))
  }, [])
  useEffect(load, [load])

  const remove = async (name: string) => {
    if (!window.confirm(`删除任务 "${name}"?(去重状态文件保留)`)) return
    try {
      const res = await del<MutationResult>(`/api/tasks/${encodeURIComponent(name)}`)
      const m = mutationMessage(res)
      setMsg(m.msg ?? '')
      setErr(m.err ?? '')
      load()
    } catch (e) {
      setErr(errMsg(e))
    }
  }

  if (editing !== null) {
    return <TaskForm editing={editing} onDone={() => { setParams({}); load() }} />
  }

  return (
    <div>
      <Callout msg={msg} err={err} />
      <div className="flex items-center justify-between mb-2">
        <p className="text-xs text-slate-500 m-0">任务 = {'{主题, 推送目标, 调度表达式}'};保存即校验引用与 cron 并热生效。</p>
        <button className="btn btn-sm" onClick={() => setParams({ edit: 'new' })}>+ 新建</button>
      </div>
      {items.length > 0 ? (
        <div className="card overflow-x-auto"><table className="tbl">
          <thead><tr><th>名称</th><th>主题</th><th>推送目标</th><th>调度</th><th>操作</th></tr></thead>
          <tbody>
            {items.map((t) => (
              <tr key={t.name}>
                <td><b>{t.name}</b></td>
                <td><span className="mono">{t.topic}</span></td>
                <td>{t.targets.join(', ')}</td>
                <td>{t.schedules.map((s, i) => <div key={i}><span className="mono">{s}</span></div>)}</td>
                <td className="whitespace-nowrap">
                  <button className="btn btn-ghost btn-sm" onClick={() => setParams({ edit: t.name })}>编辑</button>{' '}
                  <button className="btn btn-danger btn-sm" onClick={() => remove(t.name)}>删除</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table></div>
      ) : <Empty>还没有任务;新建一个,把主题和推送目标组合成定时推送</Empty>}
    </div>
  )
}

function TaskForm({ editing, onDone }: { editing: string; onDone: () => void }) {
  const isNew = editing === 'new'
  const [t, setT] = useState<Task>({ name: '', topic: '', targets: [], schedules: ['0 8 * * *'] })
  const [topics, setTopics] = useState<string[]>([])
  const [allTargets, setAllTargets] = useState<string[]>([])
  const [err, setErr] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    get<Config>('/api/config')
      .then((c) => {
        setTopics(c.topics.map((x) => x.name))
        setAllTargets(c.targets.map((x) => x.name))
        if (isNew) {
          setT((p) => ({ ...p, topic: c.topics[0]?.name ?? '' }))
        } else {
          const found = c.tasks.find((x) => x.name === editing)
          if (found) setT(found)
          else setErr(`任务 "${editing}" 不存在`)
        }
      })
      .catch((e) => setErr(errMsg(e)))
  }, [editing, isNew])

  const save = async () => {
    setSaving(true)
    setErr('')
    try {
      await (isNew
        ? post<MutationResult>('/api/tasks', t)
        : put<MutationResult>(`/api/tasks/${encodeURIComponent(editing)}`, t))
      onDone()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const toggleTarget = (name: string) => {
    setT((p) => ({
      ...p,
      targets: p.targets.includes(name) ? p.targets.filter((x) => x !== name) : [...p.targets, name],
    }))
  }

  return (
    <div>
      <Callout err={err} />
      <form
        className="card p-5 max-w-2xl"
        onSubmit={(e) => {
          e.preventDefault()
          save()
        }}
      >
        {isNew ? (
          <>
            <label className="label">名称(唯一标识;保存后不可改,也用作去重状态文件名)</label>
            <input className="input" value={t.name} onChange={(e) => setT({ ...t, name: e.target.value })} placeholder="如 ai-agent-daily" />
          </>
        ) : (
          <>
            <label className="label">名称</label>
            <span className="mono">{editing}</span>
          </>
        )}

        <label className="label">主题</label>
        <select className="input" value={t.topic} onChange={(e) => setT({ ...t, topic: e.target.value })}>
          {topics.length === 0 && <option value="">(暂无主题,请先到"主题"页新建)</option>}
          {topics.map((n) => <option key={n} value={n}>{n}</option>)}
        </select>

        <label className="label">推送目标(可多选)</label>
        {allTargets.length === 0 && <p className="text-xs text-slate-500 m-0">暂无推送目标,请先到"推送目标"页新建</p>}
        {allTargets.map((n) => (
          <label key={n} className="flex items-center gap-2 mt-1.5 text-[13px]">
            <input
              type="checkbox"
              className="accent-blue-600"
              checked={t.targets.includes(n)}
              onChange={() => toggleTarget(n)}
            />
            {n}
          </label>
        ))}

        <label className="label">调度表达式(每行一个 cron 五段表达式,本地时区;#开头为注释)</label>
        <textarea
          className="input font-mono text-xs min-h-[110px] leading-relaxed"
          value={t.schedules.join('\n')}
          onChange={(e) => setT({ ...t, schedules: e.target.value.split('\n') })}
          placeholder={'0 8 * * *\n30 20 * * *'}
        />

        <div className="flex items-center gap-3 mt-4 flex-wrap">
          <button className="btn" disabled={saving} type="submit">保存</button>
          <button className="btn btn-ghost" type="button" onClick={onDone}>取消</button>
          <span className="text-xs text-slate-500">保存即校验引用与 cron 并热生效</span>
        </div>
      </form>
    </div>
  )
}
