// 与后端 REST API(/api/*)的 JSON 结构一一对应。

export interface Topic {
  name: string
  prompt: string
  used_by?: number
}

export interface Target {
  name: string
  type: string
  to: string
  subject_prefix: string
  used_by?: number
}

export interface Subscription {
  name: string
  topics: string[] // 关联的主题名,可多个(一个主题可被多个订阅引用)
  synthesis: string // 合成提示词:多主题报告合并指令
  cron: string
  targets: string[] // 推送目标名,可多个
  space: string // 产物空间名(单个;空 = 不写产物空间)
}

export interface NextRun {
  subscription: string
  cron: string
  next: string
}

export type RunStatus = 'running' | 'success' | 'failed'

export interface RunRecord {
  id: number
  subscription: string
  trigger: string
  status: RunStatus
  started_at: string
  finished_at: string
  duration: number
  error: string
  subject: string
  body: string
  artifacts: string[] // 产物路径(DSN 格式:类型://空间名/分桶/文件名)
}

export interface LLMConfig {
  base_url: string
  model: string
  temperature: number
  timeout_sec: number
  max_step: number
}

export interface SmtpConfig {
  host: string
  port: number
  username: string
  from: string
  ssl: boolean | null
}

export interface MailerConfig {
  type: string
  smtp: SmtpConfig
}

export interface StorageSFTP {
  host: string
  port: number
  username: string
  key_path: string
  host_key: string
  base_dir: string
}

export interface StorageFTP {
  host: string
  port: number
  username: string
  base_dir: string
  tls: boolean
}

export interface StorageSpace {
  name: string
  type: string // local | sftp | ftp
  path: string
  period_days: number
  max_dirs: number
  sftp: StorageSFTP
  ftp: StorageFTP
}

export interface StorageConfig {
  spaces: StorageSpace[]
}

export interface ServerConfig {
  enabled: boolean | null
  addr: string
}

export interface Settings {
  llm: LLMConfig
  mailer: MailerConfig
  server: ServerConfig
  storage: StorageConfig
}

export interface ArtifactFile {
  space: string
  bucket: string
  name: string
  size: number
  mod_time: string
  subscription?: string
  dsn?: string
}

export interface ArtifactContent extends ArtifactFile {
  content: string
}

export interface TestResult {
  ok: boolean
  message: string
}

export interface Overview {
  version: string
  uptime_sec: number
  subscriptions: number
  running: number
  ok: number
  fail: number
  nexts: NextRun[]
  recent_runs: RunRecord[]
  targets: Target[]
  mailer_type: string
}

export interface MutationResult {
  ok: boolean
  applied?: boolean
  warning?: string
}

export interface ListResp<T> {
  items: T[]
}

export interface Config {
  database: { path: string }
  topics: Topic[]
  targets: Target[]
  subscriptions: Subscription[]
  llm: LLMConfig
  mailer: MailerConfig
  server: ServerConfig
  storage: StorageConfig
}

// 合成提示词默认值(与后端 repository.DefaultSynthesis 保持一致,新建订阅时预填)
export const DEFAULT_SYNTHESIS =
  '你是资讯编辑。请把多份主题报告合并为一份最终报告:去除重复与冗余,按主题分节或按重要性排序,保留每条的原文链接与一句话摘要,开头用一段导语概括本期整体看点。第一行只写总标题纯文本;从第二行起用中文 HTML 片段撰写正文(<section>/<h2>/<p>/<ul>/<table> 等内容级标签,不含 <html>/<body>/<style>/<script>),链接用 <a> 标签原样保留。'
