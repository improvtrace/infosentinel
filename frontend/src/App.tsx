import { NavLink, Route, Routes } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Subscriptions from './pages/Subscriptions'
import SubscriptionDetail from './pages/SubscriptionDetail'
import Runs from './pages/Runs'
import RunDetail from './pages/RunDetail'
import Artifacts from './pages/Artifacts'
import ArtifactView from './pages/ArtifactView'
import ConfigLayout from './pages/config/ConfigLayout'
import Console from './pages/config/Console'
import Topics from './pages/config/Topics'
import Targets from './pages/config/Targets'
import Sources from './pages/config/Sources'
import Settings from './pages/config/Settings'

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Dashboard />} />
        <Route path="subscriptions" element={<Subscriptions />} />
        <Route path="subscriptions/:topic" element={<SubscriptionDetail />} />
        <Route path="runs" element={<Runs />} />
        <Route path="runs/:id" element={<RunDetail />} />
        <Route path="artifacts" element={<Artifacts />} />
        <Route path="artifacts/:bucket/:name" element={<ArtifactView />} />
        <Route path="config" element={<ConfigLayout />}>
          <Route index element={<Console />} />
          <Route path="topics" element={<Topics />} />
          <Route path="targets" element={<Targets />} />
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
  { to: '/subscriptions', label: '订阅管理' },
  { to: '/runs', label: '运行历史' },
  { to: '/artifacts', label: '产物空间' },
  { to: '/config', label: '配置' },
]

export { NavLink }
