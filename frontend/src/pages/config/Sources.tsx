import { useEffect, useState } from 'react'
import { get, mutationMessage, put, errMsg } from '../../api'
import { Callout } from '../../components/ui'
import type { MutationResult } from '../../types'

export default function Sources() {
  const [text, setText] = useState('')
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    get<{ yaml: string }>('/api/sources').then((r) => setText(r.yaml)).catch((e) => setErr(errMsg(e)))
  }, [])

  const save = async () => {
    setSaving(true)
    setErr('')
    setMsg('')
    try {
      const r = await put<MutationResult>('/api/sources', { yaml: text })
      const m = mutationMessage(r)
      setMsg(m.msg ?? '')
      setErr(m.err ?? '')
    } catch (e) {
      setErr(errMsg(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      <Callout msg={msg} err={err} />
      <p className="text-xs text-slate-500">
        公用数据源池(YAML 编辑):rss / search / hackernews / github_trending / webpages,以及采集通用参数 collect。
        所有主题共享;保存时整份配置校验。
      </p>
      <form
        className="card p-5"
        onSubmit={(e) => {
          e.preventDefault()
          save()
        }}
      >
        <textarea
          className="input font-mono text-xs min-h-[320px] leading-relaxed"
          value={text}
          onChange={(e) => setText(e.target.value)}
          spellCheck={false}
        />
        <div className="flex items-center gap-3 mt-4 flex-wrap">
          <button className="btn" disabled={saving} type="submit">保存</button>
          <span className="text-xs text-slate-500">保存即校验并热生效;字段说明见 .docs/configuration.md</span>
        </div>
      </form>
    </div>
  )
}
