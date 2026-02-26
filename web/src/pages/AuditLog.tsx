import { useState, useCallback } from 'react'
import { useApi } from '@/hooks/useApi'
import { Card } from '@/components/Card'
import { Field, FieldGrid, TextInput, Select } from '@/components/FormFields'
import { Table, THead, TH, TD, TR, EmptyRow } from '@/components/Table'
import { Search, Download, FileText, Clock } from 'lucide-react'
import {
  v2QueryAudit, v2AuditStats, v2AuditExportURL,
  type AuditRecord, type AuditQueryParams,
} from '@/lib/api'

const EVENT_OPTIONS = [
  { value: '', label: 'All Events' },
  { value: 'lease.ack', label: 'Lease ACK' },
  { value: 'lease.renew', label: 'Lease Renew' },
  { value: 'lease.release', label: 'Lease Release' },
  { value: 'lease.expire', label: 'Lease Expire' },
  { value: 'lease.decline', label: 'Lease Decline' },
  { value: 'lease.nak', label: 'Lease NAK' },
]

function eventBadge(event: string) {
  const colors: Record<string, string> = {
    'lease.ack': 'bg-success/15 text-success',
    'lease.renew': 'bg-accent/15 text-accent',
    'lease.release': 'bg-warning/15 text-warning',
    'lease.expire': 'bg-text-muted/15 text-text-muted',
    'lease.decline': 'bg-danger/15 text-danger',
    'lease.nak': 'bg-danger/15 text-danger',
  }
  return (
    <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${colors[event] || 'bg-surface text-text-secondary'}`}>
      {event.replace('lease.', '')}
    </span>
  )
}

function formatTime(ts: string) {
  try {
    return new Date(ts).toLocaleString()
  } catch {
    return ts
  }
}

export default function AuditLog() {
  const [filters, setFilters] = useState<AuditQueryParams>({ limit: 200 })
  const [query, setQuery] = useState<AuditQueryParams>({ limit: 200 })

  const { data: stats } = useApi(useCallback(() => v2AuditStats(), []))
  const { data, loading } = useApi(useCallback(() => v2QueryAudit(query), [query]))

  const handleSearch = () => setQuery({ ...filters })

  const handleExport = () => {
    window.open(v2AuditExportURL(query), '_blank')
  }

  const records = data?.records || []

  return (
    <div className="p-6 max-w-7xl space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <FileText className="w-6 h-6" /> Lease Audit Log
          </h1>
          <p className="text-sm text-text-secondary mt-0.5">
            Compliance audit trail — every lease assignment, renewal, release, and expiry
            {stats && <span className="text-text-muted ml-1">({stats.total_records.toLocaleString()} total records)</span>}
          </p>
        </div>
      </div>

      {/* Filters */}
      <Card className="p-4 space-y-3">
        <div className="flex items-center gap-2 text-sm font-medium text-text-secondary">
          <Search className="w-4 h-4" /> Query Filters
        </div>
        <FieldGrid>
          <Field label="IP Address">
            <TextInput value={filters.ip || ''} onChange={v => setFilters({ ...filters, ip: v })} placeholder="10.0.0.100" mono />
          </Field>
          <Field label="MAC Address">
            <TextInput value={filters.mac || ''} onChange={v => setFilters({ ...filters, mac: v })} placeholder="aa:bb:cc:dd:ee:ff" mono />
          </Field>
          <Field label="Event Type">
            <Select value={filters.event || ''} onChange={v => setFilters({ ...filters, event: v })} options={EVENT_OPTIONS} />
          </Field>
        </FieldGrid>
        <FieldGrid>
          <Field label="Point-in-Time" hint="Who had this IP at a specific time? (RFC3339)">
            <TextInput value={filters.at || ''} onChange={v => setFilters({ ...filters, at: v })} placeholder="2025-02-15T14:30:00Z" mono />
          </Field>
          <Field label="From">
            <TextInput value={filters.from || ''} onChange={v => setFilters({ ...filters, from: v })} placeholder="2025-02-15T00:00:00Z" mono />
          </Field>
          <Field label="To">
            <TextInput value={filters.to || ''} onChange={v => setFilters({ ...filters, to: v })} placeholder="2025-02-16T00:00:00Z" mono />
          </Field>
        </FieldGrid>
        <div className="flex items-center justify-between pt-1">
          <span className="text-xs text-text-muted">
            {data && `${data.count} results`}
          </span>
          <div className="flex gap-2">
            <button onClick={handleExport} className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-border hover:bg-surface transition-colors">
              <Download className="w-3.5 h-3.5" /> Export CSV
            </button>
            <button onClick={handleSearch} disabled={loading}
              className="flex items-center gap-1.5 px-4 py-1.5 text-xs font-medium rounded-lg bg-accent text-white hover:bg-accent-hover disabled:opacity-50 transition-colors">
              <Search className="w-3.5 h-3.5" /> {loading ? 'Searching...' : 'Search'}
            </button>
          </div>
        </div>
      </Card>

      {/* Results table */}
      <Card className="overflow-hidden">
        <Table>
          <THead>
            <tr>
              <TH>Time</TH>
              <TH>Event</TH>
              <TH>IP</TH>
              <TH>MAC</TH>
              <TH>Hostname</TH>
              <TH>Subnet</TH>
              <TH>Circuit ID</TH>
              <TH>Server</TH>
            </tr>
          </THead>
          <tbody>
            {records.length === 0 ? (
              <EmptyRow cols={8} message={loading ? 'Loading...' : 'No audit records found'} />
            ) : (
              records.map(r => <AuditRow key={r.id} record={r} />)
            )}
          </tbody>
        </Table>
      </Card>
    </div>
  )
}

function AuditRow({ record: r }: { record: AuditRecord }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <>
      <TR onClick={() => setExpanded(!expanded)} className="cursor-pointer">
        <TD>
          <div className="flex items-center gap-1.5 text-xs">
            <Clock className="w-3 h-3 text-text-muted" />
            {formatTime(r.timestamp)}
          </div>
        </TD>
        <TD>{eventBadge(r.event)}</TD>
        <TD><span className="font-mono text-xs">{r.ip}</span></TD>
        <TD><span className="font-mono text-xs">{r.mac}</span></TD>
        <TD><span className="text-xs">{r.hostname || '-'}</span></TD>
        <TD><span className="text-xs">{r.subnet || '-'}</span></TD>
        <TD><span className="text-xs font-mono">{r.circuit_id || '-'}</span></TD>
        <TD>
          <span className="text-xs">{r.server_id || '-'}</span>
          {r.ha_role && <span className="ml-1 text-text-muted text-xs">({r.ha_role})</span>}
        </TD>
      </TR>
      {expanded && (
        <tr>
          <td colSpan={8} className="px-4 py-3 bg-surface/50 border-b border-border">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-xs">
              <div><span className="text-text-muted">ID:</span> {r.id}</div>
              <div><span className="text-text-muted">Client ID:</span> {r.client_id || '-'}</div>
              <div><span className="text-text-muted">FQDN:</span> {r.fqdn || '-'}</div>
              <div><span className="text-text-muted">Pool:</span> {r.pool || '-'}</div>
              <div><span className="text-text-muted">Lease Start:</span> {r.lease_start ? new Date(r.lease_start * 1000).toLocaleString() : '-'}</div>
              <div><span className="text-text-muted">Lease Expiry:</span> {r.lease_expiry ? new Date(r.lease_expiry * 1000).toLocaleString() : '-'}</div>
              <div><span className="text-text-muted">Remote ID:</span> {r.remote_id || '-'}</div>
              <div><span className="text-text-muted">GIAddr:</span> {r.giaddr || '-'}</div>
              {r.reason && <div className="col-span-2"><span className="text-text-muted">Reason:</span> {r.reason}</div>}
            </div>
          </td>
        </tr>
      )}
    </>
  )
}
