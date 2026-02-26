import { useState, useEffect, useRef, useCallback } from 'react'
import { useApi } from '@/hooks/useApi'
import { getDNSQueryLog, type DNSQueryLogEntry } from '@/lib/api'
import { Table, THead, TH, TD, TR, EmptyRow } from '@/components/Table'
import { Card } from '@/components/Card'
import { Activity, Pause, Play, Trash2, Search } from 'lucide-react'
import { formatDate } from '@/lib/utils'

// Parse DNS record string to extract the rdata (answer portion).
// Input: "google.com. 93 IN MX 10 smtp.google.com."
// Output: "10 smtp.google.com."
function parseAnswer(raw: string): string {
  // Format: name TTL class type rdata...
  const parts = raw.split(/\s+/)
  // Find the type field — skip name, TTL, class (IN/CH/etc)
  let idx = 0
  if (parts.length > 0) idx++ // skip name
  // skip TTL (numeric)
  while (idx < parts.length && /^\d+$/.test(parts[idx])) idx++
  // skip class (IN, CH, HS, etc)
  if (idx < parts.length && /^(IN|CH|HS|NONE|ANY)$/i.test(parts[idx])) idx++
  // next is the type
  const recType = idx < parts.length ? parts[idx] : ''
  idx++
  // everything after is the rdata
  const rdata = parts.slice(idx).join(' ')
  if (rdata) return rdata
  // fallback: just show the type if no rdata
  return recType || raw
}

const statusColors: Record<string, string> = {
  allowed: 'text-success',
  forwarded: 'text-info',
  cached: 'text-accent',
  local: 'text-accent',
  blocked: 'text-danger',
  failed: 'text-warning',
}

const statusBg: Record<string, string> = {
  allowed: 'bg-success/15',
  forwarded: 'bg-info/15',
  cached: 'bg-accent/15',
  local: 'bg-accent/15',
  blocked: 'bg-danger/15',
  failed: 'bg-warning/15',
}

export default function DNSQueryLog() {
  const [live, setLive] = useState(true)
  const [entries, setEntries] = useState<DNSQueryLogEntry[]>([])
  const [filter, setFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const eventSourceRef = useRef<EventSource | null>(null)
  const maxEntries = 500

  // Load initial entries
  const { data } = useApi(useCallback(() => getDNSQueryLog(200), []))
  useEffect(() => {
    if (data?.entries) {
      setEntries(data.entries)
    }
  }, [data])

  // SSE stream for live updates
  useEffect(() => {
    if (!live) {
      if (eventSourceRef.current) {
        eventSourceRef.current.close()
        eventSourceRef.current = null
      }
      return
    }

    const es = new EventSource('/api/v2/dns/querylog/stream')
    eventSourceRef.current = es

    es.onmessage = (event) => {
      try {
        const entry: DNSQueryLogEntry = JSON.parse(event.data)
        setEntries(prev => {
          const next = [entry, ...prev]
          if (next.length > maxEntries) next.length = maxEntries
          return next
        })
      } catch {
        // ignore parse errors
      }
    }

    es.onerror = () => {
      // EventSource auto-reconnects
    }

    return () => {
      es.close()
      eventSourceRef.current = null
    }
  }, [live])

  const filtered = entries.filter(e => {
    if (filter && !e.name.toLowerCase().includes(filter.toLowerCase()) &&
        !e.source.toLowerCase().includes(filter.toLowerCase())) {
      return false
    }
    if (statusFilter && e.status !== statusFilter) return false
    return true
  })

  const stats = {
    total: entries.length,
    blocked: entries.filter(e => e.status === 'blocked').length,
    cached: entries.filter(e => e.status === 'cached').length,
    forwarded: entries.filter(e => e.status === 'forwarded').length,
  }

  return (
    <div className="p-6 space-y-4 max-w-7xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">DNS Query Log</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            Live streaming DNS queries · {stats.total} entries
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setLive(!live)}
            className={`flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border transition-colors ${
              live ? 'border-success/50 bg-success/10 text-success' : 'border-border hover:bg-surface-overlay text-text-secondary'
            }`}
          >
            {live ? <><Pause className="w-3.5 h-3.5" /> Pause</> : <><Play className="w-3.5 h-3.5" /> Resume</>}
          </button>
          <button
            onClick={() => setEntries([])}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
          >
            <Trash2 className="w-3.5 h-3.5" /> Clear
          </button>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-4 gap-3">
        <Card className="p-3 text-center">
          <div className="text-lg font-bold tabular-nums">{stats.total}</div>
          <div className="text-[10px] text-text-muted uppercase tracking-wider">Total</div>
        </Card>
        <Card className="p-3 text-center">
          <div className="text-lg font-bold tabular-nums text-info">{stats.forwarded}</div>
          <div className="text-[10px] text-text-muted uppercase tracking-wider">Forwarded</div>
        </Card>
        <Card className="p-3 text-center">
          <div className="text-lg font-bold tabular-nums text-accent">{stats.cached}</div>
          <div className="text-[10px] text-text-muted uppercase tracking-wider">Cached</div>
        </Card>
        <Card className="p-3 text-center">
          <div className="text-lg font-bold tabular-nums text-danger">{stats.blocked}</div>
          <div className="text-[10px] text-text-muted uppercase tracking-wider">Blocked</div>
        </Card>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-muted" />
          <input
            type="text"
            placeholder="Filter by domain or source..."
            value={filter}
            onChange={e => setFilter(e.target.value)}
            className="w-full pl-9 pr-3 py-2 text-sm rounded-lg border border-border bg-surface-raised text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
          />
        </div>
        <select
          value={statusFilter}
          onChange={e => setStatusFilter(e.target.value)}
          className="px-3 py-2 text-sm rounded-lg border border-border bg-surface-raised text-text-primary focus:outline-none focus:border-accent"
        >
          <option value="">All Statuses</option>
          <option value="forwarded">Forwarded</option>
          <option value="cached">Cached</option>
          <option value="local">Local</option>
          <option value="blocked">Blocked</option>
          <option value="failed">Failed</option>
        </select>
        {live && (
          <div className="flex items-center gap-1.5 text-xs text-success">
            <Activity className="w-3.5 h-3.5 animate-pulse" />
            <span>Live</span>
          </div>
        )}
      </div>

      {/* Query Table */}
      <Table>
        <THead>
          <tr>
            <TH>Time</TH>
            <TH>Domain</TH>
            <TH>Type</TH>
            <TH>Source</TH>
            <TH>Status</TH>
            <TH>Latency</TH>
            <TH>Answer / Info</TH>
          </tr>
        </THead>
        <tbody>
          {!filtered.length ? (
            <EmptyRow cols={7} message={entries.length ? 'No matching queries' : 'Waiting for DNS queries...'} />
          ) : (
            filtered.slice(0, 200).map((entry, i) => (
              <TR key={`${entry.timestamp}-${i}`}>
                <TD>
                  <span className="text-[11px] text-text-muted tabular-nums">{formatDate(entry.timestamp)}</span>
                </TD>
                <TD mono>
                  <span className="text-xs">{entry.name.replace(/\.$/, '')}</span>
                </TD>
                <TD>
                  <span className="text-xs font-medium text-text-secondary">{entry.type}</span>
                </TD>
                <TD mono>
                  <span className="text-xs text-text-muted">{entry.source.split(':')[0]}</span>
                </TD>
                <TD>
                  <span className={`inline-flex px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider rounded-full ${statusBg[entry.status] || ''} ${statusColors[entry.status] || 'text-text-secondary'}`}>
                    {entry.status}
                  </span>
                </TD>
                <TD>
                  <span className="text-xs tabular-nums text-text-muted">{entry.latency_ms.toFixed(1)}ms</span>
                </TD>
                <TD>
                  {entry.status === 'blocked' ? (
                    <span className="text-xs text-danger">{entry.list_name} ({entry.action})</span>
                  ) : entry.answer ? (
                    <span className="text-xs text-text-muted font-mono" title={entry.answer}>{parseAnswer(entry.answer)}</span>
                  ) : (
                    <span className="text-text-muted">—</span>
                  )}
                </TD>
              </TR>
            ))
          )}
        </tbody>
      </Table>
    </div>
  )
}
