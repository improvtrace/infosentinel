import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { get } from '../api'
import { ftime, dur } from '../fmt'
import { Empty, Section, StatusBadge } from '../components/ui'
import type { ListResp, RunRecord } from '../types'

export default function Runs() {
  const [items, setItems] = useState<RunRecord[]>([])
  useEffect(() => {
    get<ListResp<RunRecord>>('/api/runs?limit=100').then((r) => setItems(r.items)).catch(() => null)
  }, [])

  return (
    <div>
      <h1 className="text-[19px] font-semibold my-3">运行历史</h1>
      <p className="text-xs text-slate-500">进程内存中最近 100 条;持久化产物见邮件页。</p>
      <Section title={`共 ${items.length} 条`}>
        {items.length > 0 ? (
          <div className="card overflow-x-auto"><table className="tbl">
            <thead><tr><th>#</th><th>任务</th><th>触发</th><th>状态</th><th>耗时</th><th>开始</th><th>报告 / 错误</th></tr></thead>
            <tbody>
              {items.map((r) => (
                <tr key={r.id}>
                  <td className="text-slate-500">{r.id}</td>
                  <td><Link className="text-blue-600" to={`/tasks/${r.task}`}>{r.task}</Link></td>
                  <td className="text-slate-500">{r.trigger}</td>
                  <td><StatusBadge status={r.status} /></td>
                  <td>{r.finished_at ? dur(r.duration) : '—'}</td>
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
        ) : <Empty>暂无运行记录</Empty>}
      </Section>
    </div>
  )
}
