import { useState, useCallback, useRef } from 'react'
import { useApi } from '@/hooks/useApi'
import { Card } from '@/components/Card'
import { Section, Field, FieldGrid, TextInput, NumberInput, Toggle, Select, StringArrayInput } from '@/components/FormFields'
import { Table, THead, TH, TD, TR, EmptyRow } from '@/components/Table'
import {
  Network, Settings, Shield, Zap, Globe, Radio, Upload, Save, Trash2, Plus, FileUp, AlertTriangle, Type,
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
  v2GetHostnameSanitisation, v2SetHostnameSanitisation,
  v2ImportTOML,
  type SubnetConfig, type ReservationConfig, type DefaultsConfig,
  type ConflictDetectionConfig, type HAConfigType, type HooksConfigType,
  type DDNSConfigType, type DDNSZoneType, type DNSConfigType, type PoolConfig,
  type HostnameSanitisationConfig,
} from '@/lib/api'

type Tab = 'subnets' | 'defaults' | 'conflict' | 'ha' | 'hooks' | 'ddns' | 'dns' | 'hostname' | 'import'

const tabs: { id: Tab; label: string; icon: typeof Network }[] = [
  { id: 'subnets', label: 'Subnets', icon: Network },
  { id: 'defaults', label: 'Defaults', icon: Settings },
  { id: 'conflict', label: 'Conflict Detection', icon: AlertTriangle },
  { id: 'ha', label: 'High Availability', icon: Shield },
  { id: 'hooks', label: 'Hooks', icon: Zap },
  { id: 'ddns', label: 'Dynamic DNS', icon: Globe },
  { id: 'dns', label: 'DNS Proxy', icon: Radio },
  { id: 'hostname', label: 'Hostname Sanitisation', icon: Type },
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
      {tab === 'hostname' && <HostnameSanitisationTab onStatus={showStatus} />}
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
        <Field label="Renewal Time (T1)">
          <TextInput value={s.renewal_time || ''} onChange={v => setS({ ...s, renewal_time: v })} placeholder="6h0m0s" mono />
        </Field>
        <Field label="Rebind Time (T2)">
          <TextInput value={s.rebind_time || ''} onChange={v => setS({ ...s, rebind_time: v })} placeholder="10h30m0s" mono />
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
          <div key={i} className="p-3 bg-surface-overlay/30 rounded-lg space-y-2 mb-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-text-muted">Pool {i + 1}</span>
              <button onClick={() => removePool(i)} className="p-1 text-text-muted hover:text-danger"><Trash2 className="w-3.5 h-3.5" /></button>
            </div>
            <FieldGrid>
              <Field label="Range Start"><TextInput value={p.range_start} onChange={v => updatePool(i, 'range_start', v)} placeholder="192.168.1.10" mono /></Field>
              <Field label="Range End"><TextInput value={p.range_end} onChange={v => updatePool(i, 'range_end', v)} placeholder="192.168.1.200" mono /></Field>
              <Field label="Lease Time" hint="Override subnet default">
                <TextInput value={p.lease_time || ''} onChange={v => updatePool(i, 'lease_time', v)} placeholder="" mono />
              </Field>
              <Field label="Match Circuit ID" hint="Option 82 circuit-id glob">
                <TextInput value={p.match_circuit_id || ''} onChange={v => updatePool(i, 'match_circuit_id', v)} placeholder="*" mono />
              </Field>
              <Field label="Match Remote ID" hint="Option 82 remote-id glob">
                <TextInput value={p.match_remote_id || ''} onChange={v => updatePool(i, 'match_remote_id', v)} placeholder="" mono />
              </Field>
              <Field label="Match Vendor Class" hint="Option 60 glob">
                <TextInput value={p.match_vendor_class || ''} onChange={v => updatePool(i, 'match_vendor_class', v)} placeholder="" mono />
              </Field>
              <Field label="Match User Class" hint="Option 77 glob">
                <TextInput value={p.match_user_class || ''} onChange={v => updatePool(i, 'match_user_class', v)} placeholder="" mono />
              </Field>
            </FieldGrid>
          </div>
        ))}
        <button onClick={addPool} className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover"><Plus className="w-3 h-3" /> Add Pool</button>
      </Section>

      <Section title={`Custom DHCP Options (${(s.option || []).length})`}>
        {(s.option || []).map((opt, i) => (
          <div key={i} className="flex items-end gap-2 mb-1">
            <Field label="Code"><NumberInput value={opt.code} onChange={v => {
              const opts = [...(s.option || [])]; opts[i] = { ...opts[i], code: v }; setS({ ...s, option: opts })
            }} min={1} /></Field>
            <Field label="Type">
              <Select value={opt.type || 'string'} onChange={v => {
                const opts = [...(s.option || [])]; opts[i] = { ...opts[i], type: v }; setS({ ...s, option: opts })
              }} options={[
                { value: 'ip', label: 'IP' }, { value: 'ip_list', label: 'IP List' }, { value: 'string', label: 'String' },
                { value: 'uint8', label: 'UInt8' }, { value: 'uint16', label: 'UInt16' }, { value: 'uint32', label: 'UInt32' },
                { value: 'bool', label: 'Bool' }, { value: 'bytes', label: 'Bytes (hex)' },
              ]} />
            </Field>
            <Field label="Value"><TextInput value={String(opt.value ?? '')} onChange={v => {
              const opts = [...(s.option || [])]; opts[i] = { ...opts[i], value: v }; setS({ ...s, option: opts })
            }} mono /></Field>
            <button onClick={() => setS({ ...s, option: (s.option || []).filter((_, idx) => idx !== i) })}
              className="p-2 mb-0.5 text-text-muted hover:text-danger"><Trash2 className="w-4 h-4" /></button>
          </div>
        ))}
        <button onClick={() => setS({ ...s, option: [...(s.option || []), { code: 0, type: 'string', value: '' }] })}
          className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover"><Plus className="w-3 h-3" /> Add Option</button>
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
        <div className="p-3 bg-surface-overlay/30 rounded-lg space-y-2">
          <FieldGrid>
            <Field label="MAC"><TextInput value={newRes.mac} onChange={v => setNewRes({ ...newRes, mac: v })} placeholder="00:11:22:33:44:55" mono /></Field>
            <Field label="IP"><TextInput value={newRes.ip} onChange={v => setNewRes({ ...newRes, ip: v })} placeholder="192.168.1.100" mono /></Field>
            <Field label="Hostname"><TextInput value={newRes.hostname || ''} onChange={v => setNewRes({ ...newRes, hostname: v })} placeholder="server1" /></Field>
            <Field label="DDNS Hostname" hint="Override FQDN for DNS registration">
              <TextInput value={newRes.ddns_hostname || ''} onChange={v => setNewRes({ ...newRes, ddns_hostname: v })} placeholder="server1.example.com" mono />
            </Field>
          </FieldGrid>
          <Field label="DNS Servers" hint="Per-reservation override (optional)">
            <StringArrayInput value={newRes.dns_servers || []} onChange={v => setNewRes({ ...newRes, dns_servers: v })} placeholder="8.8.8.8" mono />
          </Field>
          <div className="flex gap-2 pt-1">
            <button onClick={handleAdd} className="px-3 py-2 text-xs font-medium rounded bg-accent text-white hover:bg-accent-hover">Save</button>
            <button onClick={() => setShowAdd(false)} className="px-3 py-2 text-xs rounded border border-border hover:bg-surface-overlay">Cancel</button>
          </div>
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
      <Field label="Probe Log Level">
        <Select value={current.probe_log_level || 'debug'} onChange={v => setC({ ...current, probe_log_level: v })}
          options={[{ value: 'debug', label: 'Debug' }, { value: 'info', label: 'Info' }, { value: 'warn', label: 'Warn' }]} />
      </Field>
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

const ALL_EVENTS = [
  { group: 'Lease', events: ['lease.discover', 'lease.offer', 'lease.ack', 'lease.renew', 'lease.nak', 'lease.release', 'lease.decline', 'lease.expire'] },
  { group: 'Conflict', events: ['conflict.detected', 'conflict.decline', 'conflict.resolved', 'conflict.permanent'] },
  { group: 'HA', events: ['ha.failover', 'ha.sync_complete'] },
  { group: 'Rogue', events: ['rogue.detected', 'rogue.resolved'] },
  { group: 'Anomaly', events: ['anomaly.detected'] },
]

function EventSelector({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const toggle = (evt: string) => {
    if (value.includes(evt)) onChange(value.filter(e => e !== evt))
    else onChange([...value, evt])
  }
  const selectAll = () => onChange(ALL_EVENTS.flatMap(g => g.events))
  const selectNone = () => onChange([])

  return (
    <div className="space-y-2">
      <div className="flex gap-2 text-[10px]">
        <button type="button" onClick={selectAll} className="text-accent hover:underline">Select All</button>
        <button type="button" onClick={selectNone} className="text-text-muted hover:underline">Clear</button>
        <span className="text-text-muted">{value.length} selected</span>
      </div>
      {ALL_EVENTS.map(group => (
        <div key={group.group}>
          <div className="text-[10px] font-medium text-text-muted uppercase tracking-wide mb-0.5">{group.group}</div>
          <div className="flex flex-wrap gap-1.5">
            {group.events.map(evt => (
              <label key={evt}
                className={`flex items-center gap-1 px-2 py-0.5 rounded text-[11px] cursor-pointer border transition-colors ${
                  value.includes(evt)
                    ? 'bg-accent/15 border-accent/40 text-accent'
                    : 'bg-surface border-border text-text-muted hover:border-text-muted'
                }`}>
                <input type="checkbox" checked={value.includes(evt)} onChange={() => toggle(evt)} className="sr-only" />
                {evt}
              </label>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

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
        {(current.script || []).map((s, i) => {
          const updateScript = (patch: Record<string, unknown>) => {
            const scripts = [...(current.script || [])]; scripts[i] = { ...scripts[i], ...patch }; setH({ ...current, script: scripts })
          }
          return (
            <div key={i} className="p-3 bg-surface-overlay/30 rounded-lg space-y-2 mb-2">
              <div className="flex justify-between items-center">
                <span className="text-sm font-medium">{s.name || `Script ${i + 1}`}</span>
                <button onClick={() => setH({ ...current, script: (current.script || []).filter((_, idx) => idx !== i) })}
                  className="p-1 text-text-muted hover:text-danger"><Trash2 className="w-3.5 h-3.5" /></button>
              </div>
              <FieldGrid>
                <Field label="Name"><TextInput value={s.name} onChange={v => updateScript({ name: v })} /></Field>
                <Field label="Command"><TextInput value={s.command} onChange={v => updateScript({ command: v })} mono /></Field>
                <Field label="Timeout"><TextInput value={s.timeout || ''} onChange={v => updateScript({ timeout: v })} placeholder="10s" mono /></Field>
              </FieldGrid>
              <Field label="Events" hint="Select which events trigger this hook">
                <EventSelector value={s.events || []} onChange={v => updateScript({ events: v })} />
              </Field>
              <Field label="Subnet Filter" hint="Only fire for these subnets (empty = all)">
                <StringArrayInput value={s.subnets || []} onChange={v => updateScript({ subnets: v })} placeholder="192.168.1.0/24" mono />
              </Field>
            </div>
          )
        })}
        <button onClick={() => setH({ ...current, script: [...(current.script || []), { name: '', events: [], command: '', timeout: '10s', subnets: [] }] })}
          className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover"><Plus className="w-3 h-3" /> Add Script Hook</button>
      </Section>

      <Section title={`Webhooks (${current.webhook?.length || 0})`}>
        {(current.webhook || []).map((wh, i) => {
          const updateWH = (patch: Record<string, unknown>) => {
            const hooks = [...(current.webhook || [])]; hooks[i] = { ...hooks[i], ...patch }; setH({ ...current, webhook: hooks })
          }
          return (
            <div key={i} className="p-3 bg-surface-overlay/30 rounded-lg space-y-2 mb-2">
              <div className="flex justify-between items-center">
                <span className="text-sm font-medium">{wh.name || `Webhook ${i + 1}`}</span>
                <button onClick={() => setH({ ...current, webhook: (current.webhook || []).filter((_, idx) => idx !== i) })}
                  className="p-1 text-text-muted hover:text-danger"><Trash2 className="w-3.5 h-3.5" /></button>
              </div>
              <FieldGrid>
                <Field label="Name"><TextInput value={wh.name} onChange={v => updateWH({ name: v })} /></Field>
                <Field label="URL"><TextInput value={wh.url} onChange={v => updateWH({ url: v })} mono /></Field>
                <Field label="Method">
                  <Select value={wh.method || 'POST'} onChange={v => updateWH({ method: v })}
                    options={[{ value: 'POST', label: 'POST' }, { value: 'PUT', label: 'PUT' }, { value: 'PATCH', label: 'PATCH' }]} />
                </Field>
                <Field label="Timeout"><TextInput value={wh.timeout || ''} onChange={v => updateWH({ timeout: v })} placeholder="10s" mono /></Field>
                <Field label="Retries"><NumberInput value={wh.retries} onChange={v => updateWH({ retries: v })} min={0} /></Field>
                <Field label="Retry Backoff"><TextInput value={wh.retry_backoff || ''} onChange={v => updateWH({ retry_backoff: v })} placeholder="2s" mono /></Field>
                <Field label="HMAC Secret" hint="For X-Athena-Signature header">
                  <TextInput value={wh.secret || ''} onChange={v => updateWH({ secret: v })} placeholder="optional" />
                </Field>
                <Field label="Template" hint="slack, teams, or empty for raw JSON">
                  <TextInput value={wh.template || ''} onChange={v => updateWH({ template: v })} placeholder="" />
                </Field>
              </FieldGrid>
              <Field label="Events" hint="Select which events trigger this webhook">
                <EventSelector value={wh.events || []} onChange={v => updateWH({ events: v })} />
              </Field>
              <Field label="Custom Headers" hint="key: value, one per line">
                <textarea
                  value={Object.entries(wh.headers || {}).map(([k, v]) => `${k}: ${v}`).join('\n')}
                  onChange={e => {
                    const h: Record<string, string> = {}
                    e.target.value.split('\n').forEach(line => {
                      const idx = line.indexOf(':')
                      if (idx > 0) h[line.slice(0, idx).trim()] = line.slice(idx + 1).trim()
                    })
                    updateWH({ headers: h })
                  }}
                  rows={2}
                  placeholder="X-Custom-Header: value"
                  className="w-full px-3 py-2 text-xs font-mono rounded-lg border border-border bg-surface focus:outline-none focus:border-accent"
                />
              </Field>
            </div>
          )
        })}
        <button onClick={() => setH({ ...current, webhook: [...(current.webhook || []), { name: '', events: [], url: '', method: 'POST', timeout: '10s', retries: 3, retry_backoff: '2s', secret: '', template: '', headers: {} }] })}
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

const emptyZone: DDNSZoneType = { zone: '', method: 'rfc2136', server: '', tsig_name: '', tsig_algorithm: 'hmac-sha256', tsig_secret: '', api_key: '' }

function DDNSZoneEditor({ label, value, onChange }: { label: string; value: DDNSZoneType; onChange: (v: DDNSZoneType) => void }) {
  const set = <K extends keyof DDNSZoneType>(k: K, v: DDNSZoneType[K]) => onChange({ ...value, [k]: v })
  const isApi = value.method === 'powerdns_api' || value.method === 'technitium_api'
  return (
    <div className="border border-border/50 rounded-lg p-4 space-y-3 bg-surface/50">
      <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">{label}</h4>
      <FieldGrid>
        <Field label="Zone" hint="with trailing dot">
          <TextInput value={value.zone} onChange={v => set('zone', v)} placeholder="example.com." mono />
        </Field>
        <Field label="Method">
          <Select value={value.method} onChange={v => set('method', v)} options={[
            { value: 'rfc2136', label: 'RFC 2136 (BIND/Knot/Windows/CoreDNS)' },
            { value: 'powerdns_api', label: 'PowerDNS API' },
            { value: 'technitium_api', label: 'Technitium API' },
          ]} />
        </Field>
        <Field label="Server" hint={isApi ? 'http://host:port' : 'host:53'}>
          <TextInput value={value.server} onChange={v => set('server', v)} placeholder={isApi ? 'http://dns:8081' : 'ns1.example.com:53'} mono />
        </Field>
        {isApi ? (
          <Field label="API Key">
            <TextInput value={value.api_key} onChange={v => set('api_key', v)} placeholder="api-key" />
          </Field>
        ) : (
          <>
            <Field label="TSIG Key Name">
              <TextInput value={value.tsig_name} onChange={v => set('tsig_name', v)} placeholder="dhcp-update." mono />
            </Field>
            <Field label="TSIG Algorithm">
              <Select value={value.tsig_algorithm} onChange={v => set('tsig_algorithm', v)} options={[
                { value: 'hmac-md5', label: 'HMAC-MD5' },
                { value: 'hmac-sha1', label: 'HMAC-SHA1' },
                { value: 'hmac-sha256', label: 'HMAC-SHA256' },
                { value: 'hmac-sha512', label: 'HMAC-SHA512' },
              ]} />
            </Field>
            <Field label="TSIG Secret" hint="base64">
              <TextInput value={value.tsig_secret} onChange={v => set('tsig_secret', v)} placeholder="base64-encoded-secret" />
            </Field>
          </>
        )}
      </FieldGrid>
    </div>
  )
}

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

      <DDNSZoneEditor label="Forward Zone (A Records)" value={current.forward || emptyZone} onChange={v => setD({ ...current, forward: v })} />
      <DDNSZoneEditor label="Reverse Zone (PTR Records)" value={current.reverse || emptyZone} onChange={v => setD({ ...current, reverse: v })} />

      {/* Zone Overrides */}
      <Section title={`Per-Subnet Zone Overrides (${(current.zone_override || []).length})`}>
        {(current.zone_override || []).map((o, i) => {
          const update = (patch: Partial<typeof o>) => {
            const n = [...(current.zone_override || [])]; n[i] = { ...o, ...patch }; setD({ ...current, zone_override: n })
          }
          const isApi = o.method === 'powerdns_api' || o.method === 'technitium_api'
          return (
            <div key={i} className="border border-border/50 rounded-lg p-4 space-y-3 bg-surface/50 mb-3">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-warning">{o.subnet || 'New Override'}</span>
                <button type="button" onClick={() => setD({ ...current, zone_override: (current.zone_override || []).filter((_, idx) => idx !== i) })}
                  className="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
              <FieldGrid>
                <Field label="Subnet"><TextInput value={o.subnet || ''} onChange={v => update({ subnet: v })} placeholder="10.0.0.0/24" mono /></Field>
                <Field label="Forward Zone"><TextInput value={o.forward_zone || ''} onChange={v => update({ forward_zone: v })} placeholder="lab.example.com." mono /></Field>
                <Field label="Reverse Zone"><TextInput value={o.reverse_zone || ''} onChange={v => update({ reverse_zone: v })} placeholder="0.0.10.in-addr.arpa." mono /></Field>
                <Field label="Method">
                  <Select value={o.method || 'rfc2136'} onChange={v => update({ method: v })}
                    options={[{ value: 'rfc2136', label: 'RFC 2136' }, { value: 'powerdns_api', label: 'PowerDNS API' }, { value: 'technitium_api', label: 'Technitium API' }]} />
                </Field>
                <Field label="Server"><TextInput value={o.server || ''} onChange={v => update({ server: v })} mono /></Field>
                {isApi ? (
                  <Field label="API Key"><TextInput value={o.api_key || ''} onChange={v => update({ api_key: v })} placeholder="api-key" /></Field>
                ) : (
                  <>
                    <Field label="TSIG Key Name"><TextInput value={o.tsig_name || ''} onChange={v => update({ tsig_name: v })} placeholder="dhcp-update." mono /></Field>
                    <Field label="TSIG Algorithm">
                      <Select value={o.tsig_algorithm || 'hmac-sha256'} onChange={v => update({ tsig_algorithm: v })}
                        options={[{ value: 'hmac-md5', label: 'HMAC-MD5' }, { value: 'hmac-sha1', label: 'HMAC-SHA1' }, { value: 'hmac-sha256', label: 'HMAC-SHA256' }, { value: 'hmac-sha512', label: 'HMAC-SHA512' }]} />
                    </Field>
                    <Field label="TSIG Secret" hint="base64"><TextInput value={o.tsig_secret || ''} onChange={v => update({ tsig_secret: v })} placeholder="base64-encoded-secret" /></Field>
                  </>
                )}
              </FieldGrid>
            </div>
          )
        })}
        {(current.zone_override || []).length === 0 && <p className="text-xs text-text-muted italic">No zone overrides — all subnets use the main forward/reverse zones</p>}
        <button type="button" onClick={() => setD({ ...current, zone_override: [...(current.zone_override || []), { subnet: '', forward_zone: '', reverse_zone: '', method: 'rfc2136', server: '', api_key: '', tsig_name: '', tsig_algorithm: 'hmac-sha256', tsig_secret: '' }] })}
          className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors mt-2">
          <Plus className="w-3 h-3" /> Add Override
        </button>
      </Section>

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
      <Field label="Listen DoH" hint="DNS-over-HTTPS listen address (leave empty to disable)">
        <TextInput value={current.listen_doh || ''} onChange={v => setD({ ...current, listen_doh: v })} placeholder="0.0.0.0:443" mono />
      </Field>

      {/* Zone Overrides */}
      <Section title={`Zone Overrides (${(current.zone_override || []).length})`}>
        <p className="text-xs text-text-muted mb-2">Route queries for specific domains to dedicated nameservers (split-horizon DNS)</p>
        {(current.zone_override || []).map((zo, i) => {
          const updateZO = (patch: Record<string, unknown>) => {
            const n = [...(current.zone_override || [])]; n[i] = { ...n[i], ...patch }; setD({ ...current, zone_override: n })
          }
          return (
            <div key={i} className="p-3 bg-surface-overlay/30 rounded-lg space-y-2 mb-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-text-muted">{zo.zone || 'New Override'}</span>
                <button onClick={() => setD({ ...current, zone_override: (current.zone_override || []).filter((_, idx) => idx !== i) })}
                  className="p-1 text-text-muted hover:text-danger"><Trash2 className="w-3.5 h-3.5" /></button>
              </div>
              <FieldGrid>
                <Field label="Zone"><TextInput value={zo.zone || ''} onChange={v => updateZO({ zone: v })} placeholder="corp.internal." mono /></Field>
                <Field label="Nameserver"><TextInput value={zo.nameserver || ''} onChange={v => updateZO({ nameserver: v })} placeholder="10.0.0.1:53" mono /></Field>
              </FieldGrid>
            </div>
          )
        })}
        <button onClick={() => setD({ ...current, zone_override: [...(current.zone_override || []), { zone: '', nameserver: '', doh: false, doh_url: '' }] })}
          className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover"><Plus className="w-3 h-3" /> Add Zone Override</button>
      </Section>

      {/* Static Records */}
      <Section title={`Static Records (${(current.record || []).length})`}>
        <p className="text-xs text-text-muted mb-2">Manual DNS records for hosts not managed by DHCP</p>
        {(current.record || []).map((rec, i) => {
          const updateRec = (patch: Record<string, unknown>) => {
            const n = [...(current.record || [])]; n[i] = { ...n[i], ...patch }; setD({ ...current, record: n })
          }
          return (
            <div key={i} className="p-3 bg-surface-overlay/30 rounded-lg space-y-2 mb-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-mono text-text-muted">{rec.name || 'new'} {rec.type || 'A'} {rec.value || ''}</span>
                <button onClick={() => setD({ ...current, record: (current.record || []).filter((_, idx) => idx !== i) })}
                  className="p-1 text-text-muted hover:text-danger"><Trash2 className="w-3.5 h-3.5" /></button>
              </div>
              <FieldGrid>
                <Field label="Name"><TextInput value={rec.name || ''} onChange={v => updateRec({ name: v })} placeholder="server1.local" mono /></Field>
                <Field label="Type">
                  <Select value={rec.type || 'A'} onChange={v => updateRec({ type: v })}
                    options={[{ value: 'A', label: 'A' }, { value: 'AAAA', label: 'AAAA' }, { value: 'CNAME', label: 'CNAME' }, { value: 'MX', label: 'MX' }, { value: 'TXT', label: 'TXT' }, { value: 'SRV', label: 'SRV' }]} />
                </Field>
                <Field label="Value"><TextInput value={rec.value || ''} onChange={v => updateRec({ value: v })} placeholder="192.168.1.100" mono /></Field>
                <Field label="TTL"><NumberInput value={rec.ttl} onChange={v => updateRec({ ttl: v })} min={0} /></Field>
              </FieldGrid>
            </div>
          )
        })}
        <button onClick={() => setD({ ...current, record: [...(current.record || []), { name: '', type: 'A', value: '', ttl: 300 }] })}
          className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover"><Plus className="w-3 h-3" /> Add Static Record</button>
      </Section>

      {/* Filter Lists */}
      <Section title={`Filter Lists (${(current.list || []).length})`}>
        <p className="text-xs text-text-muted mb-2">Block or allow domains using external filter lists (hosts files, domain lists, adblock format)</p>
        {(current.list || []).map((lst, i) => {
          const updateLst = (patch: Record<string, unknown>) => {
            const n = [...(current.list || [])]; n[i] = { ...n[i], ...patch }; setD({ ...current, list: n })
          }
          return (
            <div key={i} className="p-3 bg-surface-overlay/30 rounded-lg space-y-2 mb-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-text-muted">{lst.name || 'New List'}</span>
                <div className="flex items-center gap-2">
                  <label className="flex items-center gap-1 text-xs cursor-pointer">
                    <input type="checkbox" checked={lst.enabled !== false} onChange={e => updateLst({ enabled: e.target.checked })}
                      className="rounded border-border accent-accent" />
                    Enabled
                  </label>
                  <button onClick={() => setD({ ...current, list: (current.list || []).filter((_, idx) => idx !== i) })}
                    className="p-1 text-text-muted hover:text-danger"><Trash2 className="w-3.5 h-3.5" /></button>
                </div>
              </div>
              <FieldGrid>
                <Field label="Name"><TextInput value={lst.name || ''} onChange={v => updateLst({ name: v })} placeholder="steven-black" /></Field>
                <Field label="URL"><TextInput value={lst.url || ''} onChange={v => updateLst({ url: v })} placeholder="https://..." mono /></Field>
                <Field label="Type">
                  <Select value={lst.type || 'block'} onChange={v => updateLst({ type: v })}
                    options={[{ value: 'block', label: 'Block' }, { value: 'allow', label: 'Allow' }]} />
                </Field>
                <Field label="Format">
                  <Select value={lst.format || 'hosts'} onChange={v => updateLst({ format: v })}
                    options={[{ value: 'hosts', label: 'Hosts file' }, { value: 'domains', label: 'Domain list' }, { value: 'adblock', label: 'Adblock' }]} />
                </Field>
                <Field label="Action">
                  <Select value={lst.action || 'nxdomain'} onChange={v => updateLst({ action: v })}
                    options={[{ value: 'nxdomain', label: 'NXDOMAIN' }, { value: 'zero', label: '0.0.0.0' }, { value: 'refuse', label: 'REFUSED' }]} />
                </Field>
                <Field label="Refresh Interval"><TextInput value={lst.refresh_interval || ''} onChange={v => updateLst({ refresh_interval: v })} placeholder="24h" mono /></Field>
              </FieldGrid>
            </div>
          )
        })}
        <button onClick={() => setD({ ...current, list: [...(current.list || []), { name: '', url: '', type: 'block', format: 'hosts', action: 'nxdomain', enabled: true, refresh_interval: '24h' }] })}
          className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover"><Plus className="w-3 h-3" /> Add Filter List</button>
      </Section>

      {/* DoH TLS */}
      {current.listen_doh && (
        <Section title="DoH TLS Settings">
          <FieldGrid>
            <Field label="TLS Certificate"><TextInput value={current.doh_tls?.cert_file || ''} onChange={v => setD({ ...current, doh_tls: { ...(current.doh_tls || {}), cert_file: v } })} placeholder="/path/to/cert.pem" mono /></Field>
            <Field label="TLS Key"><TextInput value={current.doh_tls?.key_file || ''} onChange={v => setD({ ...current, doh_tls: { ...(current.doh_tls || {}), key_file: v } })} placeholder="/path/to/key.pem" mono /></Field>
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

// ============== HOSTNAME SANITISATION TAB ==============

function HostnameSanitisationTab({ onStatus }: { onStatus: StatusFn }) {
  const { data } = useApi(useCallback(() => v2GetHostnameSanitisation(), []))
  const [current, setC] = useState<HostnameSanitisationConfig | null>(null)

  if (data && !current) setC(data)
  if (!current) return <Card className="p-6 text-sm text-text-muted">Loading...</Card>

  const handleSave = async () => {
    try {
      await v2SetHostnameSanitisation(current)
      onStatus('success', 'Hostname sanitisation config saved')
    } catch (e) {
      onStatus('error', e instanceof Error ? e.message : 'Save failed')
    }
  }

  return (
    <Card className="p-5 space-y-4">
      <div>
        <h3 className="text-sm font-semibold">Hostname Sanitisation Pipeline</h3>
        <p className="text-xs text-text-muted mt-1">
          Clean client-supplied hostnames (option 12) before DNS registration. Strips invalid characters,
          rejects known-bad patterns (localhost, android-*, etc.), deduplicates, and enforces naming policies.
        </p>
      </div>

      <Toggle checked={current.enabled} onChange={v => setC({ ...current, enabled: v })} label="Enabled" description="Enable the full sanitisation pipeline (basic cleanup always runs)" />

      {current.enabled && (
        <div className="space-y-4">
          <FieldGrid>
            <Toggle checked={current.lowercase} onChange={v => setC({ ...current, lowercase: v })} label="Force Lowercase" description="Convert all hostnames to lowercase" />
            <Toggle checked={current.strip_emoji} onChange={v => setC({ ...current, strip_emoji: v })} label="Strip Emoji" description="Remove emoji and symbol characters" />
            <Toggle checked={current.dedup_suffix} onChange={v => setC({ ...current, dedup_suffix: v })} label="Deduplicate" description="Append -2, -3, etc. for duplicate hostnames" />
          </FieldGrid>

          <FieldGrid>
            <Field label="Max Length" hint="Maximum hostname length (DNS label limit = 63)">
              <NumberInput value={current.max_length || 63} onChange={v => setC({ ...current, max_length: v })} min={1} max={253} />
            </Field>
            <Field label="Fallback Template" hint="Template when hostname is rejected. {mac} = MAC without colons">
              <TextInput value={current.fallback_template || ''} onChange={v => setC({ ...current, fallback_template: v })} placeholder="dhcp-{mac}" mono />
            </Field>
          </FieldGrid>

          <Field label="Allow Regex" hint="If set, hostnames must match this regex to be accepted (leave empty to allow all)">
            <TextInput value={current.allow_regex || ''} onChange={v => setC({ ...current, allow_regex: v })} placeholder="^[a-z]+-\d+$" mono />
          </Field>

          <Field label="Deny Patterns" hint="Regex patterns to reject (one per line, case-insensitive). Built-in patterns (localhost, android-*, etc.) always apply.">
            <textarea
              value={(current.deny_patterns || []).join('\n')}
              onChange={e => setC({ ...current, deny_patterns: e.target.value.split('\n').filter(Boolean) })}
              rows={4}
              placeholder={"^printer-.*\n^kiosk\\d+$\n^guest-"}
              className="w-full px-3 py-2 text-xs font-mono rounded-lg border border-border bg-surface focus:outline-none focus:border-accent"
            />
          </Field>
        </div>
      )}

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
