import type { DefaultsConfig } from '@/lib/configTypes'
import { Section, FieldGrid, Field, TextInput, StringArrayInput } from '@/components/FormFields'

export default function DefaultsSection({ value, onChange }: {
  value: DefaultsConfig
  onChange: (v: DefaultsConfig) => void
}) {
  const set = <K extends keyof DefaultsConfig>(key: K, val: DefaultsConfig[K]) =>
    onChange({ ...value, [key]: val })

  return (
    <Section title="Defaults" description="Global defaults applied to all subnets unless overridden">
      <FieldGrid>
        <Field label="Default Lease Time" hint="Go duration">
          <TextInput value={value.lease_time} onChange={v => set('lease_time', v)} placeholder="12h" mono />
        </Field>
        <Field label="Renewal Time (T1)" hint="when clients try to renew">
          <TextInput value={value.renewal_time} onChange={v => set('renewal_time', v)} placeholder="6h" mono />
        </Field>
        <Field label="Rebind Time (T2)" hint="when clients try to rebind">
          <TextInput value={value.rebind_time} onChange={v => set('rebind_time', v)} placeholder="10h30m" mono />
        </Field>
        <Field label="Domain Name">
          <TextInput value={value.domain_name} onChange={v => set('domain_name', v)} placeholder="example.com" mono />
        </Field>
      </FieldGrid>
      <Field label="DNS Servers">
        <StringArrayInput value={value.dns_servers || []} onChange={v => set('dns_servers', v)} placeholder="8.8.8.8" mono />
      </Field>
    </Section>
  )
}
