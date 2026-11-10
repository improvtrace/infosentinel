// Toast 通知:轻量全局提示(成功 / 错误),3.5 秒自动消失。
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { IconAlert, IconCheck } from './icons'

type Kind = 'ok' | 'err'
interface Toast {
  id: number
  kind: Kind
  text: string
}

interface ToastAPI {
  ok: (text: string) => void
  err: (text: string) => void
}

const Ctx = createContext<ToastAPI>({ ok: () => {}, err: () => {} })

export function useToast(): ToastAPI {
  return useContext(Ctx)
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([])

  const push = useCallback((kind: Kind, text: string) => {
    const id = Date.now() + Math.random()
    setItems((prev) => [...prev, { id, kind, text }])
    setTimeout(() => setItems((prev) => prev.filter((t) => t.id !== id)), 3500)
  }, [])

  const api = useMemo<ToastAPI>(
    () => ({ ok: (t) => push('ok', t), err: (t) => push('err', t) }),
    [push],
  )

  return (
    <Ctx.Provider value={api}>
      {children}
      <div className="fixed bottom-6 right-6 z-50 flex max-w-sm flex-col gap-2">
        {items.map((t) => (
          <div
            key={t.id}
            className={`toast-pop flex items-start gap-2.5 ${
              t.kind === 'ok' ? 'text-ok' : 'text-bad'
            }`}
            style={{ animation: 'toastIn .18s ease-out' }}
          >
            {t.kind === 'ok' ? <IconCheck className="mt-0.5 shrink-0" /> : <IconAlert className="mt-0.5 shrink-0" />}
            <span>{t.text}</span>
          </div>
        ))}
      </div>
      <style>{`@keyframes toastIn{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}`}</style>
    </Ctx.Provider>
  )
}
