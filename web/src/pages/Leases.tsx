import { useState, useCallback, useEffect } from 'react'
import { Search, Trash2, RefreshCw, Download, ChevronLeft, ChevronRight } from 'lucide-react'
import { Table, THead, TH, TD, TR, EmptyRow } from '@/components/Table'
import StatusBadge from '@/components/StatusBadge'
import { useApi } from '@/hooks/useApi'
import { getLeases, deleteLease, type Lease } from '@/lib/api'
import { useWS } from '@/lib/websocket'
import { formatDate, timeAgo } from '@/lib/utils'

export default function Leases() {
  const [search, setSearch] = useState('')
  const [stateFilter, setStateFilter] = useState('')
  const [page, setPage] = useState(1)
  const pageSize = 25

  const params = new URLSearchParams()
  params.set('page', String(page))
  params.set('page_size', String(pageSize))
  if (search) params.set('search', search)
  if (stateFilter) params.set('state', stateFilter)

  const { data, loading, refetch } = useApi(
    useCallback(() => getLeases(params.toString()), [page, search, stateFilter]) // eslint-disable-line react-hooks/exhaustive-deps
  )

  // Auto-refresh on relevant WebSocket events
  const { lastEvent } = useWS()
  useEffect(() => {
    if (!lastEvent) return
    const t = lastEvent.type
    if (t.startsWith('lease.')) refetch()
  }, [lastEvent, refetch])

  const handleDelete = async (ip: string) => {
    if (!confirm(`Delete lease for ${ip}?`)) return
    try {
      await deleteLease(ip)
      refetch()
    } catch (e) {
      alert(`Error: ${e instanceof Error ? e.message : 'Unknown error'}`)
    }
  }

  const totalPages = data ? Math.ceil(data.total / pageSize) : 0

  return (
    <div className="p-6 space-y-4 max-w-7xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Leases</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            {data ? `${data.total} total leases` : 'Loading...'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <a
            href="/api/v1/leases/export"
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
          >
            <Download className="w-3.5 h-3.5" /> Export
          </a>
          <button
            onClick={refetch}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} /> Refresh
          </button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-muted" />
          <input
            type="text"
            placeholder="Search by IP, MAC, hostname..."
            value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(1) }}
            className="w-full pl-9 pr-3 py-2 text-sm rounded-lg border border-border bg-surface-raised text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
          />
        </div>
        <select
          value={stateFilter}
          onChange={(e) => { setStateFilter(e.target.value); setPage(1) }}
          className="px-3 py-2 text-sm rounded-lg border border-border bg-surface-raised text-text-primary focus:outline-none focus:border-accent"
        >
          <option value="">All States</option>
          <option value="active">Active</option>
          <option value="offered">Offered</option>
          <option value="expired">Expired</option>
          <option value="declined">Declined</option>
        </select>
      </div>

      {/* Table */}
      <Table>
        <THead>
          <tr>
            <TH>IP Address</TH>
            <TH>MAC Address</TH>
            <TH>Hostname</TH>
            <TH>State</TH>
            <TH>Subnet</TH>
            <TH>Expires</TH>
            <TH className="w-10" />
          </tr>
        </THead>
        <tbody>
          {!data?.leases?.length ? (
            <EmptyRow cols={7} message={loading ? 'Loading leases...' : 'No leases found'} />
          ) : (
            data.leases.map((l: Lease) => (
              <TR key={l.ip}>
                <TD mono>{l.ip}</TD>
                <TD mono>{l.mac}</TD>
                <TD>{l.hostname || <span className="text-text-muted">—</span>}</TD>
                <TD><StatusBadge status={l.state} /></TD>
                <TD mono>{l.subnet}</TD>
                <TD>
                  <span className="text-text-secondary text-xs" title={formatDate(l.expiry)}>
                    {timeAgo(l.expiry)}
                  </span>
                </TD>
                <TD>
                  <button
                    onClick={() => handleDelete(l.ip)}
                    className="p-1.5 rounded-md hover:bg-danger/15 text-text-muted hover:text-danger transition-colors"
                    title="Delete lease"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </TD>
              </TR>
            ))
          )}
        </tbody>
      </Table>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between text-sm">
          <span className="text-text-muted">
            Page {page} of {totalPages}
          </span>
          <div className="flex items-center gap-1">
            <button
              onClick={() => setPage(p => Math.max(1, p - 1))}
              disabled={page === 1}
              className="p-2 rounded-lg border border-border hover:bg-surface-overlay disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <button
              onClick={() => setPage(p => Math.min(totalPages, p + 1))}
              disabled={page === totalPages}
              className="p-2 rounded-lg border border-border hover:bg-surface-overlay disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
