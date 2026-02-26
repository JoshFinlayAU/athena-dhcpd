import { useState, useCallback } from 'react'
import { useApi } from '@/hooks/useApi'
import { Card } from '@/components/Card'
import { Table, THead, TH, TD, TR } from '@/components/Table'
import { ShieldAlert, Check, Trash2, Clock } from 'lucide-react'
import {
  v2GetRogueServers, v2GetRogueStats, v2AcknowledgeRogue, v2RemoveRogue,
  type RogueServer,
} from '@/lib/api'

function formatTime(ts: string) {
  try { return new Date(ts).toLocaleString() } catch { return ts }
}

export default function RogueServers() {
  const { data: servers, refetch } = useApi(useCallback(() => v2GetRogueServers(), []))
  const { data: stats } = useApi(useCallback(() => v2GetRogueStats(), []))
  const [status, setStatus] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)

  const showStatus = (type: 'success' | 'error', msg: string) => {
    setStatus({ type, msg })
    setTimeout(() => setStatus(null), 4000)
  }

  const handleAck = async (ip: string) => {
    try {
      await v2AcknowledgeRogue(ip)
      showStatus('success', `Acknowledged ${ip}`)
      refetch()
    } catch (e) {
      showStatus('error', e instanceof Error ? e.message : 'Failed')
    }
  }

  const handleRemove = async (ip: string) => {
    try {
      await v2RemoveRogue(ip)
      showStatus('success', `Removed ${ip}`)
      refetch()
    } catch (e) {
      showStatus('error', e instanceof Error ? e.message : 'Failed')
    }
  }

  const list = servers || []

  return (
    <div className="p-6 max-w-7xl space-y-4">
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2">
          <ShieldAlert className="w-6 h-6" /> Rogue DHCP Servers
        </h1>
        <p className="text-sm text-text-secondary mt-0.5">
          Detected unauthorized DHCP servers on the network
          {stats && stats.active > 0 && (
            <span className="ml-2 px-2 py-0.5 rounded-full text-xs font-medium bg-danger/15 text-danger">
              {stats.active} active
            </span>
          )}
        </p>
      </div>

      {status && (
        <div className={`px-4 py-2.5 rounded-lg text-sm ${status.type === 'success' ? 'bg-success/15 text-success' : 'bg-danger/15 text-danger'}`}>
          {status.msg}
        </div>
      )}

      {list.length === 0 ? (
        <Card className="p-8 text-center">
          <ShieldAlert className="w-10 h-10 text-success mx-auto mb-3 opacity-50" />
          <p className="text-sm font-medium text-success">No rogue DHCP servers detected</p>
          <p className="text-xs text-text-muted mt-1">The network appears clean</p>
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <Table>
            <THead>
              <tr>
                <TH>Status</TH>
                <TH>Server IP</TH>
                <TH>Server MAC</TH>
                <TH>Last Offered IP</TH>
                <TH>Last Client</TH>
                <TH>Interface</TH>
                <TH>Count</TH>
                <TH>First Seen</TH>
                <TH>Last Seen</TH>
                <TH>Actions</TH>
              </tr>
            </THead>
            <tbody>
              {list.map(s => (
                <RogueRow key={s.server_ip} server={s} onAck={handleAck} onRemove={handleRemove} />
              ))}
            </tbody>
          </Table>
        </Card>
      )}
    </div>
  )
}

function RogueRow({ server: s, onAck, onRemove }: {
  server: RogueServer
  onAck: (ip: string) => void
  onRemove: (ip: string) => void
}) {
  return (
    <TR>
      <TD>
        {s.acknowledged ? (
          <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-text-muted/15 text-text-muted">ack'd</span>
        ) : (
          <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-danger/15 text-danger animate-pulse">active</span>
        )}
      </TD>
      <TD><span className="font-mono text-xs font-bold text-danger">{s.server_ip}</span></TD>
      <TD><span className="font-mono text-xs">{s.server_mac || '-'}</span></TD>
      <TD><span className="font-mono text-xs">{s.last_offer_ip || '-'}</span></TD>
      <TD><span className="font-mono text-xs">{s.last_client_mac || '-'}</span></TD>
      <TD><span className="text-xs">{s.interface || '-'}</span></TD>
      <TD><span className="text-xs font-medium">{s.count}</span></TD>
      <TD>
        <div className="flex items-center gap-1 text-xs text-text-muted">
          <Clock className="w-3 h-3" /> {formatTime(s.first_seen)}
        </div>
      </TD>
      <TD><span className="text-xs text-text-muted">{formatTime(s.last_seen)}</span></TD>
      <TD>
        <div className="flex gap-1">
          {!s.acknowledged && (
            <button onClick={() => onAck(s.server_ip)} title="Acknowledge"
              className="p-1 rounded hover:bg-surface-overlay text-text-muted hover:text-success transition-colors">
              <Check className="w-3.5 h-3.5" />
            </button>
          )}
          <button onClick={() => onRemove(s.server_ip)} title="Remove"
            className="p-1 rounded hover:bg-surface-overlay text-text-muted hover:text-danger transition-colors">
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      </TD>
    </TR>
  )
}
