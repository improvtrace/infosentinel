import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { errMsg, get, post } from '../api'
import { dur, ftime, triggerLabel } from '../fmt'
import { IconPlay, IconRefresh, IconSend } from '../components/icons'
import { Empty, PageHead, Section, StatBand, StatusBadge, TableWrap } from '../components/ui'
import { useToast } from '../components/Toast'
import type { MutationResult, Overview } from '../types'

export default function Dashboard() {
  const [ov, setOv] = useState<Overview | null>(null)
  const toast = useToast()

  const load = useCallback(() => {
    get<Overview>('/api/overview').then(setOv).catch((e) => toast.err(errMsg(e)))
  }, [toast])

  useEffect(() => {
    load()
    const t = setInterval(load, ov?.running ? 8000 : 45000)
    return () => clearInterval(t)
  }, [load, ov?.running])

  const runTask = async (name: string) => {
    try {
      await post<MutationResult>(`/api/subscriptions/${encodeURIComponent(name)}/run`)
      toast.ok(`已触发订阅 ${name},后台执行中`)
      setTimeout(load, 1200)
    } catch (e) {
      toast.err(errMsg(e))
    }
  }

  const testTarget = async (name: string) => {
    try {
      await post<MutationResult>(`/api/targets/${encodeURIComponent(name)}/test`)
      toast.ok(`测试邮件已发送到 ${name}`)
    } catch (e) {
      toast.err(errMsg(e))
    }
  }

  if (!ov) return <div className="empty mt-8">正在连接服务,加载概览…</div>

  return (
    <div>
      <PageHead
        title="概览"
        desc={`发送通道 ${ov.mailer_type === 'smtp' ? '邮件(SMTP)' : '站内信'};历史保存在进程内存,持久化报告见产物空间`}
        actions={
          <button className="btn btn-ghost btn-sm" onClick={load}>
            <IconRefresh width={14} height={14} /> 刷新
          </button>
        }
      />

      <StatBand
        items={[
          { key: '订阅总数', value: ov.subscriptions, tone: 'accent' },
          { key: '运行中', value: ov.running, tone: 'warn' },
          { key: '成功', value: ov.ok, tone: 'ok' },
          { key: '失败', value: ov.fail, tone: 'bad' },
        ]}
      />

      <Section title="调度计划">
        {ov.nexts.length > 0 ? (
          <TableWrap>
            <table className="tbl">
              <thead>
                <tr>
                  <th>订阅</th>
                  <th>cron</th>
                  <th>下次触发</th>
                  <th className="text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {ov.nexts.map((n, i) => (
                  <tr key={i}>
                    <td>
                      <Link className="font-medium text-accent no-underline hover:underline" to={`/subscriptions/${encodeURIComponent(n.subscription)}`}>
                        {n.subscription}
                      </Link>
                    </td>
                    <td>
                      <span className="mono">{n.cron}</span>
                    </td>
                    <td className="text-dim">{ftime(n.next)}</td>
                    <td className="text-right">
                      <button className="btn btn-ghost btn-sm" onClick={() => runTask(n.subscription)}>
                        <IconPlay width={12} height={12} /> 运行
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableWrap>
        ) : (
          <Empty>还没有调度中的订阅;到订阅管理新建订阅</Empty>
        )}
      </Section>

      <Section title="推送目标">
        {ov.targets.length > 0 ? (
          <TableWrap>
            <table className="tbl">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>通道</th>
                  <th>收件地址</th>
                  <th className="text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {ov.targets.map((t) => (
                  <tr key={t.name}>
                    <td className="font-medium">{t.name}</td>
                    <td>
                      <span className="mono">{t.type}</span>
                    </td>
                    <td className="text-dim">{t.to}</td>
                    <td className="text-right">
                      <button className="btn btn-ghost btn-sm" onClick={() => testTarget(t.name)}>
                        <IconSend width={12} height={12} /> 发送测试邮件
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableWrap>
        ) : (
          <Empty>还没有推送目标</Empty>
        )}
      </Section>

      <Section title="最近运行">
        {ov.recent_runs.length > 0 ? (
          <TableWrap>
            <table className="tbl">
              <thead>
                <tr>
                  <th>#</th>
                  <th>订阅</th>
                  <th>触发</th>
                  <th>状态</th>
                  <th>耗时</th>
                  <th>开始时间</th>
                  <th>报告 / 错误</th>
                </tr>
              </thead>
              <tbody>
                {ov.recent_runs.map((r) => (
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
                    <td>
                      {r.subject ? (
                        <Link className="text-accent no-underline hover:underline" to={`/runs/${r.id}`}>
                          {r.subject}
                        </Link>
                      ) : r.error ? (
                        <Link
                          className="text-[12px] text-ghost no-underline hover:underline"
                          to={`/runs/${r.id}`}
                        >
                          {r.error.slice(0, 56)}…
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
          <Empty>暂无运行记录;点击上方订阅行的「运行」手动触发一次</Empty>
        )}
      </Section>
    </div>
  )
}
