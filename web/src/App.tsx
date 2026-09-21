import { Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from '@/components/Layout'
import { ToastProvider } from '@/components/Toast'
import { ModelsPage } from '@/pages/Models'
import { ModelDetailPage } from '@/pages/ModelDetail'
import { PlaygroundPage } from '@/pages/Playground'
import { SystemPage } from '@/pages/System'
import { DashboardPage } from '@/pages/Dashboard'
import { TooltipProvider } from '@/components/ui/tooltip'

export default function App() {
  return (
    <TooltipProvider delayDuration={200}>
      <ToastProvider>
        <Layout>
          <Routes>
            <Route path="/" element={<Navigate to="/dashboard" replace />} />
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/models" element={<ModelsPage />} />
            <Route path="/models/:id" element={<ModelDetailPage />} />
            <Route path="/playground" element={<PlaygroundPage />} />
            <Route path="/playground/:id" element={<PlaygroundPage />} />
            <Route path="/system" element={<SystemPage />} />
            <Route path="*" element={<Navigate to="/dashboard" replace />} />
          </Routes>
        </Layout>
      </ToastProvider>
    </TooltipProvider>
  )
}
