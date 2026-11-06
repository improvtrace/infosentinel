import { NavLink, Route, Routes } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Runs from './pages/Runs'
import RunDetail from './pages/RunDetail'
import Emails from './pages/Emails'
import EmailView from './pages/EmailView'
import TaskDetail from './pages/TaskDetail'
import ConfigLayout from './pages/config/ConfigLayout'
import Console from './pages/config/Console'
import Topics from './pages/config/Topics'
import Targets from './pages/config/Targets'
import Tasks from './pages/config/Tasks'
import Sources from './pages/config/Sources'
import Settings from './pages/config/Settings'

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Dashboard />} />
        <Route path="runs" element={<Runs />} />
        <Route path="runs/:id" element={<RunDetail />} />
        <Route path="emails" element={<Emails />} />
        <Route path="emails/:name" element={<EmailView />} />
        <Route path="tasks/:name" element={<TaskDetail />} />
        <Route path="config" element={<ConfigLayout />}>
          <Route index element={<Console />} />
          <Route path="topics" element={<Topics />} />
          <Route path="targets" element={<Targets />} />
          <Route path="tasks" element={<Tasks />} />
          <Route path="sources" element={<Sources />} />
          <Route path="settings" element={<Settings />} />
        </Route>
        <Route path="*" element={<div className="empty mt-8">404 页面不存在</div>} />
      </Route>
    </Routes>
  )
}

// 导航项(与路由一致)
export const NAV = [
  { to: '/', label: '概览' },
  { to: '/runs', label: '运行' },
  { to: '/emails', label: '邮件' },
  { to: '/config', label: '配置' },
]

export { NavLink }
