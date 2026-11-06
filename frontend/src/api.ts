// REST 客户端:统一错误处理(后端错误返回 {"error": "..."} 状态码 >= 400)。

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    throw new ApiError(data?.error ?? `HTTP ${res.status}`, res.status)
  }
  return data as T
}

export function get<T>(path: string): Promise<T> {
  return request<T>(path)
}

export function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

export function put<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function del<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' })
}

/** 展示变更结果:applied=false 时提示 warning。 */
export function mutationMessage(r: {
  ok?: boolean
  applied?: boolean
  warning?: string
}): { msg?: string; err?: string } {
  if (r.warning) return { err: r.warning }
  return { msg: '已保存并生效' }
}

/** 从 catch 的 unknown 错误中取出可展示信息。 */
export function errMsg(e: unknown): string {
  if (e instanceof Error) return e.message
  return String(e)
}
