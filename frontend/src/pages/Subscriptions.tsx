import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { del, get, post, put, mutationMessage, errMsg } from '../api'
import { ftime } from '../fmt'
import { IconPlay } from '../components/icons'
import { Empty, Callout, PageHead, MultiSelect } from '../components/ui'
import { useToast } from '../components/Toast'
import { DEFAULT_SYNTHESIS } from '../types'
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

  const runNow = async (name: string) => {
    try {
      await post<MutationResult>(`/api/subscriptions/${encodeURIComponent(name)}/run`)
      toast.ok(`已触发订阅 ${name},后台执行中`)
      setTimeout(load, 1200)
    } catch (e) {
      toast.err(errMsg(e))
    }
  }

  const remove = async (name: string) => {
    if (!window.confirm(`删除订阅 "${name}"?(主题与去重状态文件保留)`)) return
    try {
      const res = await del<MutationResult>(`/api/subscriptions/${encodeURIComponent(name)}`)
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

  const nextAt = (name: string) => {
    const n = nexts.find((x) => x.subscription === name)
    return n ? ftime(n.next) : '—'
  }

  return (
    <div>
      <PageHead
        title="订阅管理"
        desc="一个订阅 = 名称 + 多个主题(各自采集后合并为一份报告)+ 一个 cron 调度 + 多个推送目标;保存即校验并热生效。"
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
          <thead><tr><th>名称</th><th>主题</th><th>cron</th><th>推送目标</th><th>产物空间</th><th>下次触发</th><th className="text-right">操作</th></tr></thead>
          <tbody>
            {items.map((s) => (
              <tr key={s.name}>
                <td>
                  <Link className="font-medium text-accent no-underline hover:underline" to={`/subscriptions/${encodeURIComponent(s.name)}`}>
                    {s.name}
                  </Link>
                </td>
                <td className="text-dim">{s.topics.join('、')}</td>
                <td><span className="mono">{s.cron}</span></td>
                <td className="text-dim">{s.targets.join(', ')}</td>
                <td className="text-dim">{s.space || '—'}</td>
                <td className="text-dim whitespace-nowrap">{nextAt(s.name)}</td>
                <td className="whitespace-nowrap text-right">
                  <button className="btn btn-ghost btn-sm" onClick={() => runNow(s.name)}>
                    <IconPlay width={12} height={12} /> 运行
                  </button>{' '}
                  <button className="btn btn-ghost btn-sm" onClick={() => setParams({ edit: s.name })}>编辑</button>{' '}
                  <button className="btn btn-danger btn-sm" onClick={() => remove(s.name)}>删除</button>
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
  const [s, setS] = useState<Subscription>({
    name: '',
    topics: [],
    synthesis: DEFAULT_SYNTHESIS,
    cron: '0 8 * * *',
    targets: [],
    space: '',
  })
  const [allTopics, setAllTopics] = useState<string[]>([])
  const [allTargets, setAllTargets] = useState<string[]>([])
  const [allSpaces, setAllSpaces] = useState<string[]>([])
  const [err, setErr] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    get<Config>('/api/config')
      .then((c) => {
        setAllTopics(c.topics.map((x) => x.name))
        setAllTargets(c.targets.map((x) => x.name))
        setAllSpaces(c.storage.spaces.map((x) => x.name))
        if (isNew) {
          setS((p) => ({ ...p, space: c.storage.spaces[0]?.name ?? '' }))
        } else {
          const found = c.subscriptions.find((x) => x.name === editing)
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
        <label className="field-label">订阅名称(唯一标识;保存后不可改)</label>
        {isNew ? (
          <input
            className="input"
            value={s.name}
            onChange={(e) => setS({ ...s, name: e.target.value })}
            placeholder="如 ai-agent-日报"
          />
        ) : (
          <span className="mono">{editing}</span>
        )}
        <p className="mt-1 text-[12px] text-ghost">产物文件与运行记录都以订阅名称标识;如需改名请新建订阅。</p>

        <label className="field-label">关联主题(可多选;各主题独立采集,报告最终合并为一份)</label>
        <MultiSelect
          options={allTopics}
          value={s.topics}
          onChange={(topics) => setS({ ...s, topics })}
          placeholder="选择要关联的主题"
          emptyText="暂无主题,请先到「配置管理 → 主题」创建"
        />
        <p className="mt-1 text-[12px] text-ghost">同一主题可被多个订阅引用;如需新主题,请先到「配置管理 → 主题」创建。</p>

        <label className="field-label">合成提示词(多主题报告合并为一份的指令)</label>
        <textarea
          className="input font-mono text-xs"
          rows={4}
          value={s.synthesis}
          onChange={(e) => setS({ ...s, synthesis: e.target.value })}
        />
        <p className="mt-1 text-[12px] text-ghost">仅关联一个主题时不会触发合并;建议 200 字以内。</p>

        <label className="field-label">调度 cron(单个五段表达式,本地时区)</label>
        <input
          className="input font-mono text-xs"
          value={s.cron}
          onChange={(e) => setS({ ...s, cron: e.target.value })}
          placeholder="0 8 * * *"
        />
        <p className="mt-1 text-[12px] text-ghost">示例:每天 8 点 <code className="mono">0 8 * * *</code>;工作日 20:30 <code className="mono">30 20 * * 1-5</code></p>

        <label className="field-label">推送目标(可多选)</label>
        <MultiSelect
          options={allTargets}
          value={s.targets}
          onChange={(targets) => setS({ ...s, targets })}
          placeholder="选择推送目标"
          emptyText="暂无推送目标,请先到「配置管理 → 推送目标」新建"
        />

        <label className="field-label">产物空间(站内信报告写入此处)</label>
        <select className="input" value={s.space} onChange={(e) => setS({ ...s, space: e.target.value })}>
          <option value="">不写产物空间(仅真实邮件推送)</option>
          {allSpaces.map((n) => <option key={n} value={n}>{n}</option>)}
        </select>
        <p className="mt-1 text-[12px] text-ghost">空间在「配置管理 → 全局设置 → 产物空间」中维护;邮件(smtp)投递不受此项影响。</p>

        <div className="mt-5 flex flex-wrap items-center gap-3">
          <button className="btn" disabled={saving} type="submit">保存</button>
          <button className="btn btn-ghost" type="button" onClick={onDone}>取消</button>
          <span className="text-[12px] text-ghost">保存即校验引用与 cron 并热生效</span>
        </div>
      </form>
    </div>
  )
}
