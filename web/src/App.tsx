import { useState, useEffect } from 'react'
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
import Config from '@/pages/ConfigV2'
import DNSFiltering from '@/pages/DNSFiltering'
import DNSQueryLog from '@/pages/DNSQueryLog'
import AuditLog from '@/pages/AuditLog'
import Fingerprints from '@/pages/Fingerprints'
import RogueServers from '@/pages/RogueServers'
import Topology from '@/pages/Topology'
import NetworkWeather from '@/pages/NetworkWeather'
import Login from '@/pages/Login'
import SetupWizard from '@/pages/SetupWizard'
import { getSetupStatus } from '@/lib/api'

function SetupGate({ children }: { children: React.ReactNode }) {
  const [needsSetup, setNeedsSetup] = useState<boolean | null>(null)

  useEffect(() => {
    getSetupStatus()
      .then(s => setNeedsSetup(s.needs_setup))
      .catch(() => setNeedsSetup(false))
  }, [])

  if (needsSetup === null) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-surface-base">
        <div className="text-text-muted text-sm">Loading...</div>
      </div>
    )
  }

  if (needsSetup) {
    return <SetupWizard />
  }

  return <>{children}</>
}

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
    <SetupGate>
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
                  <Route path="/dns/querylog" element={<DNSQueryLog />} />
                  <Route path="/audit" element={<AuditLog />} />
                  <Route path="/fingerprints" element={<Fingerprints />} />
                  <Route path="/rogue" element={<RogueServers />} />
                  <Route path="/topology" element={<Topology />} />
                  <Route path="/weather" element={<NetworkWeather />} />
                  <Route path="/config" element={<Config />} />
                </Route>
              </Routes>
            </BrowserRouter>
          </WSProvider>
        </AuthGate>
      </AuthProvider>
    </SetupGate>
  )
}
