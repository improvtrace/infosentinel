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

export interface Task {
  name: string
  topic: string
  targets: string[]
  schedules: string[]
}

export interface NextRun {
  task: string
  schedule: string
  next: string
}

export type RunStatus = 'running' | 'success' | 'failed'

export interface RunRecord {
  id: number
  task: string
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
  api_key_env: string
  temperature: number
  timeout_sec: number
  max_step: number
}

export interface SmtpConfig {
  host: string
  port: number
  username: string
  password_env: string
  from: string
  ssl: boolean | null
}

export interface MailerConfig {
  type: string
  output_dir: string
  smtp: SmtpConfig
}

export interface ServerConfig {
  enabled: boolean | null
  addr: string
}

export interface Settings {
  llm: LLMConfig
  mailer: MailerConfig
  server: ServerConfig
}

export interface EmailFile {
  name: string
  task: string
  size: number
  mod_time: string
}

export interface EmailContent extends EmailFile {
  content: string
}

export interface Overview {
  version: string
  uptime_sec: number
  tasks: number
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
  tasks: Task[]
  llm: LLMConfig
  mailer: MailerConfig
  server: ServerConfig
}

export interface Sources {
  collect: {
    window_hours: number
    max_items_per_source: number
    http_timeout_sec: number
    max_body_chars: number
    dedup_days: number
    state_dir: string
  }
  rss: { enabled: boolean; feeds: { name: string; url: string }[] }
  search: { enabled: boolean; region: string; max_results: number }
  hackernews: { enabled: boolean; top_n: number; min_points: number }
  github_trending: { enabled: boolean; language: string; top_n: number }
  webpages: { name: string; url: string }[]
}
