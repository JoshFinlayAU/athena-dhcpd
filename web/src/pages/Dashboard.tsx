import { useCallback } from 'react'
import { Network, AlertTriangle, Activity, Clock, Server } from 'lucide-react'
import { StatCard, Card } from '@/components/Card'
import StatusBadge from '@/components/StatusBadge'
import { usePolling } from '@/hooks/useApi'
import { getHealth, getLeases, getConflictStats, getHAStatus } from '@/lib/api'
import { useWS } from '@/lib/websocket'
import { formatDuration, timeAgo } from '@/lib/utils'

export default function Dashboard() {
  const { events, connected } = useWS()
  const { data: health } = usePolling(useCallback(() => getHealth(), []), 5000)
  const { data: leases } = usePolling(useCallback(() => getLeases(), []), 5000)
  const { data: conflicts } = usePolling(useCallback(() => getConflictStats(), []), 10000)
  const { data: ha } = usePolling(useCallback(() => getHAStatus(), []), 5000)

  return (
    <div className="p-6 space-y-6 max-w-7xl">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Dashboard</h1>
          <p className="text-sm text-text-secondary mt-0.5">Real-time DHCP server overview</p>
        </div>
        <div className="flex items-center gap-2">
          <div className={`w-2 h-2 rounded-full ${connected ? 'bg-success animate-pulse' : 'bg-danger'}`} />
          <span className="text-xs text-text-muted">{connected ? 'Live updates' : 'Reconnecting...'}</span>
        </div>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          label="Active Leases"
          value={leases?.total ?? '—'}
          sub={health ? `${health.lease_count} total in DB` : undefined}
          icon={Network}
          color="bg-accent/15"
        />
        <StatCard
          label="Conflicts"
          value={conflicts?.total_active ?? '—'}
          sub={conflicts ? `${conflicts.total_permanent} permanent` : undefined}
          icon={AlertTriangle}
          color="bg-danger/15"
        />
        <StatCard
          label="Uptime"
          value={health ? formatDuration(health.uptime) : '—'}
          sub={health?.version}
          icon={Clock}
          color="bg-success/15"
        />
        <StatCard
          label="HA State"
          value={ha?.enabled ? ha.state : 'Standalone'}
          sub={ha?.enabled ? `Peer: ${ha.peer_connected ? 'connected' : 'disconnected'}` : 'HA disabled'}
          icon={Server}
          color="bg-info/15"
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Recent Events */}
        <Card className="flex flex-col">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-semibold flex items-center gap-2">
              <Activity className="w-4 h-4 text-accent" />
              Live Events
            </h2>
            <span className="text-xs text-text-muted">{events.length} events</span>
          </div>
          <div className="flex-1 space-y-1 max-h-80 overflow-y-auto pr-1">
            {events.length === 0 ? (
              <p className="text-text-muted text-sm py-8 text-center">Waiting for events...</p>
            ) : (
              events.slice(0, 20).map((evt, i) => (
                <div key={i} className="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-surface-overlay/50 transition-colors">
                  <EventIcon type={evt.type} />
                  <div className="flex-1 min-w-0">
                    <p className="text-xs font-medium truncate">{eventLabel(evt.type)}</p>
                    <p className="text-[11px] text-text-muted truncate font-mono">
                      {evt.lease?.ip || evt.conflict?.ip || ''}
                      {evt.lease?.mac ? ` · ${evt.lease.mac}` : ''}
                      {evt.lease?.hostname ? ` · ${evt.lease.hostname}` : ''}
                    </p>
                  </div>
                  <span className="text-[10px] text-text-muted flex-shrink-0">{timeAgo(evt.timestamp)}</span>
                </div>
              ))
            )}
          </div>
        </Card>

        {/* Conflict Summary */}
        <Card>
          <h2 className="text-sm font-semibold flex items-center gap-2 mb-4">
            <AlertTriangle className="w-4 h-4 text-warning" />
            Conflict Summary
          </h2>
          {conflicts ? (
            <div className="space-y-4">
              <div className="grid grid-cols-3 gap-3">
                <MiniStat label="Active" value={conflicts.total_active} color="text-danger" />
                <MiniStat label="Permanent" value={conflicts.total_permanent} color="text-warning" />
                <MiniStat label="Resolved" value={conflicts.total_resolved} color="text-success" />
              </div>
              {conflicts.by_subnet && Object.keys(conflicts.by_subnet).length > 0 && (
                <div>
                  <p className="text-xs text-text-muted mb-2">By Subnet</p>
                  <div className="space-y-1.5">
                    {Object.entries(conflicts.by_subnet).map(([subnet, count]) => (
                      <div key={subnet} className="flex items-center justify-between text-xs">
                        <span className="font-mono text-text-secondary">{subnet}</span>
                        <StatusBadge status={`${count} conflict${count !== 1 ? 's' : ''}`} />
                      </div>
                    ))}
                  </div>
                </div>
              )}
              {conflicts.by_method && Object.keys(conflicts.by_method).length > 0 && (
                <div>
                  <p className="text-xs text-text-muted mb-2">By Method</p>
                  <div className="flex gap-3">
                    {Object.entries(conflicts.by_method).map(([method, count]) => (
                      <div key={method} className="text-xs">
                        <span className="text-text-muted">{method}: </span>
                        <span className="font-semibold">{count}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : (
            <p className="text-sm text-text-muted py-8 text-center">Loading...</p>
          )}
        </Card>
      </div>
    </div>
  )
}

function MiniStat({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div className="bg-surface-overlay rounded-lg p-3 text-center">
      <p className={`text-xl font-bold tabular-nums ${color}`}>{value}</p>
      <p className="text-[10px] text-text-muted uppercase tracking-wider mt-0.5">{label}</p>
    </div>
  )
}

function EventIcon({ type }: { type: string }) {
  const isConflict = type.includes('conflict')
  const isError = type.includes('nak') || type.includes('decline')
  return (
    <div className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${
      isConflict ? 'bg-danger' : isError ? 'bg-warning' : 'bg-accent'
    }`} />
  )
}

function eventLabel(type: string): string {
  return type.replace(/\./g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}
