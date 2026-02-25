import type { ConflictDetectionConfig } from '@/lib/configTypes'
import { Section, FieldGrid, Field, TextInput, NumberInput, Toggle, Select } from '@/components/FormFields'

export default function ConflictSection({ value, onChange }: {
  value: ConflictDetectionConfig
  onChange: (v: ConflictDetectionConfig) => void
}) {
  const set = <K extends keyof ConflictDetectionConfig>(key: K, val: ConflictDetectionConfig[K]) =>
    onChange({ ...value, [key]: val })

  return (
    <Section title="Conflict Detection" description="ARP/ICMP probing to detect IP conflicts before offering">
      <Toggle
        checked={value.enabled}
        onChange={v => set('enabled', v)}
        label="Enable Conflict Detection"
        description="Probe candidate IPs before every DHCPOFFER"
      />

      {value.enabled && (
        <>
          <FieldGrid>
            <Field label="Probe Strategy">
              <Select value={value.probe_strategy} onChange={v => set('probe_strategy', v)} options={[
                { value: 'sequential', label: 'Sequential — probe one at a time' },
                { value: 'parallel', label: 'Parallel — probe multiple simultaneously' },
              ]} />
            </Field>
            <Field label="Probe Timeout" hint="Go duration, e.g. 500ms">
              <TextInput value={value.probe_timeout} onChange={v => set('probe_timeout', v)} placeholder="500ms" mono />
            </Field>
            <Field label="Max Probes per DISCOVER">
              <NumberInput value={value.max_probes_per_discover} onChange={v => set('max_probes_per_discover', v)} min={1} max={10} />
            </Field>
            {value.probe_strategy === 'parallel' && (
              <Field label="Parallel Probe Count">
                <NumberInput value={value.parallel_probe_count} onChange={v => set('parallel_probe_count', v)} min={1} max={10} />
              </Field>
            )}
            <Field label="Conflict Hold Time" hint="how long IPs stay flagged">
              <TextInput value={value.conflict_hold_time} onChange={v => set('conflict_hold_time', v)} placeholder="1h" mono />
            </Field>
            <Field label="Max Conflict Count" hint="permanent flag after this many">
              <NumberInput value={value.max_conflict_count} onChange={v => set('max_conflict_count', v)} min={1} />
            </Field>
            <Field label="Probe Cache TTL" hint="cache clear results this long">
              <TextInput value={value.probe_cache_ttl} onChange={v => set('probe_cache_ttl', v)} placeholder="10s" mono />
            </Field>
            <Field label="Probe Log Level">
              <Select value={value.probe_log_level} onChange={v => set('probe_log_level', v)} options={[
                { value: 'debug', label: 'Debug' },
                { value: 'info', label: 'Info' },
                { value: 'warn', label: 'Warn' },
              ]} />
            </Field>
          </FieldGrid>

          <div className="flex flex-col gap-3 pt-2">
            <Toggle
              checked={value.send_gratuitous_arp}
              onChange={v => set('send_gratuitous_arp', v)}
              label="Send Gratuitous ARP"
              description="Send gratuitous ARP after DHCPACK on local subnets to update switch ARP caches"
            />
            <Toggle
              checked={value.icmp_fallback}
              onChange={v => set('icmp_fallback', v)}
              label="ICMP Fallback"
              description="Use ICMP ping for remote/relayed subnets where ARP isn't available"
            />
          </div>
        </>
      )}
    </Section>
  )
}
