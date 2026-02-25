import type { DNSProxyConfig, DNSListConfig, DNSZoneOverrideConfig, DNSStaticRecordConfig } from '@/lib/configTypes'
import { Section, FieldGrid, Field, TextInput, NumberInput, Toggle, Select, StringArrayInput } from '@/components/FormFields'
import { Plus, Trash2 } from 'lucide-react'

function emptyList(): DNSListConfig {
  return { name: '', url: '', type: 'block', format: 'hosts', action: 'nxdomain', enabled: true, refresh_interval: '24h' }
}

function emptyZoneOverride(): DNSZoneOverrideConfig {
  return { zone: '', nameserver: '', doh: false, doh_url: '' }
}

function emptyStaticRecord(): DNSStaticRecordConfig {
  return { name: '', type: 'A', value: '', ttl: 0 }
}

function ListEditor({ value, onChange, onRemove }: {
  value: DNSListConfig
  onChange: (v: DNSListConfig) => void
  onRemove: () => void
}) {
  const set = <K extends keyof DNSListConfig>(k: K, v: DNSListConfig[K]) => onChange({ ...value, [k]: v })
  return (
    <div className="border border-border/50 rounded-lg p-4 space-y-3 bg-surface/50">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className={`text-xs font-semibold ${value.type === 'block' ? 'text-danger' : 'text-success'}`}>
            {value.name || 'New List'}
          </span>
          <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${
            value.type === 'block'
              ? 'bg-danger/10 text-danger'
              : 'bg-success/10 text-success'
          }`}>{value.type}</span>
          {!value.enabled && (
            <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-border/50 text-text-muted font-medium">disabled</span>
          )}
        </div>
        <button type="button" onClick={onRemove} className="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
          <Trash2 className="w-3.5 h-3.5" />
        </button>
      </div>
      <Toggle checked={value.enabled} onChange={v => set('enabled', v)} label="Enabled" />
      <FieldGrid>
        <Field label="Name"><TextInput value={value.name} onChange={v => set('name', v)} placeholder="steven-black-hosts" /></Field>
        <Field label="URL"><TextInput value={value.url} onChange={v => set('url', v)} placeholder="https://raw.githubusercontent.com/..." mono /></Field>
        <Field label="Type">
          <Select value={value.type} onChange={v => set('type', v)} options={[
            { value: 'block', label: 'Blocklist' },
            { value: 'allow', label: 'Allowlist' },
          ]} />
        </Field>
        <Field label="Format" hint="how domains are listed">
          <Select value={value.format} onChange={v => set('format', v)} options={[
            { value: 'hosts', label: 'Hosts file (0.0.0.0 domain)' },
            { value: 'domains', label: 'Domain list (one per line)' },
            { value: 'adblock', label: 'Adblock (||domain^)' },
          ]} />
        </Field>
        <Field label="Action" hint="what to return for blocked queries">
          <Select value={value.action} onChange={v => set('action', v)} options={[
            { value: 'nxdomain', label: 'NXDOMAIN (domain not found)' },
            { value: 'zero', label: '0.0.0.0 / :: (null IP)' },
            { value: 'refuse', label: 'REFUSED' },
          ]} />
        </Field>
        <Field label="Refresh Interval" hint="how often to re-download">
          <TextInput value={value.refresh_interval} onChange={v => set('refresh_interval', v)} placeholder="24h" mono />
        </Field>
      </FieldGrid>
    </div>
  )
}

function ZoneOverrideEditor({ value, onChange, onRemove }: {
  value: DNSZoneOverrideConfig
  onChange: (v: DNSZoneOverrideConfig) => void
  onRemove: () => void
}) {
  const set = <K extends keyof DNSZoneOverrideConfig>(k: K, v: DNSZoneOverrideConfig[K]) => onChange({ ...value, [k]: v })
  return (
    <div className="border border-border/50 rounded-lg p-4 space-y-3 bg-surface/50">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-info">{value.zone || 'New Override'}</span>
        <button type="button" onClick={onRemove} className="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
          <Trash2 className="w-3.5 h-3.5" />
        </button>
      </div>
      <FieldGrid>
        <Field label="Zone"><TextInput value={value.zone} onChange={v => set('zone', v)} placeholder="internal.corp" mono /></Field>
        <Field label="Nameserver"><TextInput value={value.nameserver} onChange={v => set('nameserver', v)} placeholder="10.0.0.53" mono /></Field>
      </FieldGrid>
      <Toggle checked={value.doh} onChange={v => set('doh', v)} label="Use DNS-over-HTTPS" />
      {value.doh && (
        <Field label="DoH URL"><TextInput value={value.doh_url} onChange={v => set('doh_url', v)} placeholder="https://dns.example.com/dns-query" mono /></Field>
      )}
    </div>
  )
}

function StaticRecordEditor({ value, onChange, onRemove }: {
  value: DNSStaticRecordConfig
  onChange: (v: DNSStaticRecordConfig) => void
  onRemove: () => void
}) {
  const set = <K extends keyof DNSStaticRecordConfig>(k: K, v: DNSStaticRecordConfig[K]) => onChange({ ...value, [k]: v })
  return (
    <div className="flex items-center gap-2">
      <TextInput value={value.name} onChange={v => set('name', v)} placeholder="host.example.com" mono className="flex-1" />
      <Select value={value.type} onChange={v => set('type', v)} options={[
        { value: 'A', label: 'A' }, { value: 'AAAA', label: 'AAAA' },
        { value: 'CNAME', label: 'CNAME' }, { value: 'PTR', label: 'PTR' },
        { value: 'TXT', label: 'TXT' }, { value: 'MX', label: 'MX' }, { value: 'SRV', label: 'SRV' },
      ]} />
      <TextInput value={value.value} onChange={v => set('value', v)} placeholder="10.0.0.1" mono className="flex-1" />
      <NumberInput value={value.ttl} onChange={v => set('ttl', v)} min={0} placeholder="TTL" />
      <button type="button" onClick={onRemove} className="p-1.5 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
        <Trash2 className="w-3.5 h-3.5" />
      </button>
    </div>
  )
}

export default function DNSSection({ value, onChange }: {
  value: DNSProxyConfig
  onChange: (v: DNSProxyConfig) => void
}) {
  const set = <K extends keyof DNSProxyConfig>(key: K, val: DNSProxyConfig[K]) =>
    onChange({ ...value, [key]: val })

  const lists = value.list || []
  const overrides = value.zone_override || []
  const records = value.record || []

  return (
    <Section title="DNS Proxy" description="Built-in DNS server with filtering, caching, and lease registration" defaultOpen={value.enabled}>
      <Toggle checked={value.enabled} onChange={v => set('enabled', v)} label="Enable DNS Proxy" description="Start a built-in DNS server alongside DHCP" />

      {value.enabled && (
        <>
          <FieldGrid>
            <Field label="Listen UDP" hint="address:port for DNS queries">
              <TextInput value={value.listen_udp} onChange={v => set('listen_udp', v)} placeholder="0.0.0.0:53" mono />
            </Field>
            <Field label="Listen DoH" hint="optional DNS-over-HTTPS endpoint">
              <TextInput value={value.listen_doh} onChange={v => set('listen_doh', v)} placeholder="0.0.0.0:443" mono />
            </Field>
            <Field label="Domain" hint="default domain for lease hostnames">
              <TextInput value={value.domain} onChange={v => set('domain', v)} placeholder="home.local" mono />
            </Field>
            <Field label="Default TTL" hint="seconds">
              <NumberInput value={value.ttl} onChange={v => set('ttl', v)} min={1} />
            </Field>
            <Field label="Cache Size" hint="max cached responses">
              <NumberInput value={value.cache_size} onChange={v => set('cache_size', v)} min={100} />
            </Field>
            <Field label="Cache TTL" hint="default cache duration">
              <TextInput value={value.cache_ttl} onChange={v => set('cache_ttl', v)} placeholder="5m" mono />
            </Field>
          </FieldGrid>

          <Toggle checked={value.register_leases} onChange={v => set('register_leases', v)}
            label="Register DHCP Leases" description="Auto-create A records from DHCP lease hostnames" />
          <Toggle checked={value.register_leases_ptr} onChange={v => set('register_leases_ptr', v)}
            label="Register PTR Records" description="Also create reverse DNS (PTR) records for leases" />

          <Field label="Upstream Forwarders" hint="DNS servers to forward non-local queries to">
            <StringArrayInput value={value.forwarders || []} onChange={v => set('forwarders', v)} placeholder="8.8.8.8" mono />
          </Field>

          {/* Filter Lists */}
          <div className="pt-3 border-t border-border/50">
            <div className="flex items-center justify-between mb-3">
              <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Filter Lists ({lists.length})</h4>
              <button type="button" onClick={() => set('list', [...lists, emptyList()])}
                className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors">
                <Plus className="w-3 h-3" /> Add List
              </button>
            </div>
            <div className="space-y-3">
              {lists.map((l, i) => (
                <ListEditor key={i} value={l}
                  onChange={v => { const next = [...lists]; next[i] = v; set('list', next) }}
                  onRemove={() => set('list', lists.filter((_, idx) => idx !== i))} />
              ))}
              {lists.length === 0 && (
                <p className="text-xs text-text-muted italic">No filter lists configured. Add blocklists for ad-blocking and threat protection.</p>
              )}
            </div>
          </div>

          {/* Zone Overrides */}
          <div className="pt-3 border-t border-border/50">
            <div className="flex items-center justify-between mb-3">
              <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Zone Overrides ({overrides.length})</h4>
              <button type="button" onClick={() => set('zone_override', [...overrides, emptyZoneOverride()])}
                className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors">
                <Plus className="w-3 h-3" /> Add Override
              </button>
            </div>
            <div className="space-y-3">
              {overrides.map((o, i) => (
                <ZoneOverrideEditor key={i} value={o}
                  onChange={v => { const next = [...overrides]; next[i] = v; set('zone_override', next) }}
                  onRemove={() => set('zone_override', overrides.filter((_, idx) => idx !== i))} />
              ))}
              {overrides.length === 0 && <p className="text-xs text-text-muted italic">No zone overrides configured</p>}
            </div>
          </div>

          {/* Static Records */}
          <div className="pt-3 border-t border-border/50">
            <div className="flex items-center justify-between mb-3">
              <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Static Records ({records.length})</h4>
              <button type="button" onClick={() => set('record', [...records, emptyStaticRecord()])}
                className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors">
                <Plus className="w-3 h-3" /> Add Record
              </button>
            </div>
            <div className="space-y-2">
              {records.map((r, i) => (
                <StaticRecordEditor key={i} value={r}
                  onChange={v => { const next = [...records]; next[i] = v; set('record', next) }}
                  onRemove={() => set('record', records.filter((_, idx) => idx !== i))} />
              ))}
              {records.length === 0 && <p className="text-xs text-text-muted italic">No static DNS records</p>}
            </div>
          </div>
        </>
      )}
    </Section>
  )
}
