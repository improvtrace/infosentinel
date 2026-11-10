import { NavLink, Outlet } from 'react-router-dom'

const TABS = [
  { to: '.', label: '概览', end: true },
  { to: 'topics', label: '主题' },
  { to: 'targets', label: '推送目标' },
  { to: 'sources', label: '数据源' },
  { to: 'settings', label: '全局设置' },
]

export default function ConfigLayout() {
  return (
    <div>
      <h1 className="page-title">配置</h1>
      <p className="page-desc">
        配置持久化于 SQLite(唯一事实源);修改保存即校验并热生效,无需重启进程。
      </p>
      <nav className="tabs">
        {TABS.map((t) => (
          <NavLink
            key={t.to}
            to={t.to}
            end={t.end}
            className={({ isActive }) => `tab${isActive ? ' active' : ''}`}
          >
            {t.label}
          </NavLink>
        ))}
      </nav>
      <div className="mt-5">
        <Outlet />
      </div>
    </div>
  )
}
