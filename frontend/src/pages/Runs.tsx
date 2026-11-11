import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { get } from '../api'
import { ftime, dur, triggerLabel, parseDsn } from '../fmt'
import { Empty, PageHead, Section, StatusBadge, TableWrap } from '../components/ui'
import type { ListResp, RunRecord } from '../types'

// ArtifactLinks 把运行记录的产物 DSN 渲染为可跳转链接(截断展示,完整路径见 title)。
function ArtifactLinks({ dsnList }: { dsnList: string[] }) {
  if (!dsnList || dsnList.length === 0) return <span className="text-ghost">—</span>
  return (
    <span className="flex flex-col gap-0.5">
      {dsnList.map((dsn) => {
        const p = parseDsn(dsn)
        return (
          <Link
            key={dsn}
            className="font-mono text-[12px] text-accent no-underline hover:underline break-all"
            title={dsn}
            to={p ? `/artifacts/${encodeURIComponent(p.space)}/${encodeURIComponent(p.bucket)}/${encodeURIComponent(p.name)}` : '/artifacts'}
          >
            {dsn}
          </Link>
        )
      })}
    </span>
  )
}

export default function Runs() {
  const [items, setItems] = useState<RunRecord[]>([])
  useEffect(() => {
    get<ListResp<RunRecord>>('/api/runs?limit=100').then((r) => setItems(r.items)).catch(() => null)
  }, [])

  return (
    <div>
      <PageHead title="运行历史" desc="订阅的运行记录,持久化于 SQLite(最近 100 条);产物路径为 DSN(类型://空间名/分桶/文件名)。" />
      <Section title={`共 ${items.length} 条`}>
        {items.length > 0 ? (
          <TableWrap>
            <table className="tbl">
              <thead>
                <tr>
                  <th>#</th>
                  <th>订阅</th>
                  <th>触发</th>
                  <th>状态</th>
                  <th>耗时</th>
                  <th>开始</th>
                  <th>产物路径</th>
                  <th>报告 / 错误</th>
                </tr>
              </thead>
              <tbody>
                {items.map((r) => (
                  <tr key={r.id}>
                    <td className="font-mono text-[12px] text-ghost">{r.id}</td>
                    <td>
                      <Link className="text-accent no-underline hover:underline" to={`/subscriptions/${encodeURIComponent(r.subscription)}`}>
                        {r.subscription}
                      </Link>
                    </td>
                    <td className="text-[12px] text-ghost">{triggerLabel(r.trigger)}</td>
                    <td>
                      <StatusBadge status={r.status} />
                    </td>
                    <td className="font-mono text-[12px] text-dim">{r.finished_at ? dur(r.duration) : '—'}</td>
                    <td className="text-[12px] text-ghost">{ftime(r.started_at)}</td>
                    <td className="max-w-64">
                      <ArtifactLinks dsnList={r.artifacts} />
                    </td>
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
          <Empty>暂无运行记录</Empty>
        )}
      </Section>
    </div>
  )
}
