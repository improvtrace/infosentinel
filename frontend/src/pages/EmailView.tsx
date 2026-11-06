import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { get, errMsg } from '../api'
import { ftime, human } from '../fmt'
import { Crumb, Callout } from '../components/ui'
import type { EmailContent } from '../types'

export default function EmailView() {
  const { name } = useParams()
  const [mail, setMail] = useState<EmailContent | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    get<EmailContent>(`/api/emails/${encodeURIComponent(name ?? '')}`)
      .then(setMail)
      .catch((e) => setErr(errMsg(e)))
  }, [name])

  if (err) return <Callout err={err} />
  if (!mail) return null
  return (
    <div>
      <Crumb items={[{ to: '/emails', label: '邮件' }, { label: mail.name }]} />
      <h1 className="text-[19px] font-semibold my-2">{mail.name}</h1>
      <p className="text-xs text-slate-500">
        任务 <Link className="text-blue-600" to={`/tasks/${mail.task}`}>{mail.task}</Link> · {human(mail.size)} · {ftime(mail.mod_time)}
      </p>
      <pre className="mt-3 bg-slate-900 dark:bg-black text-slate-200 rounded-xl p-4 overflow-auto max-h-[70vh] whitespace-pre-wrap break-words text-[13px] leading-relaxed">
        {mail.content}
      </pre>
    </div>
  )
}
