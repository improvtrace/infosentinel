import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { errMsg, get, post } from '../../api'
import { ftime } from '../../fmt'
import { IconRefresh } from '../../components/icons'
import { Callout, Section, Stat } from '../../components/ui'
import { useToast } from '../../components/Toast'
import type { Config, MutationResult, NextRun, Overview } from '../../types'

export default function Console() {
  const [cfg, setCfg] = useState<Config | null>(null)
  const [nexts, setNexts] = useState<NextRun[]>([])
  const [busy, setBusy] = useState(false)
  const toast = useToast()

  const load = () => {
    get<Config>('/api/config').then(setCfg).catch((e) => toast.err(errMsg(e)))
    get<Overview>('/api/overview')
      .then((ov) => setNexts(ov.nexts))
      .catch(() => null)
  }
  useEffect(load, []) // eslint-disable-line react-hooks/exhaustive-deps

  const apply = async () => {
    setBusy(true)
    try {
      await post<MutationResult>('/api/apply')
      toast.ok('配置已从数据库重载并生效')
      load()
    } catch (e) {
      toast.err(errMsg(e))
    } finally {
      setBusy(false)
    }
  }

  if (!cfg) return <div className="empty mt-6">加载中…</div>

  return (
    <div>
      <div className="grid grid-cols-3 gap-3">
        <Stat value={cfg.topics.length} label={<Link className="no-underline hover:underline" to="topics">主题</Link>} tone="brand" />
        <Stat value={cfg.targets.length} label={<Link className="no-underline hover:underline" to="targets">推送目标</Link>} tone="brand" />
        <Stat value={cfg.tasks.length} label={<Link className="no-underline hover:underline" to="tasks">任务</Link>} tone="brand" />
      </div>

      <Section title="当前生效摘要">
        <div className="card overflow-x-auto">
          <table className="tbl">
            <tbody>
              <tr>
                <td className="w-28 text-slate-400 text-[12px]">模型</td>
                <td>
                  <span className="mono">{cfg.llm.model}</span> @ {cfg.llm.base_url}
                  <span className="text-slate-400 text-[12px]"> · Key 走环境变量 </span>
                  <span className="mono">{cfg.llm.api_key_env}</span>
                  <span className="text-slate-400 text-[12px]"> 或加密存储</span>
                </td>
              </tr>
              <tr>
                <td className="text-slate-400 text-[12px]">发送器</td>
                <td>
                  <span className="mono">{cfg.mailer.type}</span>
                  {cfg.mailer.type === 'smtp' && (
                    <span className="text-slate-500"> → {cfg.mailer.smtp.host}:{cfg.mailer.smtp.port}</span>
                  )}
                  {cfg.mailer.type === 'dryrun' && (
                    <span className="text-slate-400 text-[12px]"> → {cfg.mailer.output_dir}</span>
                  )}
                </td>
              </tr>
              <tr>
                <td className="text-slate-400 text-[12px]">管理界面</td>
                <td>{cfg.server.enabled === false ? '已关闭' : '已启用'} · <span className="mono">{cfg.server.addr}</span></td>
              </tr>
              {nexts.map((n, i) => (
                <tr key={i}>
                  <td className="text-slate-400 text-[12px]">调度</td>
                  <td>
                    <span className="mono">{n.task}</span>
                    <span className="text-slate-400"> · </span>
                    <span className="mono">{n.schedule}</span>
                    <span className="text-slate-500"> → 下次 {ftime(n.next)}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      <div className="flex items-center gap-3 mt-5 flex-wrap">
        <button className="btn btn-ghost" disabled={busy} onClick={apply}>
          <IconRefresh width={14} height={14} /> 从数据库重载配置
        </button>
        <span className="text-[12px] text-slate-400">
          保存操作已自动生效;此按钮用于手动重载(等待运行中的任务结束)
        </span>
      </div>

      <Section title="配置解读">
        <div className="grid md:grid-cols-2 gap-3">
          <div className="card p-4">
            <div className="text-[13px] font-medium mb-1">数据从哪里来</div>
            <p className="text-[12.5px] text-slate-500 dark:text-slate-400 m-0">
              全部业务配置存于 SQLite 数据库(唯一事实源);config.yaml 只保留数据库与日志路径。
              首次启动自动写入默认种子,之后全部在此页面维护。
            </p>
          </div>
          <div className="card p-4">
            <div className="text-[13px] font-medium mb-1">敏感数据</div>
            <p className="text-[12.5px] text-slate-500 dark:text-slate-400 m-0">
              模型 API Key 与 SMTP 授权码支持加密入库(AES-256-GCM),也可继续用环境变量;
              <Link to="settings" className="text-[#4d6bfe] no-underline hover:underline"> 前往全局设置 </Link>
              填写。
            </p>
          </div>
        </div>
      </Section>

      <Callout />
    </div>
  )
}
