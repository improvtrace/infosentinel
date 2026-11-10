import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { del, get, mutationMessage, post, put, errMsg } from '../../api'
import { Empty, Callout } from "../../components/ui"
import type { ListResp, MutationResult, Topic } from '../../types'

export default function Topics() {
  const [items, setItems] = useState<Topic[]>([])
  const [params, setParams] = useSearchParams()
  const editing = params.get('edit') // undefined=列表, "new"=新建, name=编辑
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')

  const load = useCallback(() => {
    get<ListResp<Topic>>('/api/topics').then((r) => setItems(r.items)).catch((e) => setErr(errMsg(e)))
  }, [])
  useEffect(load, [load])

  const remove = async (name: string) => {
    if (!window.confirm(`删除主题 "${name}"?`)) return
    try {
      const res = await del<MutationResult>(`/api/topics/${encodeURIComponent(name)}`)
      const m = mutationMessage(res)
      setMsg(m.msg ?? '')
      setErr(m.err ?? '')
      load()
    } catch (e) {
      setErr(errMsg(e))
    }
  }

  if (editing !== null) {
    return <TopicForm editing={editing} onDone={() => { setParams({}); load() }} />
  }

  return (
    <div>
      <Callout msg={msg} err={err} />
      <div className="flex items-center justify-between mb-3">
        <p className="text-[12.5px] text-ghost m-0">主题是一段 prompt,agent 按它采集并撰写报告;已被订阅的主题不可删除。</p>
        <button className="btn btn-sm shrink-0" onClick={() => setParams({ edit: 'new' })}>新建</button>
      </div>
      {items.length > 0 ? (
        <div className="card overflow-x-auto"><table className="tbl">
          <thead><tr><th>名称</th><th>被引用</th><th>prompt 摘要</th><th className="text-right">操作</th></tr></thead>
          <tbody>
            {items.map((t) => (
              <tr key={t.name}>
                <td className="font-medium">{t.name}</td>
                <td className="text-dim">{t.used_by ?? 0} 个订阅</td>
                <td className="text-[12.5px] text-ghost">{t.prompt.length > 50 ? t.prompt.slice(0, 50) + '…' : t.prompt}</td>
                <td className="whitespace-nowrap text-right">
                  <button className="btn btn-ghost btn-sm" onClick={() => setParams({ edit: t.name })}>编辑</button>{' '}
                  <button className="btn btn-danger btn-sm" onClick={() => remove(t.name)}>删除</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table></div>
      ) : <Empty>还没有主题;新建一个,然后在订阅管理里订阅它</Empty>}
    </div>
  )
}

function TopicForm({ editing, onDone }: { editing: string; onDone: () => void }) {
  const isNew = editing === 'new'
  const [name, setName] = useState('')
  const [prompt, setPrompt] = useState('')
  const [err, setErr] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!isNew) {
      get<{ items: Topic[] }>('/api/topics')
        .then((r) => {
          const t = r.items.find((x) => x.name === editing)
          if (t) setPrompt(t.prompt)
          else setErr(`主题 "${editing}" 不存在`)
        })
        .catch((e) => setErr(errMsg(e)))
    }
  }, [editing, isNew])

  const save = async () => {
    setSaving(true)
    setErr('')
    try {
      await (isNew
        ? post<MutationResult>('/api/topics', { name, prompt })
        : put<MutationResult>(`/api/topics/${encodeURIComponent(editing)}`, { prompt }))
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
        className="card max-w-3xl p-5"
        onSubmit={(e) => {
          e.preventDefault()
          save()
        }}
      >
        {isNew ? (
          <>
            <label className="field-label">名称(唯一标识,订阅用它引用;保存后不可改)</label>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="如 ai-agent" />
          </>
        ) : (
          <>
            <label className="field-label">名称</label>
            <span className="mono">{editing}</span>
          </>
        )}
        <label className="field-label">Prompt(关注什么、如何取舍、报告怎么写)</label>
        <textarea
          className="input min-h-[240px] font-mono text-xs leading-relaxed"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder={'关注 LLM 与 AI Agent 应用开发生态,重点:\n- 新框架、新模型的发布与重要版本更新\n报告用中文撰写,每条附一句话摘要与原文链接。'}
        />
        <div className="mt-5 flex flex-wrap items-center gap-3">
          <button className="btn" disabled={saving} type="submit">保存</button>
          <button className="btn btn-ghost" type="button" onClick={onDone}>取消</button>
          <span className="text-[12px] text-ghost">保存即校验并热生效;运行中的订阅不受影响</span>
        </div>
      </form>
      <p className="mt-4"><Link className="text-[13px] text-accent no-underline hover:underline" to="/config/topics">返回主题列表</Link></p>
    </div>
  )
}
