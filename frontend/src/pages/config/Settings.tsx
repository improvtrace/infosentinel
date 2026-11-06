import { useEffect, useState } from 'react'
import { del, errMsg, get, mutationMessage, put } from '../../api'
import { IconKey, IconTrash } from '../../components/icons'
import { Section } from '../../components/ui'
import { useToast } from '../../components/Toast'
import type { MutationResult, Settings as SettingsT } from '../../types'

interface SettingsResp extends SettingsT {
  secrets?: string[]
}

const LLM_KEY = 'llm.api_key'
const SMTP_KEY = 'smtp.password'

export default function Settings() {
  const [s, setS] = useState<SettingsResp | null>(null)
  const [secrets, setSecrets] = useState<string[]>([])
  const [llmKey, setLlmKey] = useState('')
  const [smtpPass, setSmtpPass] = useState('')
  const [saving, setSaving] = useState('')
  const toast = useToast()

  const load = () => {
    get<SettingsResp>('/api/settings')
      .then((r) => {
        setS(r)
        setSecrets(r.secrets ?? [])
      })
      .catch((e) => toast.err(errMsg(e)))
  }
  useEffect(load, []) // eslint-disable-line react-hooks/exhaustive-deps

  const save = async (section: 'llm' | 'mailer' | 'server') => {
    if (!s) return
    setSaving(section)
    try {
      const body = section === 'llm' ? s.llm : section === 'mailer' ? s.mailer : s.server
      const r = await put<MutationResult>(`/api/settings/${section}`, body)
      const m = mutationMessage(r)
      if (m.err) toast.err(m.err)
      else toast.ok(`${section === 'llm' ? '模型' : section === 'mailer' ? '发送器' : '界面'}设置已保存并生效`)
    } catch (e) {
      toast.err(errMsg(e))
    } finally {
      setSaving('')
    }
  }

  const saveSecret = async (name: string, value: string) => {
    if (!value.trim()) {
      toast.err('请输入要保存的值')
      return
    }
    try {
      await put<MutationResult>(`/api/secrets/${encodeURIComponent(name)}`, { value })
      toast.ok('已加密保存(数据库中为密文)')
      if (name === LLM_KEY) setLlmKey('')
      if (name === SMTP_KEY) setSmtpPass('')
      load()
    } catch (e) {
      toast.err(errMsg(e))
    }
  }

  const removeSecret = async (name: string) => {
    if (!window.confirm(`删除已保存的密钥 ${name}?`)) return
    try {
      await del<MutationResult>(`/api/secrets/${encodeURIComponent(name)}`)
      toast.ok('已删除')
      load()
    } catch (e) {
      toast.err(errMsg(e))
    }
  }

  if (!s) return <div className="empty mt-6">加载中…</div>

  return (
    <div>
      <Section title="模型(OpenAI 兼容协议)">
        <div className="card p-5">
          <div className="grid md:grid-cols-2 gap-x-5">
            <div>
              <label className="field-label">Base URL</label>
              <input className="input" value={s.llm.base_url} onChange={(e) => setS({ ...s, llm: { ...s.llm, base_url: e.target.value } })} />
            </div>
            <div>
              <label className="field-label">模型</label>
              <input className="input" value={s.llm.model} onChange={(e) => setS({ ...s, llm: { ...s.llm, model: e.target.value } })} />
            </div>
            <div>
              <label className="field-label">API Key 环境变量名</label>
              <input className="input" value={s.llm.api_key_env} onChange={(e) => setS({ ...s, llm: { ...s.llm, api_key_env: e.target.value } })} />
              <p className="field-hint">优先级:环境变量 → 下方加密存储</p>
            </div>
            <div>
              <label className="field-label">温度</label>
              <input className="input" type="number" step="0.1" value={s.llm.temperature} onChange={(e) => setS({ ...s, llm: { ...s.llm, temperature: Number(e.target.value) } })} />
            </div>
            <div>
              <label className="field-label">请求超时(秒)</label>
              <input className="input" type="number" value={s.llm.timeout_sec} onChange={(e) => setS({ ...s, llm: { ...s.llm, timeout_sec: Number(e.target.value) } })} />
            </div>
            <div>
              <label className="field-label">最大循环步数</label>
              <input className="input" type="number" value={s.llm.max_step} onChange={(e) => setS({ ...s, llm: { ...s.llm, max_step: Number(e.target.value) } })} />
            </div>
          </div>

          <label className="field-label flex items-center gap-1.5">
            <IconKey width={13} height={13} /> 模型 API Key(加密存储)
            {secrets.includes(LLM_KEY) && <span className="badge badge-success ml-1">已配置</span>}
          </label>
          <div className="flex gap-2">
            <input
              className="input"
              type="password"
              placeholder={secrets.includes(LLM_KEY) ? '已保存;输入新值可覆盖' : 'sk-...'}
              value={llmKey}
              onChange={(e) => setLlmKey(e.target.value)}
            />
            <button className="btn shrink-0" onClick={() => saveSecret(LLM_KEY, llmKey)}>加密保存</button>
            {secrets.includes(LLM_KEY) && (
              <button className="btn btn-danger shrink-0" onClick={() => removeSecret(LLM_KEY)}>
                <IconTrash width={13} height={13} />
              </button>
            )}
          </div>

          <div className="mt-5">
            <button className="btn" disabled={saving === 'llm'} onClick={() => save('llm')}>保存模型设置</button>
          </div>
        </div>
      </Section>

      <Section title="发送器(内置 SMTP 发件邮箱)">
        <div className="card p-5">
          <div className="grid md:grid-cols-2 gap-x-5">
            <div>
              <label className="field-label">类型</label>
              <select className="input" value={s.mailer.type} onChange={(e) => setS({ ...s, mailer: { ...s.mailer, type: e.target.value } })}>
                <option value="dryrun">dryrun(落盘预览,不发送)</option>
                <option value="smtp">smtp(真实发送)</option>
              </select>
            </div>
            <div>
              <label className="field-label">dry-run 输出目录</label>
              <input className="input" value={s.mailer.output_dir} onChange={(e) => setS({ ...s, mailer: { ...s.mailer, output_dir: e.target.value } })} />
            </div>
            <div>
              <label className="field-label">SMTP Host</label>
              <input className="input" placeholder="smtp.qq.com" value={s.mailer.smtp.host} onChange={(e) => setS({ ...s, mailer: { ...s.mailer, smtp: { ...s.mailer.smtp, host: e.target.value } } })} />
            </div>
            <div>
              <label className="field-label">SMTP 端口(465 隐式 TLS / 587 STARTTLS)</label>
              <input className="input" type="number" value={s.mailer.smtp.port} onChange={(e) => setS({ ...s, mailer: { ...s.mailer, smtp: { ...s.mailer.smtp, port: Number(e.target.value) } } })} />
            </div>
            <div>
              <label className="field-label">SMTP 用户名</label>
              <input className="input" value={s.mailer.smtp.username} onChange={(e) => setS({ ...s, mailer: { ...s.mailer, smtp: { ...s.mailer.smtp, username: e.target.value } } })} />
            </div>
            <div>
              <label className="field-label">授权码环境变量名</label>
              <input className="input" value={s.mailer.smtp.password_env} onChange={(e) => setS({ ...s, mailer: { ...s.mailer, smtp: { ...s.mailer.smtp, password_env: e.target.value } } })} />
            </div>
            <div>
              <label className="field-label">发件人地址</label>
              <input className="input" value={s.mailer.smtp.from} onChange={(e) => setS({ ...s, mailer: { ...s.mailer, smtp: { ...s.mailer.smtp, from: e.target.value } } })} />
            </div>
            <div>
              <label className="field-label">隐式 TLS(ssl)</label>
              <select
                className="input"
                value={s.mailer.smtp.ssl === null || s.mailer.smtp.ssl === undefined ? 'auto' : String(s.mailer.smtp.ssl)}
                onChange={(e) => {
                  const v = e.target.value
                  setS({
                    ...s,
                    mailer: { ...s.mailer, smtp: { ...s.mailer.smtp, ssl: v === 'auto' ? null : v === 'true' } },
                  })
                }}
              >
                <option value="auto">自动(按端口判断)</option>
                <option value="true">true</option>
                <option value="false">false</option>
              </select>
            </div>
          </div>

          <label className="field-label flex items-center gap-1.5">
            <IconKey width={13} height={13} /> SMTP 授权码(加密存储)
            {secrets.includes(SMTP_KEY) && <span className="badge badge-success ml-1">已配置</span>}
          </label>
          <div className="flex gap-2">
            <input
              className="input"
              type="password"
              placeholder={secrets.includes(SMTP_KEY) ? '已保存;输入新值可覆盖' : '邮箱授权码'}
              value={smtpPass}
              onChange={(e) => setSmtpPass(e.target.value)}
            />
            <button className="btn shrink-0" onClick={() => saveSecret(SMTP_KEY, smtpPass)}>加密保存</button>
            {secrets.includes(SMTP_KEY) && (
              <button className="btn btn-danger shrink-0" onClick={() => removeSecret(SMTP_KEY)}>
                <IconTrash width={13} height={13} />
              </button>
            )}
          </div>

          <div className="mt-5">
            <button className="btn" disabled={saving === 'mailer'} onClick={() => save('mailer')}>保存发送器设置</button>
          </div>
        </div>
      </Section>

      <Section title="Web 管理界面">
        <div className="card p-5">
          <label className="flex items-center gap-2 text-[13px]">
            <input
              type="checkbox"
              className="accent-[#4d6bfe] w-4 h-4"
              checked={s.server.enabled !== false}
              onChange={(e) => setS({ ...s, server: { ...s.server, enabled: e.target.checked } })}
            />
            启用管理界面
          </label>
          <label className="field-label">监听地址</label>
          <input className="input max-w-sm" value={s.server.addr} onChange={(e) => setS({ ...s, server: { ...s.server, addr: e.target.value } })} />
          <p className="field-hint">默认仅监听本机;修改后需重启服务生效</p>
          <div className="mt-5">
            <button className="btn" disabled={saving === 'server'} onClick={() => save('server')}>保存界面设置</button>
          </div>
        </div>
      </Section>
    </div>
  )
}
