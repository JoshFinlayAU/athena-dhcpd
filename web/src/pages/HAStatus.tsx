import { useCallback } from 'react'
import { Shield, Heart, ArrowRightLeft, Server, Radio, Wifi, WifiOff } from 'lucide-react'
import { Card, StatCard } from '@/components/Card'
import StatusBadge from '@/components/StatusBadge'
import { usePolling } from '@/hooks/useApi'
import { getHAStatus, triggerFailover, type VRRPStatus } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

function vrrpStateColor(state: string): string {
  switch (state) {
    case 'MASTER': return 'bg-success/15 text-success'
    case 'BACKUP': return 'bg-warning/15 text-warning'
    case 'FAULT': return 'bg-danger/15 text-danger'
    case 'STOPPED': return 'bg-text-muted/15 text-text-muted'
    default: return 'bg-text-muted/15 text-text-muted'
  }
}

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

  if (!ha?.enabled && !ha?.vrrp) {
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
        {ha?.enabled && (
          <button
            onClick={handleFailover}
            className="flex items-center gap-1.5 px-4 py-2 text-xs font-medium rounded-lg bg-warning/15 text-warning hover:bg-warning/25 border border-warning/30 transition-colors"
          >
            <ArrowRightLeft className="w-3.5 h-3.5" /> Trigger Failover
          </button>
        )}
      </div>

      {ha?.enabled && (
        <>
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
        </>
      )}

      {ha?.vrrp && <VRRPCard vrrp={ha.vrrp} />}
    </div>
  )
}

function VRRPCard({ vrrp }: { vrrp: VRRPStatus }) {
  return (
    <Card>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-sm font-semibold flex items-center gap-2">
          <Radio className="w-4 h-4" /> VRRP / Keepalived
        </h2>
        <span className={`px-2.5 py-0.5 rounded-full text-xs font-medium ${vrrpStateColor(vrrp.state)}`}>
          {vrrp.state}
        </span>
      </div>
      <div className="space-y-3">
        <DetailRow label="Keepalived" value={vrrp.running ? 'Running' : 'Stopped'}>
          <div className="flex items-center gap-1.5 text-sm">
            {vrrp.running
              ? <><Wifi className="w-3.5 h-3.5 text-success" /><span className="text-success">Running</span></>
              : <><WifiOff className="w-3.5 h-3.5 text-danger" /><span className="text-danger">Stopped</span></>
            }
          </div>
        </DetailRow>
        {vrrp.instance_name && (
          <DetailRow label="Instance" value={vrrp.instance_name} mono />
        )}
        {vrrp.vip && (
          <DetailRow label="Virtual IP" value={vrrp.vip} mono>
            <div className="flex items-center gap-2">
              <span className="font-mono text-sm">{vrrp.vip}</span>
              <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${vrrp.vip_on_local ? 'bg-success/15 text-success' : 'bg-text-muted/15 text-text-muted'}`}>
                {vrrp.vip_on_local ? 'local' : 'not local'}
              </span>
            </div>
          </DetailRow>
        )}
        {vrrp.interface && (
          <DetailRow label="Interface" value={vrrp.interface} mono />
        )}
        {vrrp.priority != null && vrrp.priority > 0 && (
          <DetailRow label="Priority" value={String(vrrp.priority)} />
        )}
      </div>
    </Card>
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
