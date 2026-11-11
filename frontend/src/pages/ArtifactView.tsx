import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { get, errMsg } from '../api'
import { ftime, human } from '../fmt'
import { Crumb, Callout, PageHead } from '../components/ui'
import type { ArtifactContent } from '../types'

export default function ArtifactView() {
  const { space, bucket, name } = useParams()
  const [art, setArt] = useState<ArtifactContent | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    get<ArtifactContent>(
      `/api/artifacts/${encodeURIComponent(space ?? '')}/${encodeURIComponent(bucket ?? '')}/${encodeURIComponent(name ?? '')}`,
    )
      .then(setArt)
      .catch((e) => setErr(errMsg(e)))
  }, [space, bucket, name])

  if (err) return <Callout err={err} />
  if (!art) return null
  return (
    <div>
      <Crumb items={[{ to: '/artifacts', label: '产物空间' }, { label: art.name }]} />
      <PageHead
        title={<span className="break-all">{art.name}</span>}
        desc={
          <>
            空间 <span className="mono">{art.space}</span>
            {art.subscription && (
              <>
                ，订阅{' '}
                <Link className="text-accent no-underline hover:underline" to={`/subscriptions/${encodeURIComponent(art.subscription)}`}>
                  {art.subscription}
                </Link>
              </>
            )}
            ，分桶 <span className="mono">{art.bucket}</span>，大小 {human(art.size)}
            {art.mod_time && <>，生成于 {ftime(art.mod_time)}</>}
          </>
        }
      />
      {/* 产物为独立 HTML 文档:沙箱 iframe 渲染(禁脚本,防注入) */}
      <iframe
        title={art.name}
        sandbox=""
        srcDoc={art.content}
        className="w-full rounded-xl border border-line bg-surface"
        style={{ height: '72vh' }}
      />
    </div>
  )
}
