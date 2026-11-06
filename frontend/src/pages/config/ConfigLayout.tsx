import { NavLink, Outlet } from 'react-router-dom'

const TABS = [
  { to: '.', label: '概览', end: true },
  { to: 'topics', label: '主题' },
  { to: 'targets', label: '推送目标' },
  { to: 'tasks', label: '任务' },
  { to: 'sources', label: '数据源' },
  { to: 'settings', label: '全局设置' },
]

export default function ConfigLayout() {
  return (
    <div>
      <div className="crumb"><span>配置管理</span></div>
      <h1 className="text-[19px] font-semibold my-2">配置</h1>
      <p className="text-xs text-slate-500">
        配置持久化于 SQLite(唯一事实源);修改保存即校验并热生效,无需重启进程。
      </p>
      <div className="flex gap-1 mt-3 border-b border-slate-200 dark:border-slate-800 pb-px overflow-x-auto">
        {TABS.map((t) => (
          <NavLink
            key={t.to}
            to={t.to}
            end={t.end}
            className={({ isActive }) =>
              `px-3.5 py-1.5 text-[13px] rounded-t-lg -mb-px border border-b-0 no-underline whitespace-nowrap ${
                isActive
                  ? 'border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 text-blue-600 dark:text-blue-400 font-semibold rounded-b-none'
                  : 'border-transparent text-slate-500 hover:text-slate-900 dark:hover:text-slate-100'
              }`
            }
          >
            {t.label}
          </NavLink>
        ))}
      </div>
      <div className="mt-4">
        <Outlet />
      </div>
    </div>
  )
}
