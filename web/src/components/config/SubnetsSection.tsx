import type { SubnetConfig, PoolConfig, ReservationConfig, OptionConfig } from '@/lib/configTypes'
import { Section, FieldGrid, Field, TextInput, NumberInput, Select, StringArrayInput } from '@/components/FormFields'
import { Plus, Trash2, Network, Server, BookmarkPlus } from 'lucide-react'

function emptySubnet(): SubnetConfig {
  return {
    network: '', routers: [], dns_servers: [], domain_name: '',
    lease_time: '', renewal_time: '', rebind_time: '', ntp_servers: [],
    pool: [], reservation: [], option: [],
  }
}

function emptyPool(): PoolConfig {
  return { range_start: '', range_end: '', lease_time: '', match_circuit_id: '', match_remote_id: '', match_vendor_class: '', match_user_class: '' }
}

function emptyReservation(): ReservationConfig {
  return { mac: '', identifier: '', ip: '', hostname: '', dns_servers: [], ddns_hostname: '' }
}

function emptyOption(): OptionConfig {
  return { code: 0, type: 'string', value: '' }
}

function PoolEditor({ value, onChange, onRemove, index }: {
  value: PoolConfig; onChange: (v: PoolConfig) => void; onRemove: () => void; index: number
}) {
  const set = <K extends keyof PoolConfig>(k: K, v: PoolConfig[K]) => onChange({ ...value, [k]: v })
  const hasMatch = value.match_circuit_id || value.match_remote_id || value.match_vendor_class || value.match_user_class

  return (
    <div className="border border-border/50 rounded-lg p-4 space-y-3 bg-surface/50">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Server className="w-3.5 h-3.5 text-info" />
          <span className="text-xs font-semibold text-info">
            Pool {index + 1}{value.range_start && value.range_end ? `: ${value.range_start} — ${value.range_end}` : ''}
          </span>
        </div>
        <button type="button" onClick={onRemove} className="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
          <Trash2 className="w-3.5 h-3.5" />
        </button>
      </div>
      <FieldGrid>
        <Field label="Range Start"><TextInput value={value.range_start} onChange={v => set('range_start', v)} placeholder="192.168.1.100" mono /></Field>
        <Field label="Range End"><TextInput value={value.range_end} onChange={v => set('range_end', v)} placeholder="192.168.1.200" mono /></Field>
        <Field label="Lease Time" hint="override, empty = subnet default">
          <TextInput value={value.lease_time} onChange={v => set('lease_time', v)} placeholder="" mono />
        </Field>
      </FieldGrid>

      <details className={hasMatch ? 'open' : ''}>
        <summary className="text-xs text-text-muted cursor-pointer hover:text-text-secondary transition-colors">
          Match criteria {hasMatch ? '(active)' : '(none — pool serves all clients)'}
        </summary>
        <div className="mt-2">
          <FieldGrid>
            <Field label="Match Circuit ID" hint="glob pattern, Option 82">
              <TextInput value={value.match_circuit_id} onChange={v => set('match_circuit_id', v)} placeholder="eth0/1/*" mono />
            </Field>
            <Field label="Match Remote ID" hint="glob pattern, Option 82">
              <TextInput value={value.match_remote_id} onChange={v => set('match_remote_id', v)} placeholder="switch01" mono />
            </Field>
            <Field label="Match Vendor Class" hint="glob pattern, Option 60">
              <TextInput value={value.match_vendor_class} onChange={v => set('match_vendor_class', v)} mono />
            </Field>
            <Field label="Match User Class" hint="glob pattern, Option 77">
              <TextInput value={value.match_user_class} onChange={v => set('match_user_class', v)} mono />
            </Field>
          </FieldGrid>
        </div>
      </details>
    </div>
  )
}

function ReservationEditor({ value, onChange, onRemove }: {
  value: ReservationConfig; onChange: (v: ReservationConfig) => void; onRemove: () => void
}) {
  const set = <K extends keyof ReservationConfig>(k: K, v: ReservationConfig[K]) => onChange({ ...value, [k]: v })

  return (
    <div className="border border-border/50 rounded-lg p-4 space-y-3 bg-surface/50">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <BookmarkPlus className="w-3.5 h-3.5 text-success" />
          <span className="text-xs font-semibold text-success">
            {value.hostname || value.ip || value.mac || 'New Reservation'}
          </span>
        </div>
        <button type="button" onClick={onRemove} className="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
          <Trash2 className="w-3.5 h-3.5" />
        </button>
      </div>
      <FieldGrid>
        <Field label="MAC Address"><TextInput value={value.mac} onChange={v => set('mac', v)} placeholder="00:11:22:33:44:55" mono /></Field>
        <Field label="Client Identifier" hint="alternative to MAC">
          <TextInput value={value.identifier} onChange={v => set('identifier', v)} mono />
        </Field>
        <Field label="IP Address"><TextInput value={value.ip} onChange={v => set('ip', v)} placeholder="192.168.1.10" mono /></Field>
        <Field label="Hostname"><TextInput value={value.hostname} onChange={v => set('hostname', v)} placeholder="printer" /></Field>
        <Field label="DDNS Hostname" hint="override FQDN for DNS">
          <TextInput value={value.ddns_hostname} onChange={v => set('ddns_hostname', v)} placeholder="printer.office.example.com" mono />
        </Field>
      </FieldGrid>
      <Field label="DNS Servers" hint="per-reservation override">
        <StringArrayInput value={value.dns_servers || []} onChange={v => set('dns_servers', v)} placeholder="8.8.8.8" mono />
      </Field>
    </div>
  )
}

function OptionEditor({ value, onChange, onRemove }: {
  value: OptionConfig; onChange: (v: OptionConfig) => void; onRemove: () => void
}) {
  return (
    <div className="flex items-center gap-2">
      <div className="w-20">
        <NumberInput value={value.code} onChange={v => onChange({ ...value, code: v })} min={1} max={254} placeholder="Code" />
      </div>
      <div className="w-28">
        <Select value={value.type} onChange={v => onChange({ ...value, type: v })} options={[
          { value: 'ip', label: 'IP' }, { value: 'ip_list', label: 'IP List' },
          { value: 'string', label: 'String' }, { value: 'uint8', label: 'uint8' },
          { value: 'uint16', label: 'uint16' }, { value: 'uint32', label: 'uint32' },
          { value: 'bool', label: 'Boolean' }, { value: 'bytes', label: 'Bytes (hex)' },
        ]} />
      </div>
      <div className="flex-1">
        <TextInput value={String(value.value ?? '')} onChange={v => onChange({ ...value, value: v })} placeholder="value" mono />
      </div>
      <button type="button" onClick={onRemove} className="p-1.5 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
        <Trash2 className="w-3.5 h-3.5" />
      </button>
    </div>
  )
}

function SubnetEditor({ value, onChange, onRemove, index }: {
  value: SubnetConfig; onChange: (v: SubnetConfig) => void; onRemove: () => void; index: number
}) {
  const set = <K extends keyof SubnetConfig>(k: K, v: SubnetConfig[K]) => onChange({ ...value, [k]: v })
  const pools = value.pool || []
  const reservations = value.reservation || []
  const options = value.option || []

  return (
    <div className="border border-accent/20 rounded-xl overflow-hidden">
      <div className="flex items-center justify-between px-5 py-3 bg-accent/5 border-b border-accent/20">
        <div className="flex items-center gap-2">
          <Network className="w-4 h-4 text-accent" />
          <span className="text-sm font-semibold">{value.network || `Subnet ${index + 1}`}</span>
          <span className="text-xs text-text-muted ml-2">
            {pools.length} pool{pools.length !== 1 ? 's' : ''} · {reservations.length} reservation{reservations.length !== 1 ? 's' : ''}
          </span>
        </div>
        <button type="button" onClick={onRemove}
          className="flex items-center gap-1 text-xs text-text-muted hover:text-danger transition-colors">
          <Trash2 className="w-3.5 h-3.5" /> Remove Subnet
        </button>
      </div>

      <div className="p-5 space-y-5">
        {/* Basic subnet settings */}
        <FieldGrid>
          <Field label="Network" hint="CIDR notation">
            <TextInput value={value.network} onChange={v => set('network', v)} placeholder="192.168.1.0/24" mono />
          </Field>
          <Field label="Domain Name">
            <TextInput value={value.domain_name} onChange={v => set('domain_name', v)} placeholder="office.example.com" mono />
          </Field>
          <Field label="Lease Time" hint="override, empty = global default">
            <TextInput value={value.lease_time} onChange={v => set('lease_time', v)} placeholder="" mono />
          </Field>
          <Field label="Renewal Time (T1)">
            <TextInput value={value.renewal_time} onChange={v => set('renewal_time', v)} placeholder="" mono />
          </Field>
          <Field label="Rebind Time (T2)">
            <TextInput value={value.rebind_time} onChange={v => set('rebind_time', v)} placeholder="" mono />
          </Field>
        </FieldGrid>

        <Field label="Routers" hint="default gateways — option 3">
          <StringArrayInput value={value.routers || []} onChange={v => set('routers', v)} placeholder="192.168.1.1" mono />
        </Field>
        <Field label="DNS Servers" hint="option 6, overrides global defaults">
          <StringArrayInput value={value.dns_servers || []} onChange={v => set('dns_servers', v)} placeholder="192.168.1.1" mono />
        </Field>
        <Field label="NTP Servers" hint="option 42">
          <StringArrayInput value={value.ntp_servers || []} onChange={v => set('ntp_servers', v)} placeholder="192.168.1.1" mono />
        </Field>

        {/* Pools */}
        <div className="pt-3 border-t border-border/50">
          <div className="flex items-center justify-between mb-3">
            <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Address Pools ({pools.length})</h4>
            <button type="button" onClick={() => set('pool', [...pools, emptyPool()])}
              className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors">
              <Plus className="w-3 h-3" /> Add Pool
            </button>
          </div>
          <div className="space-y-3">
            {pools.map((p, i) => (
              <PoolEditor key={i} value={p} index={i}
                onChange={v => { const n = [...pools]; n[i] = v; set('pool', n) }}
                onRemove={() => set('pool', pools.filter((_, idx) => idx !== i))} />
            ))}
            {pools.length === 0 && <p className="text-xs text-text-muted italic">No pools — this subnet can only serve reservations</p>}
          </div>
        </div>

        {/* Reservations */}
        <div className="pt-3 border-t border-border/50">
          <div className="flex items-center justify-between mb-3">
            <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Reservations ({reservations.length})</h4>
            <button type="button" onClick={() => set('reservation', [...reservations, emptyReservation()])}
              className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors">
              <Plus className="w-3 h-3" /> Add Reservation
            </button>
          </div>
          <div className="space-y-3">
            {reservations.map((r, i) => (
              <ReservationEditor key={i} value={r}
                onChange={v => { const n = [...reservations]; n[i] = v; set('reservation', n) }}
                onRemove={() => set('reservation', reservations.filter((_, idx) => idx !== i))} />
            ))}
            {reservations.length === 0 && <p className="text-xs text-text-muted italic">No static reservations</p>}
          </div>
        </div>

        {/* Custom options */}
        <div className="pt-3 border-t border-border/50">
          <div className="flex items-center justify-between mb-3">
            <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Custom DHCP Options ({options.length})</h4>
            <button type="button" onClick={() => set('option', [...options, emptyOption()])}
              className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors">
              <Plus className="w-3 h-3" /> Add Option
            </button>
          </div>
          <div className="space-y-2">
            {options.length > 0 && (
              <div className="flex items-center gap-2 text-[10px] text-text-muted uppercase tracking-wider font-medium px-1">
                <div className="w-20">Code</div>
                <div className="w-28">Type</div>
                <div className="flex-1">Value</div>
                <div className="w-8"></div>
              </div>
            )}
            {options.map((o, i) => (
              <OptionEditor key={i} value={o}
                onChange={v => { const n = [...options]; n[i] = v; set('option', n) }}
                onRemove={() => set('option', options.filter((_, idx) => idx !== i))} />
            ))}
            {options.length === 0 && <p className="text-xs text-text-muted italic">No custom options — built-in fields (routers, DNS, etc.) are set above</p>}
          </div>
        </div>
      </div>
    </div>
  )
}

export default function SubnetsSection({ value, onChange }: {
  value: SubnetConfig[]
  onChange: (v: SubnetConfig[]) => void
}) {
  return (
    <Section title={`Subnets (${value.length})`} description="Network scopes with pools, reservations, and options">
      <div className="space-y-4">
        {value.map((s, i) => (
          <SubnetEditor key={i} value={s} index={i}
            onChange={v => { const n = [...value]; n[i] = v; onChange(n) }}
            onRemove={() => onChange(value.filter((_, idx) => idx !== i))} />
        ))}
      </div>
      <button type="button" onClick={() => onChange([...value, emptySubnet()])}
        className="flex items-center gap-2 w-full justify-center py-3 rounded-xl border-2 border-dashed border-border hover:border-accent/50 text-text-muted hover:text-accent transition-colors text-sm">
        <Plus className="w-4 h-4" /> Add Subnet
      </button>
    </Section>
  )
}
