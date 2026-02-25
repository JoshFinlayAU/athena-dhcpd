import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { WSProvider } from '@/lib/websocket'
import { AuthProvider, useAuth } from '@/lib/auth'
import Layout from '@/components/Layout'
import Dashboard from '@/pages/Dashboard'
import Leases from '@/pages/Leases'
import Reservations from '@/pages/Reservations'
import Conflicts from '@/pages/Conflicts'
import Events from '@/pages/Events'
import HAStatus from '@/pages/HAStatus'
import Config from '@/pages/Config'
import DNSFiltering from '@/pages/DNSFiltering'
import Login from '@/pages/Login'

function AuthGate({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth()

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-surface-base">
        <div className="text-text-muted text-sm">Loading...</div>
      </div>
    )
  }

  if (user?.auth_required && !user?.authenticated) {
    return <Login />
  }

  return <>{children}</>
}

export default function App() {
  return (
    <AuthProvider>
      <AuthGate>
        <WSProvider>
          <BrowserRouter>
            <Routes>
              <Route element={<Layout />}>
                <Route path="/" element={<Dashboard />} />
                <Route path="/leases" element={<Leases />} />
                <Route path="/reservations" element={<Reservations />} />
                <Route path="/conflicts" element={<Conflicts />} />
                <Route path="/events" element={<Events />} />
                <Route path="/ha" element={<HAStatus />} />
                <Route path="/dns" element={<DNSFiltering />} />
                <Route path="/config" element={<Config />} />
              </Route>
            </Routes>
          </BrowserRouter>
        </WSProvider>
      </AuthGate>
    </AuthProvider>
  )
}
