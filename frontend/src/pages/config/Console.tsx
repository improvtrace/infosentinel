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

  if (!cfg) return <div className="empty mt-6">正在加载配置…</div>

  return (
    <div>
      <div className="grid grid-cols-3 gap-3">
        <Stat value={cfg.topics.length} label={<Link className="text-dim no-underline hover:underline" to="topics">主题</Link>} tone="accent" />
        <Stat value={cfg.targets.length} label={<Link className="text-dim no-underline hover:underline" to="targets">推送目标</Link>} tone="accent" />
        <Stat value={cfg.subscriptions.length} label={<Link className="text-dim no-underline hover:underline" to="/subscriptions">订阅</Link>} tone="accent" />
      </div>

      <Section title="当前生效摘要">
        <div className="card overflow-x-auto">
          <table className="tbl">
            <tbody>
              <tr>
                <td className="w-28 text-[12px] text-ghost">模型</td>
                <td>
                  <span className="mono">{cfg.llm.model}</span> <span className="text-ghost">@ {cfg.llm.base_url}</span>
                  <span className="ml-3 text-[12px] text-ghost">API Key 加密存储于全局设置</span>
                </td>
              </tr>
              <tr>
                <td className="text-[12px] text-ghost">发送</td>
                <td>
                  <span className="mono">{cfg.mailer.type}</span>
                  {cfg.mailer.type === 'smtp' && (
                    <span className="ml-2 text-dim">→ {cfg.mailer.smtp.host}:{cfg.mailer.smtp.port}</span>
                  )}
                  {cfg.mailer.type === '站内信' && (
                    <span className="ml-2 text-[12px] text-ghost">报告存入产物空间({cfg.storage.type}),在界面在线查看</span>
                  )}
                </td>
              </tr>
              <tr>
                <td className="text-[12px] text-ghost">管理界面</td>
                <td>
                  {cfg.server.enabled === false ? '已关闭' : '已启用'} <span className="ml-2 mono">{cfg.server.addr}</span>
                </td>
              </tr>
              {nexts.map((n, i) => (
                <tr key={i}>
                  <td className="text-[12px] text-ghost">调度</td>
                  <td>
                    <span className="mono">{n.topic}</span> <span className="mono">{n.cron}</span>
                    <span className="ml-3 text-[12px] text-dim">下次 {ftime(n.next)}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      <div className="mt-6 flex flex-wrap items-center gap-3">
        <button className="btn btn-ghost" disabled={busy} onClick={apply}>
          <IconRefresh width={14} height={14} /> 从数据库重载配置
        </button>
        <span className="text-[12px] text-ghost">
          保存操作已自动生效;此按钮用于手动重载(等待运行中的订阅结束)
        </span>
      </div>

      <Section title="配置解读">
        <div className="grid gap-3 md:grid-cols-2">
          <div className="card p-4">
            <div className="mb-1 text-[13px] font-medium">数据从哪里来</div>
            <p className="m-0 text-[12.5px] leading-relaxed text-ghost">
              全部业务配置存于 SQLite 数据库(唯一事实源);config.yaml 只保留数据库与日志路径。
              首次启动自动写入默认种子,之后全部在此页面维护。
            </p>
          </div>
          <div className="card p-4">
            <div className="mb-1 text-[13px] font-medium">敏感数据</div>
            <p className="m-0 text-[12.5px] leading-relaxed text-ghost">
              模型 API Key 与 SMTP 授权码在界面填写后加密入库(AES-256-GCM),数据库密文是唯一来源;
              <Link to="settings" className="text-accent no-underline hover:underline"> 前往全局设置 </Link>
              填写。
            </p>
          </div>
        </div>
      </Section>

      <Callout />
    </div>
  )
}
