import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { marked } from 'marked'
import { get, errMsg } from '../api'
import { ftime, dur, triggerLabel } from '../fmt'
import { Crumb, Callout, PageHead, StatusBadge } from '../components/ui'
import type { RunRecord } from '../types'

// reportDoc 将 Markdown 报告渲染为独立 HTML 文档(与产物空间同样的文档风格)。
function reportDoc(md: string): string {
  const body = marked.parse(md, { async: false })
  return `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
body{margin:0;padding:24px 28px;font-family:'IBM Plex Sans',-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;font-size:14px;line-height:1.8;color:#17202F;}
h1,h2,h3{margin:22px 0 10px;line-height:1.4;font-weight:600;}
h2{font-size:16px;padding-bottom:8px;border-bottom:1px solid #E3E8F0;}
p{margin:10px 0}a{color:#2456D6;text-decoration:none;word-break:break-all}
ul,ol{padding-left:22px;margin:10px 0}li{margin:4px 0}
blockquote{margin:12px 0;padding:8px 14px;border-left:2px solid #C9D3E4;background:#F4F6FA;color:#55617A}
code{font-family:'IBM Plex Mono',ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12.5px;background:#F1F4F9;border-radius:5px;padding:1px 5px}
table{border-collapse:collapse;width:100%;margin:14px 0;font-size:13px}
th,td{border:1px solid #E3E8F0;padding:6px 10px;text-align:left}th{background:#F4F6FA;font-weight:500}
</style></head><body>${body}</body></html>`
}

export default function RunDetail() {
  const { id } = useParams()
  const [r, setR] = useState<RunRecord | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    get<RunRecord>(`/api/runs/${id}`).then(setR).catch((e) => setErr(errMsg(e)))
  }, [id])

  // 报告正文:Markdown → HTML,沙箱 iframe 渲染(禁脚本)
  const doc = useMemo(() => (r?.body ? reportDoc(r.body) : ''), [r?.body])

  if (err) return <Callout err={err} />
  if (!r) return null
  return (
    <div>
      <Crumb items={[{ to: '/', label: '概览' }, { to: `/subscriptions/${encodeURIComponent(r.topic)}`, label: r.topic }, { label: `运行 #${r.id}` }]} />
      <PageHead
        title={
          <span className="flex items-center gap-3">
            运行 <span className="font-mono text-dim">#{r.id}</span> <StatusBadge status={r.status} />
          </span>
        }
        desc={
          <>
            订阅 <Link className="text-accent no-underline hover:underline" to={`/subscriptions/${encodeURIComponent(r.topic)}`}>{r.topic}</Link>
            ，触发方式 {triggerLabel(r.trigger)}，开始于 {ftime(r.started_at)}
            {r.finished_at && (
              <>
                ，结束于 {ftime(r.finished_at)}，耗时 {dur(r.duration)}
              </>
            )}
          </>
        }
        actions={
          <Link className="btn btn-ghost btn-sm no-underline" to="/artifacts">
            查看产物空间
          </Link>
        }
      />
      {r.error && <div className="callout callout-error mb-4">{r.error}</div>}
      {r.subject && <h2 className="mt-5 text-[15px] font-semibold">{r.subject}</h2>}
      {doc && (
        <iframe
          title={`运行 #${r.id} 报告`}
          sandbox=""
          srcDoc={doc}
          className="mt-3 w-full rounded-xl border border-line bg-surface"
          style={{ height: '64vh' }}
        />
      )}
    </div>
  )
}
