import { useCallback, useEffect } from 'react'
import { AlertTriangle, Trash2, Ban, RefreshCw } from 'lucide-react'
import { Table, THead, TH, TD, TR, EmptyRow } from '@/components/Table'
import StatusBadge from '@/components/StatusBadge'
import { useApi } from '@/hooks/useApi'
import { getConflicts, clearConflict, excludeConflict, type ConflictEntry } from '@/lib/api'
import { useWS } from '@/lib/websocket'
import { formatDate, timeAgo } from '@/lib/utils'

export default function Conflicts() {
  const { data, loading, refetch } = useApi(useCallback(() => getConflicts(), []))
  const { lastEvent } = useWS()

  useEffect(() => {
    if (lastEvent?.type.includes('conflict')) refetch()
  }, [lastEvent, refetch])

  const handleClear = async (ip: string) => {
    if (!confirm(`Clear conflict for ${ip}? It will be eligible for allocation again.`)) return
    try {
      await clearConflict(ip)
      refetch()
    } catch (e) {
      alert(`Error: ${e instanceof Error ? e.message : 'Unknown error'}`)
    }
  }

  const handleExclude = async (ip: string) => {
    if (!confirm(`Permanently exclude ${ip} from allocation?`)) return
    try {
      await excludeConflict(ip)
      refetch()
    } catch (e) {
      alert(`Error: ${e instanceof Error ? e.message : 'Unknown error'}`)
    }
  }

  return (
    <div className="p-6 space-y-4 max-w-7xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Conflicts</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            {data ? `${data.length} active conflicts` : 'Loading...'}
          </p>
        </div>
        <button
          onClick={refetch}
          className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} /> Refresh
        </button>
      </div>

      <Table>
        <THead>
          <tr>
            <TH>IP Address</TH>
            <TH>Subnet</TH>
            <TH>Method</TH>
            <TH>Responder MAC</TH>
            <TH>Status</TH>
            <TH>Probes</TH>
            <TH>Detected</TH>
            <TH>Hold Until</TH>
            <TH className="w-24">Actions</TH>
          </tr>
        </THead>
        <tbody>
          {!data?.length ? (
            <EmptyRow cols={9} message={loading ? 'Loading...' : 'No active conflicts — all clear!'} />
          ) : (
            data.map((c: ConflictEntry) => (
              <TR key={c.ip}>
                <TD mono>{c.ip}</TD>
                <TD mono>{c.subnet}</TD>
                <TD>
                  <span className="inline-flex items-center gap-1.5 text-xs">
                    <AlertTriangle className="w-3 h-3 text-warning" />
                    {c.detection_method}
                  </span>
                </TD>
                <TD mono>{c.responder_mac || '—'}</TD>
                <TD>
                  <StatusBadge status={c.permanent ? 'permanent' : c.resolved ? 'resolved' : 'conflict'} />
                </TD>
                <TD>
                  <span className="tabular-nums font-medium">{c.probe_count}</span>
                </TD>
                <TD>
                  <span className="text-xs text-text-secondary" title={formatDate(c.detected_at)}>
                    {timeAgo(c.detected_at)}
                  </span>
                </TD>
                <TD>
                  <span className="text-xs text-text-secondary" title={formatDate(c.hold_until)}>
                    {timeAgo(c.hold_until)}
                  </span>
                </TD>
                <TD>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => handleClear(c.ip)}
                      className="p-1.5 rounded-md hover:bg-success/15 text-text-muted hover:text-success transition-colors"
                      title="Clear conflict"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                    <button
                      onClick={() => handleExclude(c.ip)}
                      className="p-1.5 rounded-md hover:bg-danger/15 text-text-muted hover:text-danger transition-colors"
                      title="Exclude IP permanently"
                    >
                      <Ban className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </TD>
              </TR>
            ))
          )}
        </tbody>
      </Table>
    </div>
  )
}
