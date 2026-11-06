import { useEffect, useState } from 'react'
import { NavLink, Link, Outlet, useLocation } from 'react-router-dom'
import { get } from '../api'
import { uptime } from '../fmt'
import { IconConfig, IconDashboard, IconMail, IconRuns } from './icons'

interface Health {
  version: string
  uptime_sec?: number
  uptime?: string
}

const NAV = [
  { to: '/', label: '概览', icon: IconDashboard, end: true },
  { to: '/runs', label: '运行历史', icon: IconRuns },
  { to: '/emails', label: '邮件产物', icon: IconMail },
  { to: '/config', label: '配置管理', icon: IconConfig },
]

export default function Layout() {
  const [info, setInfo] = useState<Health | null>(null)
  const loc = useLocation()

  useEffect(() => {
    const load = () => get<Health>('/api/healthz').then(setInfo).catch(() => null)
    load()
    const t = setInterval(load, 30000)
    return () => clearInterval(t)
  }, [])

  return (
    <div className="min-h-screen">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <span className="brand-dot">IS</span>
          <span>InfoSentinel</span>
        </div>

        <nav className="flex flex-col gap-0.5 mt-3">
          {NAV.map((n) => (
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
        </nav>

        <div className="mt-auto px-5 py-4 border-t border-slate-200/70 dark:border-slate-800/70">
          <div className="text-[11.5px] text-slate-400 dark:text-slate-500 leading-relaxed">
            <div className="flex items-center gap-1.5">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 inline-block" />
              服务运行中
            </div>
            <div className="mt-1">v{info?.version ?? '—'}</div>
            <div>已运行 {info ? (info.uptime ?? uptime(info.uptime_sec ?? 0)) : '—'}</div>
          </div>
        </div>
      </aside>

      <div className="main-wrap">
        <header className="topbar">
          <span className="text-[12.5px] text-slate-400 dark:text-slate-500">
            兴趣资讯收集 Agent · Eino
          </span>
          <div className="ml-auto flex items-center gap-2">
            <a
              href="/api/healthz"
              className="text-[12.5px] text-slate-400 hover:text-[#4d6bfe] px-2 py-1 rounded-md hover:bg-slate-100 dark:hover:bg-slate-800/60"
            >
              healthz
            </a>
            <Link
              to="/config/settings"
              className="text-[12.5px] text-slate-400 hover:text-[#4d6bfe] px-2 py-1 rounded-md hover:bg-slate-100 dark:hover:bg-slate-800/60"
            >
              设置
            </Link>
          </div>
        </header>

        <main className="content" key={loc.pathname}>
          <Outlet />
        </main>

        <footer className="px-8 py-5 text-[11.5px] text-slate-400 dark:text-slate-600">
          InfoSentinel v{info?.version ?? '—'} · 配置存储于 SQLite · 界面由 React + kratos 驱动
        </footer>
      </div>
    </div>
  )
}
