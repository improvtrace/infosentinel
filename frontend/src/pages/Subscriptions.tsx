import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { del, get, post, put, mutationMessage, errMsg } from '../api'
import { ftime } from '../fmt'
import { IconPlay } from '../components/icons'
import { Empty, Callout, PageHead } from '../components/ui'
import { useToast } from '../components/Toast'
import type { Config, ListResp, MutationResult, NextRun, Overview, Subscription } from '../types'

export default function Subscriptions() {
  const [items, setItems] = useState<Subscription[]>([])
  const [nexts, setNexts] = useState<NextRun[]>([])
  const [params, setParams] = useSearchParams()
  const editing = params.get('edit')
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')
  const toast = useToast()

  const load = useCallback(() => {
    get<ListResp<Subscription>>('/api/subscriptions').then((r) => setItems(r.items)).catch((e) => setErr(errMsg(e)))
    get<Overview>('/api/overview').then((o) => setNexts(o.nexts)).catch(() => null)
  }, [])
  useEffect(load, [load])

  const runNow = async (topic: string) => {
    try {
      await post<MutationResult>(`/api/subscriptions/${encodeURIComponent(topic)}/run`)
      toast.ok(`已触发订阅 ${topic},后台执行中`)
      setTimeout(load, 1200)
    } catch (e) {
      toast.err(errMsg(e))
    }
  }

  const remove = async (topic: string) => {
    if (!window.confirm(`删除订阅 "${topic}"?(去重状态文件保留)`)) return
    try {
      const res = await del<MutationResult>(`/api/subscriptions/${encodeURIComponent(topic)}`)
      const m = mutationMessage(res)
      setMsg(m.msg ?? '')
      setErr(m.err ?? '')
      load()
    } catch (e) {
      setErr(errMsg(e))
    }
  }

  if (editing !== null) {
    return <SubscriptionForm editing={editing} onDone={() => { setParams({}); load() }} />
  }

  const nextAt = (topic: string) => {
    const n = nexts.find((x) => x.topic === topic)
    return n ? ftime(n.next) : '—'
  }

  return (
    <div>
      <PageHead
        title="订阅管理"
        desc="一个订阅 = 主题 + 一个 cron 调度 + 多个推送目标;主题即订阅身份,保存即校验并热生效。"
      />
      <Callout msg={msg} err={err} />
      <div className="flex items-center justify-between mb-3">
        <p className="text-[12.5px] text-ghost m-0">
          订阅持久化于 SQLite;主题与推送目标在 <Link className="text-dim underline decoration-line hover:text-accent" to="/config">配置管理</Link> 中维护。
        </p>
        <button className="btn btn-sm shrink-0" onClick={() => setParams({ edit: 'new' })}>新建订阅</button>
      </div>
      {items.length > 0 ? (
        <div className="card overflow-x-auto"><table className="tbl">
          <thead><tr><th>主题</th><th>cron</th><th>推送目标</th><th>下次触发</th><th className="text-right">操作</th></tr></thead>
          <tbody>
            {items.map((s) => (
              <tr key={s.topic}>
                <td>
                  <Link className="font-medium text-accent no-underline hover:underline" to={`/subscriptions/${encodeURIComponent(s.topic)}`}>
                    {s.topic}
                  </Link>
                </td>
                <td><span className="mono">{s.cron}</span></td>
                <td className="text-dim">{s.targets.join(', ')}</td>
                <td className="text-dim whitespace-nowrap">{nextAt(s.topic)}</td>
                <td className="whitespace-nowrap text-right">
                  <button className="btn btn-ghost btn-sm" onClick={() => runNow(s.topic)}>
                    <IconPlay width={12} height={12} /> 运行
                  </button>{' '}
                  <button className="btn btn-ghost btn-sm" onClick={() => setParams({ edit: s.topic })}>编辑</button>{' '}
                  <button className="btn btn-danger btn-sm" onClick={() => remove(s.topic)}>删除</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table></div>
      ) : <Empty>还没有订阅;新建一个,把主题、cron 和推送目标组合成定时推送</Empty>}
    </div>
  )
}

function SubscriptionForm({ editing, onDone }: { editing: string; onDone: () => void }) {
  const isNew = editing === 'new'
  const [s, setS] = useState<Subscription>({ topic: '', cron: '0 8 * * *', targets: [] })
  const [topics, setTopics] = useState<string[]>([])
  const [allTargets, setAllTargets] = useState<string[]>([])
  const [err, setErr] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    get<Config>('/api/config')
      .then((c) => {
        setAllTargets(c.targets.map((x) => x.name))
        if (isNew) {
          // 新建时只能选择尚未被订阅的主题(主题即订阅身份)
          const subscribed = new Set(c.subscriptions.map((x) => x.topic))
          const avail = c.topics.map((x) => x.name).filter((n) => !subscribed.has(n))
          setTopics(avail)
          setS((p) => ({ ...p, topic: avail[0] ?? '' }))
        } else {
          setTopics(c.topics.map((x) => x.name))
          const found = c.subscriptions.find((x) => x.topic === editing)
          if (found) setS(found)
          else setErr(`订阅 "${editing}" 不存在`)
        }
      })
      .catch((e) => setErr(errMsg(e)))
  }, [editing, isNew])

  const save = async () => {
    setSaving(true)
    setErr('')
    try {
      await (isNew
        ? post<MutationResult>('/api/subscriptions', s)
        : put<MutationResult>(`/api/subscriptions/${encodeURIComponent(editing)}`, s))
      onDone()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const toggleTarget = (name: string) => {
    setS((p) => ({
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
        <label className="field-label">主题(订阅的唯一标识;保存后不可改)</label>
        {isNew ? (
          <>
            <select className="input" value={s.topic} onChange={(e) => setS({ ...s, topic: e.target.value })}>
              {topics.length === 0 && <option value="">(没有可订阅的主题:全部已订阅或尚未创建)</option>}
              {topics.map((n) => <option key={n} value={n}>{n}</option>)}
            </select>
            <p className="mt-1 text-[12px] text-ghost">一个主题最多一条订阅;如需新主题,请先到"配置管理 → 主题"创建。</p>
          </>
        ) : (
          <>
            <span className="mono">{editing}</span>
            <p className="mt-1 text-[12px] text-ghost">主题即订阅身份,不可改;如需换主题请新建订阅。</p>
          </>
        )}

        <label className="field-label">调度 cron(单个五段表达式,本地时区)</label>
        <input
          className="input font-mono text-xs"
          value={s.cron}
          onChange={(e) => setS({ ...s, cron: e.target.value })}
          placeholder="0 8 * * *"
        />
        <p className="mt-1 text-[12px] text-ghost">示例:每天 8 点 <code className="mono">0 8 * * *</code>;工作日 20:30 <code className="mono">30 20 * * 1-5</code></p>

        <label className="field-label">推送目标(可多选)</label>
        {allTargets.length === 0 && <p className="text-[12px] text-ghost m-0">暂无推送目标,请先到"配置管理 → 推送目标"新建</p>}
        {allTargets.map((n) => (
          <label key={n} className="mt-1.5 flex items-center gap-2 text-[13px]">
            <input type="checkbox" className="check" checked={s.targets.includes(n)} onChange={() => toggleTarget(n)} />
            {n}
          </label>
        ))}

        <div className="mt-5 flex flex-wrap items-center gap-3">
          <button className="btn" disabled={saving} type="submit">保存</button>
          <button className="btn btn-ghost" type="button" onClick={onDone}>取消</button>
          <span className="text-[12px] text-ghost">保存即校验引用与 cron 并热生效</span>
        </div>
      </form>
    </div>
  )
}
