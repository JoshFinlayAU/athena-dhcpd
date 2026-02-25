import type { DDNSConfig, DDNSZoneConfig, DDNSZoneOverride } from '@/lib/configTypes'
import { Section, FieldGrid, Field, TextInput, NumberInput, Toggle, Select } from '@/components/FormFields'
import { Plus, Trash2 } from 'lucide-react'

function ZoneEditor({ label, value, onChange }: {
  label: string
  value: DDNSZoneConfig
  onChange: (v: DDNSZoneConfig) => void
}) {
  const set = <K extends keyof DDNSZoneConfig>(k: K, v: DDNSZoneConfig[K]) => onChange({ ...value, [k]: v })
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

function emptyOverride(): DDNSZoneOverride {
  return { subnet: '', forward_zone: '', reverse_zone: '', method: 'rfc2136', server: '', api_key: '', tsig_name: '', tsig_algorithm: 'hmac-sha256', tsig_secret: '' }
}

export default function DDNSSection({ value, onChange }: {
  value: DDNSConfig
  onChange: (v: DDNSConfig) => void
}) {
  const set = <K extends keyof DDNSConfig>(key: K, val: DDNSConfig[K]) =>
    onChange({ ...value, [key]: val })

  const overrides = value.zone_override || []

  return (
    <Section title="Dynamic DNS" description="Automatic A/PTR record management on lease events" defaultOpen={value.enabled}>
      <Toggle
        checked={value.enabled}
        onChange={v => set('enabled', v)}
        label="Enable Dynamic DNS"
        description="Register/remove DNS records on lease ack/release/expire"
      />

      {value.enabled && (
        <>
          <FieldGrid>
            <Field label="TTL" hint="seconds">
              <NumberInput value={value.ttl} onChange={v => set('ttl', v)} min={60} />
            </Field>
            <Field label="Conflict Policy">
              <Select value={value.conflict_policy} onChange={v => set('conflict_policy', v)} options={[
                { value: 'overwrite', label: 'Overwrite existing records' },
                { value: 'client_wins', label: 'Client wins (FQDN takes priority)' },
                { value: 'server_wins', label: 'Server wins' },
              ]} />
            </Field>
          </FieldGrid>

          <div className="flex flex-col gap-3">
            <Toggle checked={value.allow_client_fqdn} onChange={v => set('allow_client_fqdn', v)}
              label="Allow Client FQDN" description="Honour client-supplied FQDN from option 81" />
            <Toggle checked={value.fallback_to_mac} onChange={v => set('fallback_to_mac', v)}
              label="Fallback to MAC" description="Generate hostname from MAC if no hostname/FQDN available" />
            <Toggle checked={value.update_on_renew} onChange={v => set('update_on_renew', v)}
              label="Update on Renew" description="Also update DNS records on lease renewals, not just initial ACK" />
            <Toggle checked={value.use_dhcid} onChange={v => set('use_dhcid', v)}
              label="Use DHCID Records" description="RFC 4701 conflict detection for multi-server environments" />
          </div>

          <ZoneEditor label="Forward Zone (A Records)" value={value.forward} onChange={v => set('forward', v)} />
          <ZoneEditor label="Reverse Zone (PTR Records)" value={value.reverse} onChange={v => set('reverse', v)} />

          {/* Zone overrides */}
          <div className="pt-3 border-t border-border/50">
            <div className="flex items-center justify-between mb-3">
              <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Per-Subnet Zone Overrides ({overrides.length})</h4>
              <button type="button" onClick={() => set('zone_override', [...overrides, emptyOverride()])}
                className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors">
                <Plus className="w-3 h-3" /> Add Override
              </button>
            </div>
            {overrides.map((o, i) => (
              <div key={i} className="border border-border/50 rounded-lg p-4 space-y-3 bg-surface/50 mb-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-semibold text-warning">{o.subnet || 'New Override'}</span>
                  <button type="button" onClick={() => set('zone_override', overrides.filter((_, idx) => idx !== i))}
                    className="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
                <FieldGrid>
                  <Field label="Subnet"><TextInput value={o.subnet} onChange={v => { const n = [...overrides]; n[i] = { ...o, subnet: v }; set('zone_override', n) }} placeholder="10.0.0.0/24" mono /></Field>
                  <Field label="Forward Zone"><TextInput value={o.forward_zone} onChange={v => { const n = [...overrides]; n[i] = { ...o, forward_zone: v }; set('zone_override', n) }} placeholder="lab.example.com." mono /></Field>
                  <Field label="Reverse Zone"><TextInput value={o.reverse_zone} onChange={v => { const n = [...overrides]; n[i] = { ...o, reverse_zone: v }; set('zone_override', n) }} placeholder="0.0.10.in-addr.arpa." mono /></Field>
                  <Field label="Method">
                    <Select value={o.method} onChange={v => { const n = [...overrides]; n[i] = { ...o, method: v }; set('zone_override', n) }} options={[
                      { value: 'rfc2136', label: 'RFC 2136' }, { value: 'powerdns_api', label: 'PowerDNS API' }, { value: 'technitium_api', label: 'Technitium API' },
                    ]} />
                  </Field>
                  <Field label="Server"><TextInput value={o.server} onChange={v => { const n = [...overrides]; n[i] = { ...o, server: v }; set('zone_override', n) }} mono /></Field>
                  {(o.method === 'powerdns_api' || o.method === 'technitium_api') ? (
                    <Field label="API Key"><TextInput value={o.api_key} onChange={v => { const n = [...overrides]; n[i] = { ...o, api_key: v }; set('zone_override', n) }} placeholder="api-key" /></Field>
                  ) : (
                    <>
                      <Field label="TSIG Key Name"><TextInput value={o.tsig_name} onChange={v => { const n = [...overrides]; n[i] = { ...o, tsig_name: v }; set('zone_override', n) }} placeholder="dhcp-update." mono /></Field>
                      <Field label="TSIG Algorithm">
                        <Select value={o.tsig_algorithm} onChange={v => { const n = [...overrides]; n[i] = { ...o, tsig_algorithm: v }; set('zone_override', n) }} options={[
                          { value: 'hmac-md5', label: 'HMAC-MD5' }, { value: 'hmac-sha1', label: 'HMAC-SHA1' }, { value: 'hmac-sha256', label: 'HMAC-SHA256' }, { value: 'hmac-sha512', label: 'HMAC-SHA512' },
                        ]} />
                      </Field>
                      <Field label="TSIG Secret" hint="base64"><TextInput value={o.tsig_secret} onChange={v => { const n = [...overrides]; n[i] = { ...o, tsig_secret: v }; set('zone_override', n) }} placeholder="base64-encoded-secret" /></Field>
                    </>
                  )}
                </FieldGrid>
              </div>
            ))}
            {overrides.length === 0 && <p className="text-xs text-text-muted italic">No zone overrides — all subnets use the main forward/reverse zones</p>}
          </div>
        </>
      )}
    </Section>
  )
}
