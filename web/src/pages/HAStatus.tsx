import { useCallback } from 'react'
import { Shield, Heart, ArrowRightLeft, Server, Globe, CheckCircle, XCircle, AlertTriangle } from 'lucide-react'
import { Card, StatCard } from '@/components/Card'
import StatusBadge from '@/components/StatusBadge'
import { usePolling } from '@/hooks/useApi'
import { getHAStatus, triggerFailover, type VIPGroupStatus, type VIPEntryStatus } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

export default function HAStatus() {
  const { data: ha, loading, refetch } = usePolling(useCallback(() => getHAStatus(), []), 5000)

  const handleFailover = async () => {
    if (!confirm('Trigger manual failover? This will switch the active/standby roles.')) return
    try {
      await triggerFailover()
      refetch()
    } catch (e) {
      alert(`Error: ${e instanceof Error ? e.message : 'Unknown error'}`)
    }
  }

  if (loading && !ha) {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold mb-4">HA Status</h1>
        <p className="text-text-muted">Loading...</p>
      </div>
    )
  }

  if (!ha?.enabled) {
    return (
      <div className="p-6 max-w-7xl">
        <h1 className="text-2xl font-bold mb-6">HA Status</h1>
        <Card className="flex flex-col items-center justify-center py-16">
          <Shield className="w-12 h-12 text-text-muted mb-4 opacity-30" />
          <h2 className="text-lg font-semibold text-text-secondary">Standalone Mode</h2>
          <p className="text-sm text-text-muted mt-1">High availability is not enabled in the configuration.</p>
        </Card>
      </div>
    )
  }

  return (
    <div className="p-6 space-y-6 max-w-7xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">HA Status</h1>
          <p className="text-sm text-text-secondary mt-0.5">High availability cluster status</p>
        </div>
        <button
          onClick={handleFailover}
          className="flex items-center gap-1.5 px-4 py-2 text-xs font-medium rounded-lg bg-warning/15 text-warning hover:bg-warning/25 border border-warning/30 transition-colors"
        >
          <ArrowRightLeft className="w-3.5 h-3.5" /> Trigger Failover
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <StatCard
          label="Role"
          value={ha.role || '—'}
          icon={Shield}
          color="bg-accent/15"
        />
        <StatCard
          label="State"
          value={ha.state || '—'}
          icon={Server}
          color={ha.state === 'ACTIVE' ? 'bg-success/15' : 'bg-warning/15'}
        />
        <StatCard
          label="Last Heartbeat"
          value={ha.last_heartbeat ? timeAgo(ha.last_heartbeat) : 'Never'}
          icon={Heart}
          color="bg-info/15"
        />
      </div>

      <Card>
        <h2 className="text-sm font-semibold mb-4">Peer Connection</h2>
        <div className="space-y-3">
          <DetailRow label="Peer Address" value={ha.peer_address || '—'} mono />
          <DetailRow label="Connection" value={ha.peer_connected ? 'Connected' : 'Disconnected'}>
            <StatusBadge status={ha.peer_connected ? 'connected' : 'disconnected'} />
          </DetailRow>
          <DetailRow label="State" value={ha.state || '—'}>
            <StatusBadge status={(ha.state || 'unknown').toLowerCase()} />
          </DetailRow>
        </div>
      </Card>

      {ha.vip && ha.vip.configured && <VIPCard vip={ha.vip} />}
    </div>
  )
}

function VIPCard({ vip }: { vip: VIPGroupStatus }) {
  const heldCount = vip.entries?.filter(e => e.held).length ?? 0
  const totalCount = vip.entries?.length ?? 0

  return (
    <Card>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-sm font-semibold flex items-center gap-2">
          <Globe className="w-4 h-4" /> Floating Virtual IPs
        </h2>
        <span className={`px-2.5 py-0.5 rounded-full text-xs font-medium ${
          vip.active ? 'bg-success/15 text-success' : 'bg-text-muted/15 text-text-muted'
        }`}>
          {vip.active ? `${heldCount}/${totalCount} held` : 'standby'}
        </span>
      </div>

      {vip.entries && vip.entries.length > 0 ? (
        <div className="space-y-2">
          {vip.entries.map((entry, i) => (
            <VIPEntryRow key={`${entry.ip}-${entry.interface}-${i}`} entry={entry} />
          ))}
        </div>
      ) : (
        <p className="text-xs text-text-muted">No floating IPs configured. Add them in Configuration &gt; HA.</p>
      )}
    </Card>
  )
}

function VIPEntryRow({ entry }: { entry: VIPEntryStatus }) {
  return (
    <div className="flex items-center justify-between py-2 px-3 border border-border/50 rounded-lg">
      <div className="flex items-center gap-3">
        {entry.error ? (
          <AlertTriangle className="w-4 h-4 text-danger shrink-0" />
        ) : entry.held ? (
          <CheckCircle className="w-4 h-4 text-success shrink-0" />
        ) : (
          <XCircle className="w-4 h-4 text-text-muted shrink-0" />
        )}
        <div>
          <div className="flex items-center gap-2">
            <span className="text-sm font-mono font-medium">{entry.ip}/{entry.cidr}</span>
            <span className="text-[10px] text-text-muted font-mono">dev {entry.interface}</span>
            {entry.label && (
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-accent/10 text-accent">{entry.label}</span>
            )}
          </div>
          {entry.error && (
            <p className="text-[11px] text-danger mt-0.5">{entry.error}</p>
          )}
        </div>
      </div>
      <div className="flex items-center gap-2">
        <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${
          entry.on_local ? 'bg-success/15 text-success' : 'bg-text-muted/15 text-text-muted'
        }`}>
          {entry.on_local ? 'on local' : 'not local'}
        </span>
        <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${
          entry.held ? 'bg-success/15 text-success' : 'bg-text-muted/15 text-text-muted'
        }`}>
          {entry.held ? 'held' : 'released'}
        </span>
      </div>
    </div>
  )
}

function DetailRow({ label, value, mono, children }: {
  label: string; value: string; mono?: boolean; children?: React.ReactNode
}) {
  return (
    <div className="flex items-center justify-between py-2 border-b border-border/50 last:border-0">
      <span className="text-xs text-text-muted">{label}</span>
      {children || <span className={`text-sm ${mono ? 'font-mono' : ''}`}>{value}</span>}
    </div>
  )
}
