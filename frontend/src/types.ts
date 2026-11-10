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
  topic: string
  cron: string
  targets: string[]
}

export interface NextRun {
  topic: string
  cron: string
  next: string
}

export type RunStatus = 'running' | 'success' | 'failed'

export interface RunRecord {
  id: number
  topic: string
  trigger: string
  status: RunStatus
  started_at: string
  finished_at: string
  duration: number
  error: string
  subject: string
  body: string
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

export interface StorageConfig {
  type: string // local | sftp | ftp
  path: string
  period_days: number
  max_dirs: number
  sftp: StorageSFTP
  ftp: StorageFTP
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
  bucket: string
  name: string
  size: number
  mod_time: string
  topic?: string
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
  sources: Sources
  topics: Topic[]
  targets: Target[]
  subscriptions: Subscription[]
  llm: LLMConfig
  mailer: MailerConfig
  server: ServerConfig
  storage: StorageConfig
}

export interface Sources {
  collect: {
    window_hours: number
    max_items_per_source: number
    http_timeout_sec: number
    max_body_chars: number
    dedup_days: number
  }
  rss: { enabled: boolean; feeds: { name: string; url: string }[] }
  search: { enabled: boolean; region: string; max_results: number }
  hackernews: { enabled: boolean; top_n: number; min_points: number }
  github_trending: { enabled: boolean; language: string; top_n: number }
  webpages: { name: string; url: string }[]
}
