import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { del, get, mutationMessage, post, put, errMsg } from '../../api'
import { Empty, Callout } from '../../components/ui'
import type { ListResp, MutationResult, Target } from '../../types'

export default function Targets() {
  const [items, setItems] = useState<Target[]>([])
  const [params, setParams] = useSearchParams()
  const editing = params.get('edit')
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')

  const load = useCallback(() => {
    get<ListResp<Target>>('/api/targets').then((r) => setItems(r.items)).catch((e) => setErr(errMsg(e)))
  }, [])
  useEffect(load, [load])

  const remove = async (name: string) => {
    if (!window.confirm(`删除推送目标 "${name}"?`)) return
    try {
      const res = await del<MutationResult>(`/api/targets/${encodeURIComponent(name)}`)
      const m = mutationMessage(res)
      setMsg(m.msg ?? '')
      setErr(m.err ?? '')
      load()
    } catch (e) {
      setErr(errMsg(e))
    }
  }

  if (editing !== null) {
    return <TargetForm editing={editing} onDone={() => { setParams({}); load() }} />
  }

  return (
    <div>
      <Callout msg={msg} err={err} />
      <div className="flex items-center justify-between mb-3">
        <p className="text-[12.5px] text-ghost m-0">报告推送的通道;被订阅引用的目标不可删除。真实投递需在"全局设置"中启用 SMTP。</p>
        <button className="btn btn-sm shrink-0" onClick={() => setParams({ edit: 'new' })}>新建</button>
      </div>
      {items.length > 0 ? (
        <div className="card overflow-x-auto"><table className="tbl">
          <thead><tr><th>名称</th><th>通道</th><th>地址</th><th>被引用</th><th className="text-right">操作</th></tr></thead>
          <tbody>
            {items.map((t) => (
              <tr key={t.name}>
                <td className="font-medium">{t.name}</td>
                <td><span className="mono">{t.type}</span></td>
                <td className="text-dim">{t.to}</td>
                <td className="text-dim">{t.used_by ?? 0} 个订阅</td>
                <td className="whitespace-nowrap text-right">
                  <button className="btn btn-ghost btn-sm" onClick={() => setParams({ edit: t.name })}>编辑</button>{' '}
                  <button className="btn btn-danger btn-sm" onClick={() => remove(t.name)}>删除</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table></div>
      ) : <Empty>还没有推送目标;新建一个 email 目标,然后在订阅里引用它</Empty>}
    </div>
  )
}

function TargetForm({ editing, onDone }: { editing: string; onDone: () => void }) {
  const isNew = editing === 'new'
  const [t, setT] = useState<Target>({ name: '', type: 'email', to: '', subject_prefix: '[InfoSentinel]' })
  const [err, setErr] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!isNew) {
      get<{ items: Target[] }>('/api/targets')
        .then((r) => {
          const found = r.items.find((x) => x.name === editing)
          if (found) setT({ name: found.name, type: found.type, to: found.to, subject_prefix: found.subject_prefix })
          else setErr(`推送目标 "${editing}" 不存在`)
        })
        .catch((e) => setErr(errMsg(e)))
    }
  }, [editing, isNew])

  const save = async () => {
    setSaving(true)
    setErr('')
    try {
      const body = { name: t.name, type: t.type, to: t.to, subject_prefix: t.subject_prefix }
      await (isNew
        ? post<MutationResult>('/api/targets', body)
        : put<MutationResult>(`/api/targets/${encodeURIComponent(editing)}`, body))
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
        {isNew ? (
          <>
            <label className="field-label">名称(唯一标识,订阅用它引用;保存后不可改)</label>
            <input className="input" value={t.name} onChange={(e) => setT({ ...t, name: e.target.value })} placeholder="如 mailbox" />
          </>
        ) : (
          <>
            <label className="field-label">名称</label>
            <span className="mono">{editing}</span>
          </>
        )}
        <label className="field-label">通道类型</label>
        <select className="input" value={t.type} onChange={(e) => setT({ ...t, type: e.target.value })}>
          <option value="email">email</option>
        </select>
        <label className="field-label">收件邮箱</label>
        <input className="input" value={t.to} onChange={(e) => setT({ ...t, to: e.target.value })} placeholder="me@example.com" />
        <label className="field-label">标题前缀</label>
        <input className="input" value={t.subject_prefix} onChange={(e) => setT({ ...t, subject_prefix: e.target.value })} placeholder="[InfoSentinel]" />
        <div className="mt-5 flex flex-wrap items-center gap-3">
          <button className="btn" disabled={saving} type="submit">保存</button>
          <button className="btn btn-ghost" type="button" onClick={onDone}>取消</button>
          <span className="text-[12px] text-ghost">保存即校验并热生效;真实投递需在"全局设置"中启用 SMTP</span>
        </div>
      </form>
    </div>
  )
}
