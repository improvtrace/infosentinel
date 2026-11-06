import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { get } from '../api'
import { ftime, human } from '../fmt'
import { Empty, Section } from '../components/ui'
import type { EmailFile, ListResp } from '../types'

export default function Emails() {
  const [items, setItems] = useState<EmailFile[]>([])
  useEffect(() => {
    get<ListResp<EmailFile>>('/api/emails').then((r) => setItems(r.items)).catch(() => null)
  }, [])

  return (
    <div>
      <h1 className="text-[19px] font-semibold my-3">邮件产物</h1>
      <p className="text-xs text-slate-500">dry-run 模式的邮件落盘目录(mailer.output_dir)。</p>
      <Section title={`共 ${items.length} 个`}>
        {items.length > 0 ? (
          <div className="card overflow-x-auto"><table className="tbl">
            <thead><tr><th>文件</th><th>任务</th><th>大小</th><th>生成时间</th></tr></thead>
            <tbody>
              {items.map((f) => (
                <tr key={f.name}>
                  <td><Link className="text-blue-600" to={`/emails/${encodeURIComponent(f.name)}`}>{f.name}</Link></td>
                  <td><Link className="text-blue-600" to={`/tasks/${f.task}`}>{f.task}</Link></td>
                  <td>{human(f.size)}</td>
                  <td className="text-slate-500">{ftime(f.mod_time)}</td>
                </tr>
              ))}
            </tbody>
          </table></div>
        ) : <Empty>暂无邮件产物;任务成功推送后 dry-run 产物会出现在这里</Empty>}
      </Section>
    </div>
  )
}
