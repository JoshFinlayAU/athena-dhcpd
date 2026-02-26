import { useState, useCallback, useRef } from 'react'
import { useApi } from '@/hooks/useApi'
import { Card } from '@/components/Card'
import { Section, Field, FieldGrid, TextInput, NumberInput, Toggle, Select, StringArrayInput } from '@/components/FormFields'
import { Table, THead, TH, TD, TR, EmptyRow } from '@/components/Table'
import {
  Network, Settings, Shield, Zap, Globe, Radio, Upload, Save, Trash2, Plus, FileUp, AlertTriangle,
} from 'lucide-react'
import {
  v2GetSubnets, v2CreateSubnet, v2UpdateSubnet, v2DeleteSubnet,
  v2GetReservations, v2CreateReservation, v2DeleteReservation, v2ImportReservations,
  v2GetDefaults, v2SetDefaults,
  v2GetConflictConfig, v2SetConflictConfig,
  v2GetHAConfig, v2SetHAConfig,
  v2GetHooksConfig, v2SetHooksConfig,
  v2GetDDNSConfig, v2SetDDNSConfig,
  v2GetDNSConfig, v2SetDNSConfig,
  v2ImportTOML,
  type SubnetConfig, type ReservationConfig, type DefaultsConfig,
  type ConflictDetectionConfig, type HAConfigType, type HooksConfigType,
  type DDNSConfigType, type DNSConfigType, type PoolConfig,
} from '@/lib/api'

type Tab = 'subnets' | 'defaults' | 'conflict' | 'ha' | 'hooks' | 'ddns' | 'dns' | 'import'

const tabs: { id: Tab; label: string; icon: typeof Network }[] = [
  { id: 'subnets', label: 'Subnets', icon: Network },
  { id: 'defaults', label: 'Defaults', icon: Settings },
  { id: 'conflict', label: 'Conflict Detection', icon: AlertTriangle },
  { id: 'ha', label: 'High Availability', icon: Shield },
  { id: 'hooks', label: 'Hooks', icon: Zap },
  { id: 'ddns', label: 'Dynamic DNS', icon: Globe },
  { id: 'dns', label: 'DNS Proxy', icon: Radio },
  { id: 'import', label: 'Import', icon: Upload },
]

export default function ConfigV2() {
  const [tab, setTab] = useState<Tab>('subnets')
  const [status, setStatus] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)

  const showStatus = (type: 'success' | 'error', msg: string) => {
    setStatus({ type, msg })
    setTimeout(() => setStatus(null), 4000)
  }

  return (
    <div className="p-6 max-w-7xl space-y-4">
      <div>
        <h1 className="text-2xl font-bold">Configuration</h1>
        <p className="text-sm text-text-secondary mt-0.5">Manage server configuration stored in database</p>
      </div>

      {status && (
        <div className={`px-4 py-2.5 rounded-lg text-sm ${status.type === 'success' ? 'bg-success/15 text-success' : 'bg-danger/15 text-danger'}`}>
          {status.msg}
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 overflow-x-auto border-b border-border pb-px">
        {tabs.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`flex items-center gap-2 px-4 py-2.5 text-sm font-medium whitespace-nowrap border-b-2 transition-colors ${
              tab === t.id
                ? 'border-accent text-accent'
                : 'border-transparent text-text-secondary hover:text-text-primary hover:border-border'
            }`}
          >
            <t.icon className="w-4 h-4" />
            {t.label}
          </button>
        ))}
      </div>

      {/* Tab content */}
      {tab === 'subnets' && <SubnetsTab onStatus={showStatus} />}
      {tab === 'defaults' && <DefaultsTab onStatus={showStatus} />}
      {tab === 'conflict' && <ConflictTab onStatus={showStatus} />}
      {tab === 'ha' && <HATab onStatus={showStatus} />}
      {tab === 'hooks' && <HooksTab onStatus={showStatus} />}
      {tab === 'ddns' && <DDNSTab onStatus={showStatus} />}
      {tab === 'dns' && <DNSTab onStatus={showStatus} />}
      {tab === 'import' && <ImportTab onStatus={showStatus} />}
    </div>
  )
}

type StatusFn = (type: 'success' | 'error', msg: string) => void

// ============== SUBNETS TAB ==============

function SubnetsTab({ onStatus }: { onStatus: StatusFn }) {
  const { data: subnets, refetch } = useApi(useCallback(() => v2GetSubnets(), []))
  const [editing, setEditing] = useState<SubnetConfig | null>(null)
  const [selectedSubnet, setSelectedSubnet] = useState<string | null>(null)
  const [showNew, setShowNew] = useState(false)

  const handleSave = async (sub: SubnetConfig, isNew: boolean) => {
    try {
      if (isNew) {
        await v2CreateSubnet(sub)
        onStatus('success', `Subnet ${sub.network} created`)
      } else {
        await v2UpdateSubnet(sub.network, sub)
        onStatus('success', `Subnet ${sub.network} updated`)
      }
      setEditing(null)
      setShowNew(false)
      refetch()
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Failed to save')
    }
  }

  const handleDelete = async (network: string) => {
    if (!confirm(`Delete subnet ${network}? This will also delete all its pools and reservations.`)) return
    try {
      await v2DeleteSubnet(network)
      onStatus('success', `Subnet ${network} deleted`)
      refetch()
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Failed to delete')
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <button onClick={() => { setShowNew(true); setEditing({ network: '', pool: [], reservation: [] } as SubnetConfig) }}
          className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg bg-accent text-white hover:bg-accent-hover transition-colors">
          <Plus className="w-3.5 h-3.5" /> Add Subnet
        </button>
      </div>

      {(showNew || editing) && (
        <SubnetEditor
          subnet={editing!}
          isNew={showNew}
          onSave={handleSave}
          onCancel={() => { setEditing(null); setShowNew(false) }}
        />
      )}

      {subnets?.map(sub => (
        <Card key={sub.network} className="p-0 overflow-hidden">
          <div className="flex items-center justify-between px-5 py-3 bg-surface-overlay/30">
            <div>
              <span className="font-mono font-semibold text-sm">{sub.network}</span>
              {sub.interface && <span className="text-xs text-accent font-mono ml-2">{sub.interface}</span>}
              {sub.domain_name && <span className="text-xs text-text-muted ml-3">{sub.domain_name}</span>}
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-text-muted">{sub.pool?.length || 0} pools · {sub.reservation?.length || 0} reservations</span>
              <button onClick={() => setSelectedSubnet(selectedSubnet === sub.network ? null : sub.network)}
                className="px-2.5 py-1 text-xs font-medium rounded border border-border hover:bg-surface-overlay transition-colors">
                {selectedSubnet === sub.network ? 'Collapse' : 'Manage'}
              </button>
              <button onClick={() => { setEditing(sub); setShowNew(false) }}
                className="px-2.5 py-1 text-xs font-medium rounded border border-border hover:bg-surface-overlay transition-colors">
                Edit
              </button>
              <button onClick={() => handleDelete(sub.network)}
                className="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
                <Trash2 className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
          {selectedSubnet === sub.network && (
            <div className="border-t border-border">
              <ReservationsPanel network={sub.network} onStatus={onStatus} />
            </div>
          )}
        </Card>
      ))}

      {!subnets?.length && (
        <Card className="p-8 text-center text-text-muted text-sm">No subnets configured. Add one to get started.</Card>
      )}
    </div>
  )
}

function SubnetEditor({ subnet, isNew, onSave, onCancel }: {
  subnet: SubnetConfig; isNew: boolean; onSave: (s: SubnetConfig, isNew: boolean) => void; onCancel: () => void
}) {
  const [s, setS] = useState<SubnetConfig>({ ...subnet, pool: subnet.pool || [], reservation: subnet.reservation || [] })

  const updatePool = (idx: number, field: keyof PoolConfig, val: string) => {
    const pools = [...(s.pool || [])]
    pools[idx] = { ...pools[idx], [field]: val }
    setS({ ...s, pool: pools })
  }
  const addPool = () => setS({ ...s, pool: [...(s.pool || []), { range_start: '', range_end: '' }] })
  const removePool = (idx: number) => setS({ ...s, pool: (s.pool || []).filter((_, i) => i !== idx) })

  return (
    <Card className="p-5 space-y-4 border-accent/30">
      <h3 className="text-sm font-semibold">{isNew ? 'New Subnet' : `Edit ${subnet.network}`}</h3>
      <FieldGrid>
        <Field label="Network" hint="CIDR">
          <TextInput value={s.network} onChange={v => setS({ ...s, network: v })} placeholder="192.168.1.0/24" mono disabled={!isNew} />
        </Field>
        <Field label="Interface" hint="Network interface for this subnet">
          <TextInput value={s.interface || ''} onChange={v => setS({ ...s, interface: v })} placeholder="eth0" mono />
        </Field>
        <Field label="Domain Name">
          <TextInput value={s.domain_name || ''} onChange={v => setS({ ...s, domain_name: v })} placeholder="example.com" />
        </Field>
        <Field label="Lease Time">
          <TextInput value={s.lease_time || ''} onChange={v => setS({ ...s, lease_time: v })} placeholder="12h0m0s" mono />
        </Field>
        <Field label="Routers">
          <StringArrayInput value={s.routers || []} onChange={v => setS({ ...s, routers: v })} placeholder="192.168.1.1" mono />
        </Field>
        <Field label="DNS Servers">
          <StringArrayInput value={s.dns_servers || []} onChange={v => setS({ ...s, dns_servers: v })} placeholder="8.8.8.8" mono />
        </Field>
        <Field label="NTP Servers">
          <StringArrayInput value={s.ntp_servers || []} onChange={v => setS({ ...s, ntp_servers: v })} placeholder="pool.ntp.org" mono />
        </Field>
      </FieldGrid>

      <Section title="IP Pools" defaultOpen>
        {(s.pool || []).map((p, i) => (
          <div key={i} className="flex items-end gap-2">
            <Field label="Start"><TextInput value={p.range_start} onChange={v => updatePool(i, 'range_start', v)} placeholder="192.168.1.10" mono /></Field>
            <Field label="End"><TextInput value={p.range_end} onChange={v => updatePool(i, 'range_end', v)} placeholder="192.168.1.200" mono /></Field>
            <button onClick={() => removePool(i)} className="p-2 mb-0.5 text-text-muted hover:text-danger"><Trash2 className="w-4 h-4" /></button>
          </div>
        ))}
        <button onClick={addPool} className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover"><Plus className="w-3 h-3" /> Add Pool</button>
      </Section>

      <div className="flex justify-end gap-2 pt-2">
        <button onClick={onCancel} className="px-4 py-2 text-sm rounded-lg border border-border hover:bg-surface-overlay transition-colors">Cancel</button>
        <button onClick={() => onSave(s, isNew)} className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg bg-accent text-white hover:bg-accent-hover transition-colors">
          <Save className="w-3.5 h-3.5" /> {isNew ? 'Create' : 'Save'}
        </button>
      </div>
    </Card>
  )
}

// ============== RESERVATIONS PANEL ==============

function ReservationsPanel({ network, onStatus }: { network: string; onStatus: StatusFn }) {
  const { data: reservations, refetch } = useApi(useCallback(() => v2GetReservations(network), [network]))
  const [showAdd, setShowAdd] = useState(false)
  const [newRes, setNewRes] = useState<ReservationConfig>({ mac: '', ip: '', hostname: '' })
  const csvRef = useRef<HTMLInputElement>(null)

  const handleAdd = async () => {
    try {
      await v2CreateReservation(network, newRes)
      onStatus('success', `Reservation ${newRes.mac} added`)
      setNewRes({ mac: '', ip: '', hostname: '' })
      setShowAdd(false)
      refetch()
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Failed to add')
    }
  }

  const handleDelete = async (mac: string) => {
    if (!confirm(`Delete reservation for ${mac}?`)) return
    try {
      await v2DeleteReservation(network, mac)
      onStatus('success', `Reservation ${mac} deleted`)
      refetch()
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Failed to delete')
    }
  }

  const handleCSV = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    try {
      const fd = new FormData()
      fd.append('file', file)
      const result = await v2ImportReservations(network, fd)
      onStatus('success', `Imported: ${(result as { added: number; updated: number }).added} added, ${(result as { added: number; updated: number }).updated} updated`)
      refetch()
    } catch (err) {
      onStatus('error', err instanceof Error ? err.message : 'CSV import failed')
    }
    if (csvRef.current) csvRef.current.value = ''
  }

  return (
    <div className="p-4 space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-xs font-semibold uppercase text-text-muted tracking-wider">Reservations</h4>
        <div className="flex gap-2">
          <input ref={csvRef} type="file" accept=".csv" onChange={handleCSV} className="hidden" />
          <button onClick={() => csvRef.current?.click()}
            className="flex items-center gap-1 px-2 py-1 text-xs rounded border border-border hover:bg-surface-overlay transition-colors">
            <FileUp className="w-3 h-3" /> CSV Import
          </button>
          <button onClick={() => setShowAdd(!showAdd)}
            className="flex items-center gap-1 px-2 py-1 text-xs rounded bg-accent text-white hover:bg-accent-hover transition-colors">
            <Plus className="w-3 h-3" /> Add
          </button>
        </div>
      </div>

      {showAdd && (
        <div className="flex items-end gap-2 p-3 bg-surface-overlay/30 rounded-lg">
          <Field label="MAC"><TextInput value={newRes.mac} onChange={v => setNewRes({ ...newRes, mac: v })} placeholder="00:11:22:33:44:55" mono /></Field>
          <Field label="IP"><TextInput value={newRes.ip} onChange={v => setNewRes({ ...newRes, ip: v })} placeholder="192.168.1.100" mono /></Field>
          <Field label="Hostname"><TextInput value={newRes.hostname || ''} onChange={v => setNewRes({ ...newRes, hostname: v })} placeholder="server1" /></Field>
          <button onClick={handleAdd} className="px-3 py-2 mb-0.5 text-xs font-medium rounded bg-accent text-white hover:bg-accent-hover">Save</button>
          <button onClick={() => setShowAdd(false)} className="px-3 py-2 mb-0.5 text-xs rounded border border-border hover:bg-surface-overlay">Cancel</button>
        </div>
      )}

      <Table>
        <THead>
          <tr><TH>MAC</TH><TH>IP</TH><TH>Hostname</TH><TH className="w-10" /></tr>
        </THead>
        <tbody>
          {!reservations?.length ? (
            <EmptyRow cols={4} message="No reservations" />
          ) : reservations.map(r => (
            <TR key={r.mac}>
              <TD mono>{r.mac}</TD>
              <TD mono>{r.ip}</TD>
              <TD>{r.hostname || <span className="text-text-muted">—</span>}</TD>
              <TD>
                <button onClick={() => handleDelete(r.mac)} className="p-1 rounded hover:bg-danger/10 text-text-muted hover:text-danger transition-colors">
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </TD>
            </TR>
          ))}
        </tbody>
      </Table>
    </div>
  )
}

// ============== DEFAULTS TAB ==============

function DefaultsTab({ onStatus }: { onStatus: StatusFn }) {
  const { data, refetch } = useApi(useCallback(() => v2GetDefaults(), []))
  const [d, setD] = useState<DefaultsConfig | null>(null)
  const current = d || data

  const handleSave = async () => {
    if (!current) return
    try {
      await v2SetDefaults(current)
      onStatus('success', 'Defaults saved')
      setD(null)
      refetch()
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Failed to save')
    }
  }

  if (!current) return <Card className="p-8 text-center text-text-muted">Loading...</Card>

  return (
    <Card className="p-5 space-y-4">
      <FieldGrid>
        <Field label="Lease Time"><TextInput value={current.lease_time || ''} onChange={v => setD({ ...current, lease_time: v })} placeholder="12h0m0s" mono /></Field>
        <Field label="Renewal Time (T1)"><TextInput value={current.renewal_time || ''} onChange={v => setD({ ...current, renewal_time: v })} placeholder="6h0m0s" mono /></Field>
        <Field label="Rebind Time (T2)"><TextInput value={current.rebind_time || ''} onChange={v => setD({ ...current, rebind_time: v })} placeholder="10h30m0s" mono /></Field>
        <Field label="Domain Name"><TextInput value={current.domain_name || ''} onChange={v => setD({ ...current, domain_name: v })} placeholder="example.com" /></Field>
      </FieldGrid>
      <Field label="DNS Servers">
        <StringArrayInput value={current.dns_servers || []} onChange={v => setD({ ...current, dns_servers: v })} placeholder="8.8.8.8" mono />
      </Field>
      <div className="flex justify-end pt-2">
        <button onClick={handleSave} className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg bg-accent text-white hover:bg-accent-hover transition-colors">
          <Save className="w-3.5 h-3.5" /> Save Defaults
        </button>
      </div>
    </Card>
  )
}

// ============== CONFLICT DETECTION TAB ==============

function ConflictTab({ onStatus }: { onStatus: StatusFn }) {
  const { data, refetch } = useApi(useCallback(() => v2GetConflictConfig(), []))
  const [c, setC] = useState<ConflictDetectionConfig | null>(null)
  const current = c || data

  const handleSave = async () => {
    if (!current) return
    try {
      await v2SetConflictConfig(current)
      onStatus('success', 'Conflict detection settings saved')
      setC(null)
      refetch()
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Failed to save')
    }
  }

  if (!current) return <Card className="p-8 text-center text-text-muted">Loading...</Card>

  return (
    <Card className="p-5 space-y-4">
      <Toggle checked={current.enabled} onChange={v => setC({ ...current, enabled: v })} label="Enable Conflict Detection"
        description="Probe candidate IPs before every DHCPOFFER to prevent conflicts" />
      <FieldGrid>
        <Field label="Probe Strategy">
          <Select value={current.probe_strategy || 'sequential'} onChange={v => setC({ ...current, probe_strategy: v })}
            options={[{ value: 'sequential', label: 'Sequential' }, { value: 'parallel', label: 'Parallel' }]} />
        </Field>
        <Field label="Probe Timeout"><TextInput value={current.probe_timeout || ''} onChange={v => setC({ ...current, probe_timeout: v })} placeholder="500ms" mono /></Field>
        <Field label="Max Probes per Discover"><NumberInput value={current.max_probes_per_discover} onChange={v => setC({ ...current, max_probes_per_discover: v })} min={1} /></Field>
        <Field label="Parallel Probe Count"><NumberInput value={current.parallel_probe_count} onChange={v => setC({ ...current, parallel_probe_count: v })} min={1} /></Field>
        <Field label="Conflict Hold Time"><TextInput value={current.conflict_hold_time || ''} onChange={v => setC({ ...current, conflict_hold_time: v })} placeholder="1h0m0s" mono /></Field>
        <Field label="Max Conflict Count"><NumberInput value={current.max_conflict_count} onChange={v => setC({ ...current, max_conflict_count: v })} min={1} /></Field>
        <Field label="Probe Cache TTL"><TextInput value={current.probe_cache_ttl || ''} onChange={v => setC({ ...current, probe_cache_ttl: v })} placeholder="10s" mono /></Field>
      </FieldGrid>
      <Toggle checked={current.send_gratuitous_arp} onChange={v => setC({ ...current, send_gratuitous_arp: v })} label="Send Gratuitous ARP" description="Send gratuitous ARP after DHCPACK on local subnets" />
      <Toggle checked={current.icmp_fallback} onChange={v => setC({ ...current, icmp_fallback: v })} label="ICMP Fallback" description="Use ICMP ping for relayed/remote subnets" />
      <div className="flex justify-end pt-2">
        <button onClick={handleSave} className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg bg-accent text-white hover:bg-accent-hover transition-colors">
          <Save className="w-3.5 h-3.5" /> Save
        </button>
      </div>
    </Card>
  )
}

// ============== HA TAB ==============

function HATab({ onStatus }: { onStatus: StatusFn }) {
  const { data, refetch } = useApi(useCallback(() => v2GetHAConfig(), []))
  const [h, setH] = useState<HAConfigType | null>(null)
  const current = h || data

  const handleSave = async () => {
    if (!current) return
    try {
      await v2SetHAConfig(current)
      onStatus('success', 'HA settings saved')
      setH(null)
      refetch()
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Failed to save')
    }
  }

  if (!current) return <Card className="p-8 text-center text-text-muted">Loading...</Card>

  return (
    <Card className="p-5 space-y-4">
      <Toggle checked={current.enabled} onChange={v => setH({ ...current, enabled: v })} label="Enable High Availability"
        description="Synchronize leases and conflicts with a peer node" />
      <FieldGrid>
        <Field label="Role">
          <Select value={current.role || ''} onChange={v => setH({ ...current, role: v })}
            options={[{ value: 'primary', label: 'Primary' }, { value: 'secondary', label: 'Secondary' }]} placeholder="Select role" />
        </Field>
        <Field label="Peer Address"><TextInput value={current.peer_address || ''} onChange={v => setH({ ...current, peer_address: v })} placeholder="10.0.0.2:9067" mono /></Field>
        <Field label="Listen Address"><TextInput value={current.listen_address || ''} onChange={v => setH({ ...current, listen_address: v })} placeholder="0.0.0.0:9067" mono /></Field>
        <Field label="Heartbeat Interval"><TextInput value={current.heartbeat_interval || ''} onChange={v => setH({ ...current, heartbeat_interval: v })} placeholder="1s" mono /></Field>
        <Field label="Failover Timeout"><TextInput value={current.failover_timeout || ''} onChange={v => setH({ ...current, failover_timeout: v })} placeholder="10s" mono /></Field>
        <Field label="Sync Batch Size"><NumberInput value={current.sync_batch_size} onChange={v => setH({ ...current, sync_batch_size: v })} min={1} /></Field>
      </FieldGrid>
      {current.tls && (
        <Section title="TLS" defaultOpen={current.tls.enabled}>
          <Toggle checked={current.tls.enabled} onChange={v => setH({ ...current, tls: { ...current.tls, enabled: v } })} label="Enable TLS" />
          <FieldGrid>
            <Field label="Certificate File"><TextInput value={current.tls.cert_file || ''} onChange={v => setH({ ...current, tls: { ...current.tls, cert_file: v } })} /></Field>
            <Field label="Key File"><TextInput value={current.tls.key_file || ''} onChange={v => setH({ ...current, tls: { ...current.tls, key_file: v } })} /></Field>
            <Field label="CA File"><TextInput value={current.tls.ca_file || ''} onChange={v => setH({ ...current, tls: { ...current.tls, ca_file: v } })} /></Field>
          </FieldGrid>
        </Section>
      )}
      <div className="flex justify-end pt-2">
        <button onClick={handleSave} className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg bg-accent text-white hover:bg-accent-hover transition-colors">
          <Save className="w-3.5 h-3.5" /> Save
        </button>
      </div>
    </Card>
  )
}

// ============== HOOKS TAB ==============

function HooksTab({ onStatus }: { onStatus: StatusFn }) {
  const { data, refetch } = useApi(useCallback(() => v2GetHooksConfig(), []))
  const [h, setH] = useState<HooksConfigType | null>(null)
  const current = h || data

  const handleSave = async () => {
    if (!current) return
    try {
      await v2SetHooksConfig(current)
      onStatus('success', 'Hooks settings saved')
      setH(null)
      refetch()
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Failed to save')
    }
  }

  if (!current) return <Card className="p-8 text-center text-text-muted">Loading...</Card>

  return (
    <Card className="p-5 space-y-4">
      <FieldGrid>
        <Field label="Event Buffer Size"><NumberInput value={current.event_buffer_size} onChange={v => setH({ ...current, event_buffer_size: v })} min={100} /></Field>
        <Field label="Script Concurrency"><NumberInput value={current.script_concurrency} onChange={v => setH({ ...current, script_concurrency: v })} min={1} /></Field>
        <Field label="Script Timeout"><TextInput value={current.script_timeout || ''} onChange={v => setH({ ...current, script_timeout: v })} placeholder="10s" mono /></Field>
      </FieldGrid>

      <Section title={`Script Hooks (${current.script?.length || 0})`}>
        {(current.script || []).map((s, i) => (
          <div key={i} className="p-3 bg-surface-overlay/30 rounded-lg space-y-2">
            <div className="flex justify-between items-center">
              <span className="text-sm font-medium">{s.name || `Script ${i + 1}`}</span>
              <button onClick={() => setH({ ...current, script: (current.script || []).filter((_, idx) => idx !== i) })}
                className="p-1 text-text-muted hover:text-danger"><Trash2 className="w-3.5 h-3.5" /></button>
            </div>
            <FieldGrid>
              <Field label="Name"><TextInput value={s.name} onChange={v => {
                const scripts = [...(current.script || [])]; scripts[i] = { ...scripts[i], name: v }; setH({ ...current, script: scripts })
              }} /></Field>
              <Field label="Command"><TextInput value={s.command} onChange={v => {
                const scripts = [...(current.script || [])]; scripts[i] = { ...scripts[i], command: v }; setH({ ...current, script: scripts })
              }} mono /></Field>
            </FieldGrid>
          </div>
        ))}
        <button onClick={() => setH({ ...current, script: [...(current.script || []), { name: '', events: [], command: '', timeout: '10s' }] })}
          className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover"><Plus className="w-3 h-3" /> Add Script Hook</button>
      </Section>

      <Section title={`Webhooks (${current.webhook?.length || 0})`}>
        {(current.webhook || []).map((wh, i) => (
          <div key={i} className="p-3 bg-surface-overlay/30 rounded-lg space-y-2">
            <div className="flex justify-between items-center">
              <span className="text-sm font-medium">{wh.name || `Webhook ${i + 1}`}</span>
              <button onClick={() => setH({ ...current, webhook: (current.webhook || []).filter((_, idx) => idx !== i) })}
                className="p-1 text-text-muted hover:text-danger"><Trash2 className="w-3.5 h-3.5" /></button>
            </div>
            <FieldGrid>
              <Field label="Name"><TextInput value={wh.name} onChange={v => {
                const hooks = [...(current.webhook || [])]; hooks[i] = { ...hooks[i], name: v }; setH({ ...current, webhook: hooks })
              }} /></Field>
              <Field label="URL"><TextInput value={wh.url} onChange={v => {
                const hooks = [...(current.webhook || [])]; hooks[i] = { ...hooks[i], url: v }; setH({ ...current, webhook: hooks })
              }} mono /></Field>
            </FieldGrid>
          </div>
        ))}
        <button onClick={() => setH({ ...current, webhook: [...(current.webhook || []), { name: '', events: [], url: '', method: 'POST', timeout: '10s', retries: 3, retry_backoff: '2s' }] })}
          className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover"><Plus className="w-3 h-3" /> Add Webhook</button>
      </Section>

      <div className="flex justify-end pt-2">
        <button onClick={handleSave} className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg bg-accent text-white hover:bg-accent-hover transition-colors">
          <Save className="w-3.5 h-3.5" /> Save
        </button>
      </div>
    </Card>
  )
}

// ============== DDNS TAB ==============

function DDNSTab({ onStatus }: { onStatus: StatusFn }) {
  const { data, refetch } = useApi(useCallback(() => v2GetDDNSConfig(), []))
  const [d, setD] = useState<DDNSConfigType | null>(null)
  const current = d || data

  const handleSave = async () => {
    if (!current) return
    try {
      await v2SetDDNSConfig(current)
      onStatus('success', 'DDNS settings saved')
      setD(null)
      refetch()
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Failed to save')
    }
  }

  if (!current) return <Card className="p-8 text-center text-text-muted">Loading...</Card>

  return (
    <Card className="p-5 space-y-4">
      <Toggle checked={current.enabled} onChange={v => setD({ ...current, enabled: v })} label="Enable Dynamic DNS"
        description="Automatically register A/PTR records for DHCP leases" />
      <FieldGrid>
        <Field label="TTL"><NumberInput value={current.ttl} onChange={v => setD({ ...current, ttl: v })} min={60} /></Field>
        <Field label="Conflict Policy">
          <Select value={current.conflict_policy || 'overwrite'} onChange={v => setD({ ...current, conflict_policy: v })}
            options={[{ value: 'overwrite', label: 'Overwrite' }, { value: 'skip', label: 'Skip' }, { value: 'append', label: 'Append' }]} />
        </Field>
      </FieldGrid>
      <Toggle checked={current.allow_client_fqdn} onChange={v => setD({ ...current, allow_client_fqdn: v })} label="Allow Client FQDN (Option 81)" />
      <Toggle checked={current.fallback_to_mac} onChange={v => setD({ ...current, fallback_to_mac: v })} label="Fallback to MAC" description="Generate hostname from MAC if none provided" />
      <Toggle checked={current.update_on_renew} onChange={v => setD({ ...current, update_on_renew: v })} label="Update on Renew" />
      <Toggle checked={current.use_dhcid} onChange={v => setD({ ...current, use_dhcid: v })} label="Use DHCID Records (RFC 4701)" />

      {current.forward && (
        <Section title="Forward Zone">
          <FieldGrid>
            <Field label="Zone"><TextInput value={current.forward.zone || ''} onChange={v => setD({ ...current, forward: { ...current.forward, zone: v } })} placeholder="example.com" /></Field>
            <Field label="Method">
              <Select value={current.forward.method || ''} onChange={v => setD({ ...current, forward: { ...current.forward, method: v } })}
                options={[{ value: 'rfc2136', label: 'RFC 2136' }, { value: 'powerdns_api', label: 'PowerDNS API' }, { value: 'technitium_api', label: 'Technitium API' }]} placeholder="Select" />
            </Field>
            <Field label="Server"><TextInput value={current.forward.server || ''} onChange={v => setD({ ...current, forward: { ...current.forward, server: v } })} placeholder="ns1.example.com:53" mono /></Field>
            <Field label="TSIG Name"><TextInput value={current.forward.tsig_name || ''} onChange={v => setD({ ...current, forward: { ...current.forward, tsig_name: v } })} /></Field>
          </FieldGrid>
        </Section>
      )}

      <div className="flex justify-end pt-2">
        <button onClick={handleSave} className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg bg-accent text-white hover:bg-accent-hover transition-colors">
          <Save className="w-3.5 h-3.5" /> Save
        </button>
      </div>
    </Card>
  )
}

// ============== DNS PROXY TAB ==============

function DNSTab({ onStatus }: { onStatus: StatusFn }) {
  const { data, refetch } = useApi(useCallback(() => v2GetDNSConfig(), []))
  const [d, setD] = useState<DNSConfigType | null>(null)
  const current = d || data

  const handleSave = async () => {
    if (!current) return
    try {
      await v2SetDNSConfig(current)
      onStatus('success', 'DNS proxy settings saved')
      setD(null)
      refetch()
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Failed to save')
    }
  }

  if (!current) return <Card className="p-8 text-center text-text-muted">Loading...</Card>

  return (
    <Card className="p-5 space-y-4">
      <Toggle checked={current.enabled} onChange={v => setD({ ...current, enabled: v })} label="Enable DNS Proxy"
        description="Built-in DNS server with DHCP lease registration and filtering" />
      <FieldGrid>
        <Field label="Listen UDP"><TextInput value={current.listen_udp || ''} onChange={v => setD({ ...current, listen_udp: v })} placeholder="0.0.0.0:53" mono /></Field>
        <Field label="Domain"><TextInput value={current.domain || ''} onChange={v => setD({ ...current, domain: v })} placeholder="local" /></Field>
        <Field label="TTL"><NumberInput value={current.ttl} onChange={v => setD({ ...current, ttl: v })} min={1} /></Field>
        <Field label="Cache Size"><NumberInput value={current.cache_size} onChange={v => setD({ ...current, cache_size: v })} min={0} /></Field>
        <Field label="Cache TTL"><TextInput value={current.cache_ttl || ''} onChange={v => setD({ ...current, cache_ttl: v })} placeholder="5m0s" mono /></Field>
      </FieldGrid>
      <Toggle checked={current.register_leases} onChange={v => setD({ ...current, register_leases: v })} label="Register Leases" description="Auto-create DNS A records for active leases" />
      <Toggle checked={current.register_leases_ptr} onChange={v => setD({ ...current, register_leases_ptr: v })} label="Register PTR Records" />
      <Toggle checked={current.use_root_servers} onChange={v => setD({ ...current, use_root_servers: v })} label="Use Root Servers" />
      <Field label="Forwarders">
        <StringArrayInput value={current.forwarders || []} onChange={v => setD({ ...current, forwarders: v })} placeholder="8.8.8.8:53" mono />
      </Field>
      <div className="flex justify-end pt-2">
        <button onClick={handleSave} className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg bg-accent text-white hover:bg-accent-hover transition-colors">
          <Save className="w-3.5 h-3.5" /> Save
        </button>
      </div>
    </Card>
  )
}

// ============== IMPORT TAB ==============

function ImportTab({ onStatus }: { onStatus: StatusFn }) {
  const [toml, setToml] = useState('')
  const [importing, setImporting] = useState(false)

  const handleImport = async () => {
    if (!toml.trim()) return
    setImporting(true)
    try {
      const result = await v2ImportTOML(toml)
      onStatus('success', `Imported ${result.subnets} subnets from TOML`)
      setToml('')
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Import failed')
    }
    setImporting(false)
  }

  return (
    <div className="space-y-6">
      <Card className="p-5 space-y-4">
        <div>
          <h3 className="text-sm font-semibold">Import v1 TOML Configuration</h3>
          <p className="text-xs text-text-muted mt-1">Paste a full athena-dhcpd TOML config file to import all dynamic configuration (subnets, defaults, hooks, DDNS, DNS, etc.) into the database.</p>
        </div>
        <textarea
          value={toml}
          onChange={e => setToml(e.target.value)}
          placeholder="# Paste your TOML configuration here..."
          spellCheck={false}
          className="w-full h-64 p-4 bg-surface text-sm font-mono rounded-lg border border-border focus:outline-none focus:border-accent resize-y"
        />
        <div className="flex justify-end">
          <button onClick={handleImport} disabled={importing || !toml.trim()}
            className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg bg-accent text-white hover:bg-accent-hover disabled:opacity-50 transition-colors">
            <Upload className="w-3.5 h-3.5" /> {importing ? 'Importing...' : 'Import'}
          </button>
        </div>
      </Card>
    </div>
  )
}
