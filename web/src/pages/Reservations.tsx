import { useState, useCallback } from 'react'
import { Plus, Pencil, Trash2, RefreshCw, X } from 'lucide-react'
import { Table, THead, TH, TD, TR, EmptyRow } from '@/components/Table'
import { Card } from '@/components/Card'
import { useApi } from '@/hooks/useApi'
import { getReservations, createReservation, deleteReservation, type Reservation } from '@/lib/api'

const emptyRes: Reservation = { mac: '', identifier: '', ip: '', hostname: '' }

export default function Reservations() {
  const { data, loading, refetch } = useApi(useCallback(() => getReservations(), []))
  const [editing, setEditing] = useState<Reservation | null>(null)
  const [isNew, setIsNew] = useState(false)

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
            <TH>MAC Address</TH>
            <TH>IP Address</TH>
            <TH>Hostname</TH>
            <TH>Identifier</TH>
            <TH className="w-20" />
          </tr>
        </THead>
        <tbody>
          {!data?.length ? (
            <EmptyRow cols={5} message={loading ? 'Loading...' : 'No reservations configured'} />
          ) : (
            data.map((r) => (
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
