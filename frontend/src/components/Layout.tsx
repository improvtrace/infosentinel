import { useEffect, useState } from 'react'
import { NavLink, Link, Outlet, useLocation } from 'react-router-dom'
import { get } from '../api'
import { uptime } from '../fmt'
import { IconArchive, IconConfig, IconDashboard, IconRuns, IconSubs } from './icons'

interface Health {
  version: string
  uptime_sec?: number
  uptime?: string
}

const NAV_GROUPS = [
  {
    label: '运行态势',
    items: [
      { to: '/', label: '概览', icon: IconDashboard, end: true },
      { to: '/subscriptions', label: '订阅管理', icon: IconSubs },
      { to: '/runs', label: '运行历史', icon: IconRuns },
      { to: '/artifacts', label: '产物空间', icon: IconArchive },
    ],
  },
  {
    label: '系统管理',
    items: [{ to: '/config', label: '配置管理', icon: IconConfig }],
  },
]

/** 品牌标识:雷达刻度盘 + 旋转扫描线(全站唯一常驻动效)。 */
function BrandMark() {
  return (
    <span className="brand-mark">
      <svg
        viewBox="0 0 24 24"
        width={18}
        height={18}
        fill="none"
        stroke="rgba(255,255,255,.85)"
        strokeWidth={1.4}
      >
        <circle cx="12" cy="12" r="8.5" opacity=".45" />
        <circle cx="12" cy="12" r="4.5" opacity=".7" />
        <circle cx="16" cy="8" r="1.1" fill="rgba(255,255,255,.95)" stroke="none" />
        <circle cx="12" cy="12" r="1" fill="rgba(255,255,255,.95)" stroke="none" />
      </svg>
      <span className="brand-sweep" />
    </span>
  )
}

export default function Layout() {
  const [info, setInfo] = useState<Health | null>(null)
  const [down, setDown] = useState(false)
  const loc = useLocation()

  useEffect(() => {
    const load = () =>
      get<Health>('/api/healthz')
        .then((h) => {
          setInfo(h)
          setDown(false)
        })
        .catch(() => setDown(true))
    load()
    const t = setInterval(load, 30000)
    return () => clearInterval(t)
  }, [])

  return (
    <div className="min-h-screen">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <BrandMark />
          <span className="text-[15px] font-semibold tracking-tight">InfoSentinel</span>
        </div>

        <nav className="mt-1 flex flex-col">
          {NAV_GROUPS.map((g) => (
            <div key={g.label}>
              <div className="nav-group">{g.label}</div>
              {g.items.map((n) => (
                <NavLink
                  key={n.to}
                  to={n.to}
                  end={n.end}
                  className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}
                >
                  <n.icon />
                  {n.label}
                </NavLink>
              ))}
            </div>
          ))}
        </nav>

        <div className="side-foot">
          <div className="flex items-center gap-2 text-[12px] font-medium">
            <span className={`pulse-dot${down ? ' off' : ''}`} />
            <span className={down ? 'text-bad' : 'text-dim'}>{down ? '服务离线' : '服务运行中'}</span>
          </div>
          <div className="mt-1.5">
            <div className="readout">
              <span className="readout-key">版本</span>
              <span className="readout-val">v{info?.version ?? '—'}</span>
            </div>
            <div className="readout">
              <span className="readout-key">已运行</span>
              <span className="readout-val">
                {info ? (info.uptime ?? uptime(info.uptime_sec ?? 0)) : '—'}
              </span>
            </div>
          </div>
        </div>
      </aside>

      <div className="main-wrap">
        <header className="topbar">
          <span className="text-[12.5px] text-ghost">兴趣资讯收集 Agent</span>
          <div className="ml-auto flex items-center gap-3">
            <span className="chip">
              <span className={`dot${down ? ' !bg-bad' : ''}`} />
              {down ? '离线' : '在线'}
            </span>
            <Link
              to="/config/settings"
              className="rounded-md px-2 py-1 text-[12.5px] text-dim no-underline transition-colors hover:bg-hover hover:text-accent"
            >
              设置
            </Link>
            <a
              href="/api/healthz"
              className="font-mono text-[11.5px] text-ghost no-underline transition-colors hover:text-accent"
            >
              healthz
            </a>
          </div>
        </header>

        <nav className="m-nav">
          {NAV_GROUPS.flatMap((g) => g.items).map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.end}
              className={({ isActive }) => `m-nav-item${isActive ? ' active' : ''}`}
            >
              <n.icon />
              {n.label}
            </NavLink>
          ))}
        </nav>

        <main className="content" key={loc.pathname}>
          <Outlet />
        </main>
      </div>
    </div>
  )
}
