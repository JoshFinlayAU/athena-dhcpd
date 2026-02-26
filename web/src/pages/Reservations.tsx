import { useState, useCallback, useMemo } from 'react'
import { Plus, Pencil, Trash2, RefreshCw, X, ArrowUp, ArrowDown } from 'lucide-react'
import { Table, THead, TH, TD, TR, EmptyRow } from '@/components/Table'
import { Card } from '@/components/Card'
import { useApi } from '@/hooks/useApi'
import { getReservations, createReservation, deleteReservation, type Reservation } from '@/lib/api'

type SortKey = 'mac' | 'ip' | 'hostname' | 'identifier'
type SortDir = 'asc' | 'desc'

const emptyRes: Reservation = { mac: '', identifier: '', ip: '', hostname: '' }

function compareIP(a: string, b: string): number {
  const partsA = a.split('.').map(Number)
  const partsB = b.split('.').map(Number)
  for (let i = 0; i < 4; i++) {
    if ((partsA[i] || 0) !== (partsB[i] || 0)) return (partsA[i] || 0) - (partsB[i] || 0)
  }
  return 0
}

export default function Reservations() {
  const { data, loading, refetch } = useApi(useCallback(() => getReservations(), []))
  const [editing, setEditing] = useState<Reservation | null>(null)
  const [isNew, setIsNew] = useState(false)
  const [sortKey, setSortKey] = useState<SortKey>('ip')
  const [sortDir, setSortDir] = useState<SortDir>('asc')

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    else { setSortKey(key); setSortDir('asc') }
  }

  const sorted = useMemo(() => {
    if (!data?.length) return data
    return [...data].sort((a, b) => {
      let cmp: number
      if (sortKey === 'ip') cmp = compareIP(a.ip, b.ip)
      else cmp = (a[sortKey] || '').localeCompare(b[sortKey] || '')
      return sortDir === 'asc' ? cmp : -cmp
    })
  }, [data, sortKey, sortDir])

  const handleAdd = () => { setEditing({ ...emptyRes }); setIsNew(true) }
  const handleEdit = (r: Reservation) => { setEditing({ ...r }); setIsNew(false) }
  const handleCancel = () => { setEditing(null); setIsNew(false) }

  const handleSave = async () => {
    if (!editing) return
    try {
      await createReservation(editing)
      setEditing(null)
      setIsNew(false)
      refetch()
    } catch (e) {
      alert(`Error: ${e instanceof Error ? e.message : 'Unknown error'}`)
    }
  }

  const handleDelete = async (mac: string) => {
    if (!confirm(`Delete reservation for ${mac}?`)) return
    try {
      await deleteReservation(mac)
      refetch()
    } catch (e) {
      alert(`Error: ${e instanceof Error ? e.message : 'Unknown error'}`)
    }
  }

  return (
    <div className="p-6 space-y-4 max-w-7xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Reservations</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            {data ? `${data.length} static leases` : 'Loading...'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={refetch}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} /> Refresh
          </button>
          <button
            onClick={handleAdd}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg bg-accent hover:bg-accent-hover text-white transition-colors"
          >
            <Plus className="w-3.5 h-3.5" /> Add Reservation
          </button>
        </div>
      </div>

      {/* Edit/Add Form */}
      {editing && (
        <Card className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold">{isNew ? 'New Reservation' : 'Edit Reservation'}</h3>
            <button onClick={handleCancel} className="p-1 rounded hover:bg-surface-overlay">
              <X className="w-4 h-4" />
            </button>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
            <FormField label="MAC Address" value={editing.mac} onChange={v => setEditing({ ...editing, mac: v })} placeholder="aa:bb:cc:dd:ee:ff" />
            <FormField label="IP Address" value={editing.ip} onChange={v => setEditing({ ...editing, ip: v })} placeholder="192.168.1.100" />
            <FormField label="Hostname" value={editing.hostname} onChange={v => setEditing({ ...editing, hostname: v })} placeholder="server-01" />
            <FormField label="Identifier" value={editing.identifier} onChange={v => setEditing({ ...editing, identifier: v })} placeholder="Optional" />
          </div>
          <div className="flex justify-end gap-2">
            <button onClick={handleCancel} className="px-4 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors">
              Cancel
            </button>
            <button onClick={handleSave} className="px-4 py-2 text-xs font-medium rounded-lg bg-accent hover:bg-accent-hover text-white transition-colors">
              {isNew ? 'Create' : 'Save'}
            </button>
          </div>
        </Card>
      )}

      {/* Table */}
      <Table>
        <THead>
          <tr>
            <SortTH label="MAC Address" sortKey="mac" currentKey={sortKey} dir={sortDir} onSort={toggleSort} />
            <SortTH label="IP Address" sortKey="ip" currentKey={sortKey} dir={sortDir} onSort={toggleSort} />
            <SortTH label="Hostname" sortKey="hostname" currentKey={sortKey} dir={sortDir} onSort={toggleSort} />
            <SortTH label="Identifier" sortKey="identifier" currentKey={sortKey} dir={sortDir} onSort={toggleSort} />
            <TH className="w-20" />
          </tr>
        </THead>
        <tbody>
          {!sorted?.length ? (
            <EmptyRow cols={5} message={loading ? 'Loading...' : 'No reservations configured'} />
          ) : (
            sorted.map((r) => (
              <TR key={`${r.mac}-${r.ip}`}>
                <TD mono>{r.mac}</TD>
                <TD mono>{r.ip}</TD>
                <TD>{r.hostname || <span className="text-text-muted">—</span>}</TD>
                <TD>{r.identifier || <span className="text-text-muted">—</span>}</TD>
                <TD>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => handleEdit(r)}
                      className="p-1.5 rounded-md hover:bg-accent/15 text-text-muted hover:text-accent transition-colors"
                      title="Edit"
                    >
                      <Pencil className="w-3.5 h-3.5" />
                    </button>
                    <button
                      onClick={() => handleDelete(r.mac)}
                      className="p-1.5 rounded-md hover:bg-danger/15 text-text-muted hover:text-danger transition-colors"
                      title="Delete"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
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

function SortTH({ label, sortKey: key, currentKey, dir, onSort }: {
  label: string; sortKey: SortKey; currentKey: SortKey; dir: SortDir; onSort: (k: SortKey) => void
}) {
  const active = key === currentKey
  return (
    <th
      onClick={() => onSort(key)}
      className="px-4 py-3 text-left text-[11px] font-semibold uppercase tracking-wider text-text-muted cursor-pointer select-none hover:text-text-primary transition-colors"
    >
      <span className="inline-flex items-center gap-1">
        {label}
        {active && (dir === 'asc' ? <ArrowUp className="w-3 h-3" /> : <ArrowDown className="w-3 h-3" />)}
      </span>
    </th>
  )
}

function FormField({ label, value, onChange, placeholder }: {
  label: string; value: string; onChange: (v: string) => void; placeholder?: string
}) {
  return (
    <div>
      <label className="block text-xs font-medium text-text-muted mb-1">{label}</label>
      <input
        type="text"
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent font-mono transition-colors"
      />
    </div>
  )
}
