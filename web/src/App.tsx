import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { WSProvider } from '@/lib/websocket'
import Layout from '@/components/Layout'
import Dashboard from '@/pages/Dashboard'
import Leases from '@/pages/Leases'
import Reservations from '@/pages/Reservations'
import Conflicts from '@/pages/Conflicts'
import Events from '@/pages/Events'
import HAStatus from '@/pages/HAStatus'
import Config from '@/pages/Config'
import DNSFiltering from '@/pages/DNSFiltering'

export default function App() {
  return (
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
  )
}
