import type { ServerConfig } from '@/lib/configTypes'
import { Section, FieldGrid, Field, TextInput, NumberInput, Toggle, Select } from '@/components/FormFields'

export default function ServerSection({ value, onChange }: {
  value: ServerConfig
  onChange: (v: ServerConfig) => void
}) {
  const set = <K extends keyof ServerConfig>(key: K, val: ServerConfig[K]) =>
    onChange({ ...value, [key]: val })

  return (
    <Section title="Server" description="Core server settings — interface, bind address, logging">
      <FieldGrid>
        <Field label="Network Interface" hint="e.g. eth0">
          <TextInput value={value.interface} onChange={v => set('interface', v)} placeholder="eth0" mono />
        </Field>
        <Field label="Bind Address" hint="ip:port">
          <TextInput value={value.bind_address} onChange={v => set('bind_address', v)} placeholder="0.0.0.0:67" mono />
        </Field>
        <Field label="Server ID" hint="your server's IP, sent in option 54">
          <TextInput value={value.server_id} onChange={v => set('server_id', v)} placeholder="192.168.1.1" mono />
        </Field>
        <Field label="Log Level">
          <Select value={value.log_level} onChange={v => set('log_level', v)} options={[
            { value: 'debug', label: 'Debug' },
            { value: 'info', label: 'Info' },
            { value: 'warn', label: 'Warn' },
            { value: 'error', label: 'Error' },
          ]} />
        </Field>
        <Field label="Lease Database Path">
          <TextInput value={value.lease_db} onChange={v => set('lease_db', v)} placeholder="/var/lib/athena-dhcpd/leases.db" mono />
        </Field>
        <Field label="PID File" hint="empty to disable">
          <TextInput value={value.pid_file} onChange={v => set('pid_file', v)} placeholder="/var/run/athena-dhcpd.pid" mono />
        </Field>
      </FieldGrid>

      <div className="pt-3 border-t border-border/50">
        <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider mb-3">Rate Limiting</h4>
        <Toggle
          checked={value.rate_limit.enabled}
          onChange={v => set('rate_limit', { ...value.rate_limit, enabled: v })}
          label="Enable Rate Limiting"
          description="Prevent DHCP starvation attacks from misbehaving clients"
        />
        {value.rate_limit.enabled && (
          <FieldGrid>
            <Field label="Max DISCOVERs/sec" hint="global limit">
              <NumberInput value={value.rate_limit.max_discovers_per_second} onChange={v => set('rate_limit', { ...value.rate_limit, max_discovers_per_second: v })} min={1} />
            </Field>
            <Field label="Max per MAC/sec">
              <NumberInput value={value.rate_limit.max_per_mac_per_second} onChange={v => set('rate_limit', { ...value.rate_limit, max_per_mac_per_second: v })} min={1} />
            </Field>
          </FieldGrid>
        )}
      </div>
    </Section>
  )
}
