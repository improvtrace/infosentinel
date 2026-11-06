import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { errMsg, get, post } from '../api'
import { dur, ftime } from '../fmt'
import { IconPlay, IconRefresh, IconSend } from '../components/icons'
import { Empty, Section, Stat, StatusBadge, TableWrap } from '../components/ui'
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
      await post<MutationResult>(`/api/tasks/${encodeURIComponent(name)}/run`)
      toast.ok(`已触发任务 ${name},后台执行中`)
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

  if (!ov) return <div className="empty mt-8">加载中…</div>

  return (
    <div>
      <div className="flex items-end gap-3 mb-5">
        <div>
          <h1 className="page-title">概览</h1>
          <p className="text-[12.5px] text-slate-400 dark:text-slate-500 mt-1">
            发送器 {ov.mailer_type} · 历史仅存进程内存 · 持久化产物见邮件产物页
          </p>
        </div>
        <button className="btn btn-ghost btn-sm ml-auto" onClick={load}>
          <IconRefresh width={14} height={14} /> 刷新
        </button>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <Stat value={ov.tasks} label="任务总数" tone="brand" />
        <Stat value={ov.running} label="运行中" tone="warn" />
        <Stat value={ov.ok} label="成功(内存历史)" tone="ok" />
        <Stat value={ov.fail} label="失败(内存历史)" tone="bad" />
      </div>

      <Section title="调度计划">
        {ov.nexts.length > 0 ? (
          <TableWrap>
            <table className="tbl">
              <thead>
                <tr>
                  <th>任务</th>
                  <th>cron</th>
                  <th>下次触发</th>
                  <th className="text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {ov.nexts.map((n, i) => (
                  <tr key={i}>
                    <td>
                      <Link className="text-[#4d6bfe] font-medium no-underline hover:underline" to={`/tasks/${n.task}`}>
                        {n.task}
                      </Link>
                    </td>
                    <td><span className="mono">{n.schedule}</span></td>
                    <td className="text-slate-500">{ftime(n.next)}</td>
                    <td className="text-right">
                      <button className="btn btn-ghost btn-sm" onClick={() => runTask(n.task)}>
                        <IconPlay width={12} height={12} /> 运行
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableWrap>
        ) : (
          <Empty>还没有调度规则;到配置管理创建任务</Empty>
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
                    <td><span className="mono">{t.type}</span></td>
                    <td className="text-slate-500">{t.to}</td>
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
                  <th>任务</th>
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
                    <td className="text-slate-400">{r.id}</td>
                    <td>
                      <Link className="text-[#4d6bfe] no-underline hover:underline" to={`/tasks/${r.task}`}>
                        {r.task}
                      </Link>
                    </td>
                    <td className="text-slate-400 text-[12px]">{r.trigger}</td>
                    <td><StatusBadge status={r.status} /></td>
                    <td className="text-slate-500">{r.finished_at ? dur(r.duration) : '—'}</td>
                    <td className="text-slate-400 text-[12px]">{ftime(r.started_at)}</td>
                    <td>
                      {r.subject ? (
                        <Link className="text-[#4d6bfe] no-underline hover:underline" to={`/runs/${r.id}`}>{r.subject}</Link>
                      ) : r.error ? (
                        <Link className="text-slate-400 no-underline hover:underline text-[12px]" to={`/runs/${r.id}`}>
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
          <Empty>暂无运行记录;点击上方任务行的“运行”手动触发一次</Empty>
        )}
      </Section>
    </div>
  )
}
