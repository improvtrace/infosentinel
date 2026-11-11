import { useEffect, useState } from 'react'
import { del, errMsg, get, post, mutationMessage, put } from '../../api'
import { IconKey, IconTrash } from '../../components/icons'
import { Section } from '../../components/ui'
import { useToast } from '../../components/Toast'
import type { MutationResult, Settings as SettingsT, StorageSpace, TestResult } from '../../types'

interface SettingsResp extends SettingsT {
  secrets?: string[]
}

const LLM_KEY = 'llm.api_key'
const SMTP_KEY = 'smtp.password'

// spacePasswordName 与后端 config.SpacePasswordName 一致:逐空间密码名。
const spacePasswordName = (space: string) => `storage.${space}.password`

// newSpace 返回一个空白产物空间(用于「添加空间」)。
const newSpace = (): StorageSpace => ({
  name: '',
  type: 'local',
  path: '',
  period_days: 1,
  max_dirs: 30,
  sftp: { host: '', port: 22, username: '', key_path: '', host_key: '', base_dir: '' },
  ftp: { host: '', port: 21, username: '', base_dir: '', tls: false },
})

export default function Settings() {
  const [s, setS] = useState<SettingsResp | null>(null)
  const [secrets, setSecrets] = useState<string[]>([])
  const [llmKey, setLlmKey] = useState('')
  const [smtpPass, setSmtpPass] = useState('')
  const [spacePasses, setSpacePasses] = useState<Record<string, string>>({})
  const [testTo, setTestTo] = useState('')
  const [saving, setSaving] = useState('')
  const [testing, setTesting] = useState('')
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

  // save 保存分区设置;表单中输入的密钥随请求体一并提交,由后端加密落库
  const save = async (section: 'llm' | 'mailer' | 'server' | 'storage') => {
    if (!s) return false
    setSaving(section)
    try {
      let body: Record<string, unknown> = { ...s[section] }
      let submittedPasswords: string[] = []
      if (section === 'llm' && llmKey.trim() !== '') {
        body.secret = llmKey
        submittedPasswords = [llmKey]
      }
      if (section === 'mailer' && smtpPass.trim() !== '') {
        body.secret = smtpPass
        submittedPasswords = [smtpPass]
      }
      if (section === 'storage') {
        // 逐空间密码:非空的随本次保存提交,空值不动已存密钥
        const passEntries = Object.entries(spacePasses).filter(([, v]) => v.trim() !== '')
        if (passEntries.length > 0) body.secrets = Object.fromEntries(passEntries)
        submittedPasswords = passEntries.map(([, v]) => v)
      }
      const r = await put<MutationResult>(`/api/settings/${section}`, body)
      const m = mutationMessage(r)
      if (m.err) {
        toast.err(m.err)
        return false
      }
      toast.ok(
        `${{ llm: '模型', mailer: '发送', server: '界面', storage: '产物空间' }[section]}设置已保存并生效${submittedPasswords.length > 0 ? ',密钥已加密保存' : ''}`,
      )
      if (submittedPasswords.length > 0) {
        if (section === 'llm') setLlmKey('')
        if (section === 'mailer') setSmtpPass('')
        if (section === 'storage') setSpacePasses({})
        load() // 刷新"已配置"徽标
      }
      return true
    } catch (e) {
      toast.err(errMsg(e))
      return false
    } finally {
      setSaving('')
    }
  }

  // test 用表单当前值做同步测试(不落库):表单密钥非空时优先使用,空则回退已保存密钥
  const test = async (kind: 'llm' | 'mailer') => {
    if (!s) return
    setTesting(kind)
    try {
      const body =
        kind === 'llm'
          ? { config: s.llm, secret: llmKey }
          : { config: s.mailer, secret: smtpPass, to: testTo.trim() }
      const r = await post<TestResult>(`/api/settings/${kind}/test`, body)
      if (r.ok) toast.ok(r.message)
      else toast.err(r.message)
    } catch (e) {
      toast.err(errMsg(e))
    } finally {
      setTesting('')
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

  const secretRow = (
    name: string,
    label: string,
    placeholder: string,
    value: string,
    setValue: (v: string) => void,
  ) => (
    <>
      <label className="field-label flex items-center gap-1.5">
        <IconKey width={13} height={13} /> {label}(加密存储)
        {secrets.includes(name) && <span className="badge badge-success ml-1">已配置</span>}
      </label>
      <div className="flex gap-2">
        <input
          className="input"
          type="password"
          placeholder={secrets.includes(name) ? '已保存;输入新值可覆盖' : placeholder}
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
        {secrets.includes(name) && (
          <button className="btn btn-danger shrink-0" onClick={() => removeSecret(name)}>
            <IconTrash width={13} height={13} />
          </button>
        )}
      </div>
      <p className="field-hint">输入后随下方保存按钮一并加密保存;留空则保持已保存值</p>
    </>
  )

  // ---- 产物空间列表编辑 ----
  const updateSpace = (i: number, patch: Partial<StorageSpace>) => {
    setS((prev) => {
      if (!prev) return prev
      const spaces = prev.storage.spaces.map((sp, j) => (j === i ? { ...sp, ...patch } : sp))
      return { ...prev, storage: { spaces } }
    })
  }

  const addSpace = () => {
    setS((prev) => {
      if (!prev) return prev
      return { ...prev, storage: { spaces: [...prev.storage.spaces, newSpace()] } }
    })
  }

  const removeSpace = (i: number) => {
    setS((prev) => {
      if (!prev) return prev
      const sp = prev.storage.spaces[i]
      if (!window.confirm(`删除产物空间 "${sp.name || '(未命名)'}"?已关联它的订阅保存时会校验失败`)) return prev
      return { ...prev, storage: { spaces: prev.storage.spaces.filter((_, j) => j !== i) } }
    })
  }

  const spaceCard = (sp: StorageSpace, i: number) => (
    <div key={i} className="card p-5">
      <div className="flex items-center justify-between mb-3">
        <h3 className="m-0 text-[13.5px] font-semibold">
          空间 {i + 1}
          {sp.name && secrets.includes(spacePasswordName(sp.name)) && (
            <span className="badge badge-success ml-2">密码已配置</span>
          )}
        </h3>
        <button className="btn btn-danger btn-sm" onClick={() => removeSpace(i)}>
          <IconTrash width={12} height={12} /> 删除
        </button>
      </div>
      <div className="grid md:grid-cols-2 gap-x-5">
        <div>
          <label className="field-label">空间名称(唯一,订阅按名引用)</label>
          <input className="input" placeholder="如 默认空间 / 备份空间" value={sp.name} onChange={(e) => updateSpace(i, { name: e.target.value })} />
        </div>
        <div>
          <label className="field-label">存储类型</label>
          <select className="input" value={sp.type} onChange={(e) => updateSpace(i, { type: e.target.value })}>
            <option value="local">local(本地磁盘)</option>
            <option value="sftp">sftp</option>
            <option value="ftp">ftp</option>
          </select>
        </div>
        <div>
          <label className="field-label">产物路径(相对根目录)</label>
          <input className="input" placeholder="如 products,留空为根目录" value={sp.path} onChange={(e) => updateSpace(i, { path: e.target.value })} />
          <p className="field-hint">本地磁盘实际路径 = 配置文件 storage.base_dir(绝对路径) + 本路径</p>
        </div>
        <div>
          <label className="field-label">分桶周期(天)</label>
          <input className="input" type="number" min={1} value={sp.period_days} onChange={(e) => updateSpace(i, { period_days: Number(e.target.value) })} />
          <p className="field-hint">每 N 天的产物保存到同一个以起始日期命名的子目录</p>
        </div>
        <div>
          <label className="field-label">最多保留子目录数</label>
          <input className="input" type="number" min={1} value={sp.max_dirs} onChange={(e) => updateSpace(i, { max_dirs: Number(e.target.value) })} />
          <p className="field-hint">超出后自动删除最旧的子目录</p>
        </div>
      </div>

      {sp.type === 'sftp' && (
        <div className="mt-4 grid md:grid-cols-2 gap-x-5 border-t border-line pt-4">
          <div>
            <label className="field-label">SFTP Host</label>
            <input className="input" value={sp.sftp.host} onChange={(e) => updateSpace(i, { sftp: { ...sp.sftp, host: e.target.value } })} />
          </div>
          <div>
            <label className="field-label">SFTP 端口(默认 22)</label>
            <input className="input" type="number" value={sp.sftp.port} onChange={(e) => updateSpace(i, { sftp: { ...sp.sftp, port: Number(e.target.value) } })} />
          </div>
          <div>
            <label className="field-label">用户名</label>
            <input className="input" value={sp.sftp.username} onChange={(e) => updateSpace(i, { sftp: { ...sp.sftp, username: e.target.value } })} />
          </div>
          <div>
            <label className="field-label">远端基础目录(空为登录目录)</label>
            <input className="input" value={sp.sftp.base_dir} onChange={(e) => updateSpace(i, { sftp: { ...sp.sftp, base_dir: e.target.value } })} />
          </div>
          <div>
            <label className="field-label">私钥路径(可选)</label>
            <input className="input" placeholder="服务器上的私钥文件路径" value={sp.sftp.key_path} onChange={(e) => updateSpace(i, { sftp: { ...sp.sftp, key_path: e.target.value } })} />
          </div>
          <div>
            <label className="field-label">Host Key(可选)</label>
            <input className="input" placeholder="authorized_keys 单行格式;留空不校验指纹" value={sp.sftp.host_key} onChange={(e) => updateSpace(i, { sftp: { ...sp.sftp, host_key: e.target.value } })} />
          </div>
        </div>
      )}

      {sp.type === 'ftp' && (
        <div className="mt-4 grid md:grid-cols-2 gap-x-5 border-t border-line pt-4">
          <div>
            <label className="field-label">FTP Host</label>
            <input className="input" value={sp.ftp.host} onChange={(e) => updateSpace(i, { ftp: { ...sp.ftp, host: e.target.value } })} />
          </div>
          <div>
            <label className="field-label">FTP 端口(默认 21)</label>
            <input className="input" type="number" value={sp.ftp.port} onChange={(e) => updateSpace(i, { ftp: { ...sp.ftp, port: Number(e.target.value) } })} />
          </div>
          <div>
            <label className="field-label">用户名(空按匿名)</label>
            <input className="input" value={sp.ftp.username} onChange={(e) => updateSpace(i, { ftp: { ...sp.ftp, username: e.target.value } })} />
          </div>
          <div>
            <label className="field-label">远端基础目录(空为登录目录)</label>
            <input className="input" value={sp.ftp.base_dir} onChange={(e) => updateSpace(i, { ftp: { ...sp.ftp, base_dir: e.target.value } })} />
          </div>
          <div>
            <label className="field-label">FTPS(显式 TLS)</label>
            <select className="input" value={String(sp.ftp.tls)} onChange={(e) => updateSpace(i, { ftp: { ...sp.ftp, tls: e.target.value === 'true' } })}>
              <option value="false">false</option>
              <option value="true">true</option>
            </select>
          </div>
        </div>
      )}

      {sp.type !== 'local' && sp.name && (
        <div className="mt-4">
          {secretRow(spacePasswordName(sp.name), `SFTP/FTP 登录密码(${sp.name})`, '登录密码', spacePasses[sp.name] ?? '', (v) =>
            setSpacePasses((prev) => ({ ...prev, [sp.name]: v })),
          )}
        </div>
      )}
    </div>
  )

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

          {secretRow(LLM_KEY, '模型 API Key', 'sk-...', llmKey, setLlmKey)}

          <div className="mt-5 flex gap-2">
            <button className="btn" disabled={saving === 'llm'} onClick={() => save('llm')}>保存模型设置</button>
            <button className="btn btn-ghost" disabled={testing === 'llm'} onClick={() => test('llm')}>
              {testing === 'llm' ? '测试中…' : '测试'}
            </button>
            <span className="text-[12px] text-slate-400 self-center">
              用当前表单值发送一次真实请求(不保存);API Key 留空时使用已保存密钥
            </span>
          </div>
        </div>
      </Section>

      <Section title="发送(站内信 / 邮件)">
        <div className="card p-5">
          <div className="grid md:grid-cols-2 gap-x-5">
            <div>
              <label className="field-label">投递类型</label>
              <select className="input" value={s.mailer.type} onChange={(e) => setS({ ...s, mailer: { ...s.mailer, type: e.target.value } })}>
                <option value="站内信">站内信(报告存入产物空间,在界面在线查看)</option>
                <option value="smtp">邮件(通过内置 SMTP 发件邮箱真实发送)</option>
              </select>
              <p className="field-hint">两种类型都面向 email 推送目标;站内信不发站外邮件</p>
            </div>
          </div>

          {s.mailer.type === 'smtp' && (
            <>
              <div className="mt-4 grid md:grid-cols-2 gap-x-5 border-t border-line pt-4">
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

              <div className="mt-4">
                {secretRow(SMTP_KEY, 'SMTP 授权码', '邮箱授权码', smtpPass, setSmtpPass)}
              </div>
            </>
          )}

          <div className="mt-5 flex gap-2 items-center flex-wrap">
            <button className="btn" disabled={saving === 'mailer'} onClick={() => save('mailer')}>保存发送设置</button>
            <button className="btn btn-ghost" disabled={testing === 'mailer'} onClick={() => test('mailer')}>
              {testing === 'mailer' ? '测试中…' : '测试'}
            </button>
            <input
              className="input max-w-56"
              placeholder="测试收件人(可选)"
              value={testTo}
              onChange={(e) => setTestTo(e.target.value)}
            />
            <span className="text-[12px] text-slate-400 self-center">
              用当前表单值同步测试:站内信写入测试产物,邮件真实发送;授权码留空时使用已保存密钥
            </span>
          </div>
        </div>
      </Section>

      <Section title="产物空间(可配置多个;订阅按名关联其一)">
        <div className="flex flex-col gap-4">
          {s.storage.spaces.map((sp, i) => spaceCard(sp, i))}
        </div>
        <div className="mt-4 flex items-center gap-3">
          <button className="btn btn-ghost" onClick={addSpace}>+ 添加产物空间</button>
          <button className="btn" disabled={saving === 'storage'} onClick={() => save('storage')}>保存产物空间设置</button>
          <span className="text-[12px] text-ghost">全部空间一起保存;订阅在订阅管理中按名称关联其中一个空间</span>
        </div>
      </Section>

      <Section title="Web 管理界面">
        <div className="card p-5">
          <label className="flex items-center gap-2 text-[13px]">
            <input
              type="checkbox"
              className="check"
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
