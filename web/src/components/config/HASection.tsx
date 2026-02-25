import type { HAConfig } from '@/lib/configTypes'
import { Section, FieldGrid, Field, TextInput, NumberInput, Toggle, Select } from '@/components/FormFields'

export default function HASection({ value, onChange }: {
  value: HAConfig
  onChange: (v: HAConfig) => void
}) {
  const set = <K extends keyof HAConfig>(key: K, val: HAConfig[K]) =>
    onChange({ ...value, [key]: val })

  return (
    <Section title="High Availability" description="Active-standby failover with peer lease synchronisation" defaultOpen={value.enabled}>
      <Toggle
        checked={value.enabled}
        onChange={v => set('enabled', v)}
        label="Enable High Availability"
        description="Sync leases and conflicts with a peer node for automatic failover"
      />

      {value.enabled && (
        <>
          <FieldGrid>
            <Field label="Role">
              <Select value={value.role} onChange={v => set('role', v)} options={[
                { value: 'primary', label: 'Primary — starts as active' },
                { value: 'secondary', label: 'Secondary — starts as standby' },
              ]} />
            </Field>
            <Field label="Peer Address" hint="the other node's ip:port">
              <TextInput value={value.peer_address} onChange={v => set('peer_address', v)} placeholder="192.168.1.2:8067" mono />
            </Field>
            <Field label="Listen Address" hint="bind address for peer connections">
              <TextInput value={value.listen_address} onChange={v => set('listen_address', v)} placeholder="0.0.0.0:8067" mono />
            </Field>
            <Field label="Heartbeat Interval">
              <TextInput value={value.heartbeat_interval} onChange={v => set('heartbeat_interval', v)} placeholder="1s" mono />
            </Field>
            <Field label="Failover Timeout" hint="declare peer dead after this">
              <TextInput value={value.failover_timeout} onChange={v => set('failover_timeout', v)} placeholder="10s" mono />
            </Field>
            <Field label="Sync Batch Size" hint="leases per batch during bulk sync">
              <NumberInput value={value.sync_batch_size} onChange={v => set('sync_batch_size', v)} min={1} />
            </Field>
          </FieldGrid>

          <div className="pt-3 border-t border-border/50">
            <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider mb-3">TLS</h4>
            <Toggle
              checked={value.tls.enabled}
              onChange={v => set('tls', { ...value.tls, enabled: v })}
              label="Enable TLS"
              description="Encrypt peer communication — recommended for untrusted networks"
            />
            {value.tls.enabled && (
              <FieldGrid>
                <Field label="Certificate File">
                  <TextInput value={value.tls.cert_file} onChange={v => set('tls', { ...value.tls, cert_file: v })} placeholder="/etc/athena-dhcpd/tls/server.crt" mono />
                </Field>
                <Field label="Key File">
                  <TextInput value={value.tls.key_file} onChange={v => set('tls', { ...value.tls, key_file: v })} placeholder="/etc/athena-dhcpd/tls/server.key" mono />
                </Field>
                <Field label="CA File" hint="for peer certificate verification">
                  <TextInput value={value.tls.ca_file} onChange={v => set('tls', { ...value.tls, ca_file: v })} placeholder="/etc/athena-dhcpd/tls/ca.crt" mono />
                </Field>
              </FieldGrid>
            )}
          </div>
        </>
      )}
    </Section>
  )
}
