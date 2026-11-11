import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { get } from '../api'
import { ftime, human } from '../fmt'
import { Empty, PageHead, Section, TableWrap } from '../components/ui'
import type { ArtifactFile, ListResp } from '../types'

export default function Artifacts() {
  const [items, setItems] = useState<ArtifactFile[]>([])
  const [params] = useSearchParams()
  const subscription = params.get('subscription') ?? ''

  useEffect(() => {
    const q = subscription ? `?subscription=${encodeURIComponent(subscription)}` : ''
    get<ListResp<ArtifactFile>>(`/api/artifacts${q}`).then((r) => setItems(r.items)).catch(() => null)
  }, [subscription])

  return (
    <div>
      <PageHead
        title="产物空间"
        desc="站内信报告与运行产物(HTML 文档),按分桶周期存放在各产物空间(本地磁盘 / SFTP / FTP)中。"
      />
      <Section title={`共 ${items.length} 个${subscription ? `(订阅 ${subscription})` : ''}`}>
        {items.length > 0 ? (
          <TableWrap>
            <table className="tbl">
              <thead>
                <tr>
                  <th>文件</th>
                  <th>空间</th>
                  <th>订阅</th>
                  <th>分桶</th>
                  <th>大小</th>
                  <th>生成时间</th>
                </tr>
              </thead>
              <tbody>
                {items.map((f) => (
                  <tr key={`${f.space}/${f.bucket}/${f.name}`}>
                    <td>
                      <Link
                        className="text-accent no-underline hover:underline"
                        to={`/artifacts/${encodeURIComponent(f.space)}/${encodeURIComponent(f.bucket)}/${encodeURIComponent(f.name)}`}
                      >
                        {f.name}
                      </Link>
                    </td>
                    <td>
                      <span className="mono">{f.space}</span>
                    </td>
                    <td>
                      {f.subscription && (
                        <Link className="text-accent no-underline hover:underline" to={`/subscriptions/${encodeURIComponent(f.subscription)}`}>
                          {f.subscription}
                        </Link>
                      )}
                    </td>
                    <td>
                      <span className="mono">{f.bucket}</span>
                    </td>
                    <td className="font-mono text-[12px] text-dim">{human(f.size)}</td>
                    <td className="text-[12px] text-ghost">{ftime(f.mod_time)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableWrap>
        ) : (
          <Empty>暂无产物;订阅成功运行后站内信报告会出现在这里</Empty>
        )}
      </Section>
    </div>
  )
}
