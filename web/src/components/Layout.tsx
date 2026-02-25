import { NavLink, Outlet } from 'react-router-dom'
import { useWS } from '@/lib/websocket'
import { useAuth } from '@/lib/auth'
import {
  LayoutDashboard, Network, BookmarkPlus, AlertTriangle,
  Activity, Settings, Shield, ShieldCheck, Wifi, WifiOff, LogOut,
} from 'lucide-react'
import { cn } from '@/lib/utils'

const navItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/leases', icon: Network, label: 'Leases' },
  { to: '/reservations', icon: BookmarkPlus, label: 'Reservations' },
  { to: '/conflicts', icon: AlertTriangle, label: 'Conflicts' },
  { to: '/events', icon: Activity, label: 'Events' },
  { to: '/ha', icon: Shield, label: 'HA Status' },
  { to: '/dns', icon: ShieldCheck, label: 'DNS Filtering' },
  { to: '/config', icon: Settings, label: 'Config' },
]

export default function Layout() {
  const { connected } = useWS()
  const { user, logout } = useAuth()

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Sidebar */}
      <aside className="w-64 flex-shrink-0 border-r border-border bg-surface-raised flex flex-col">
        {/* Logo */}
        <div className="h-16 flex items-center gap-3 px-5 border-b border-border">
          <div className="w-8 h-8 rounded-lg bg-accent flex items-center justify-center">
            <Network className="w-4 h-4 text-white" />
          </div>
          <div>
            <h1 className="text-sm font-semibold tracking-tight">athena-dhcpd</h1>
            <p className="text-[10px] text-text-muted uppercase tracking-widest">dhcp server</p>
          </div>
        </div>

        {/* Nav */}
        <nav className="flex-1 py-4 px-3 space-y-1 overflow-y-auto">
          {navItems.map(({ to, icon: Icon, label }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-all duration-150',
                  isActive
                    ? 'bg-accent/15 text-accent-hover'
                    : 'text-text-secondary hover:text-text-primary hover:bg-surface-overlay'
                )
              }
            >
              <Icon className="w-4 h-4 flex-shrink-0" />
              {label}
            </NavLink>
          ))}
        </nav>

        {/* Footer */}
        <div className="px-4 py-3 border-t border-border space-y-2">
          <div className="flex items-center gap-2 text-xs">
            {connected ? (
              <>
                <Wifi className="w-3.5 h-3.5 text-success" />
                <span className="text-success">Live</span>
              </>
            ) : (
              <>
                <WifiOff className="w-3.5 h-3.5 text-danger" />
                <span className="text-danger">Disconnected</span>
              </>
            )}
          </div>
          {user?.auth_required && user?.authenticated && (
            <div className="flex items-center justify-between">
              <span className="text-xs text-text-muted truncate">{user.username} ({user.role})</span>
              <button
                onClick={logout}
                className="p-1 rounded hover:bg-surface-overlay text-text-muted hover:text-text-primary transition-colors"
                title="Sign out"
              >
                <LogOut className="w-3.5 h-3.5" />
              </button>
            </div>
          )}
        </div>
      </aside>

      {/* Main Content */}
      <main className="flex-1 overflow-y-auto">
        <Outlet />
      </main>
    </div>
  )
}
