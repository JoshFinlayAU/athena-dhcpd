import type { HooksConfig, ScriptHook, WebhookHook } from '@/lib/configTypes'
import { Section, FieldGrid, Field, TextInput, NumberInput, Select, StringArrayInput, KeyValueInput } from '@/components/FormFields'
import { Plus, Trash2 } from 'lucide-react'

function emptyScript(): ScriptHook {
  return { name: '', events: [], command: '', timeout: '10s', subnets: [] }
}

function emptyWebhook(): WebhookHook {
  return { name: '', events: [], url: '', method: 'POST', headers: {}, timeout: '5s', retries: 3, retry_backoff: '2s', secret: '', template: '' }
}

function ScriptEditor({ value, onChange, onRemove }: {
  value: ScriptHook
  onChange: (v: ScriptHook) => void
  onRemove: () => void
}) {
  const set = <K extends keyof ScriptHook>(k: K, v: ScriptHook[K]) => onChange({ ...value, [k]: v })
  return (
    <div className="border border-border/50 rounded-lg p-4 space-y-3 bg-surface/50">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-accent">{value.name || 'New Script Hook'}</span>
        <button type="button" onClick={onRemove} className="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
          <Trash2 className="w-3.5 h-3.5" />
        </button>
      </div>
      <FieldGrid>
        <Field label="Name"><TextInput value={value.name} onChange={v => set('name', v)} placeholder="my-hook" /></Field>
        <Field label="Command"><TextInput value={value.command} onChange={v => set('command', v)} placeholder="/usr/local/bin/hook.sh" mono /></Field>
        <Field label="Timeout"><TextInput value={value.timeout} onChange={v => set('timeout', v)} placeholder="10s" mono /></Field>
      </FieldGrid>
      <Field label="Events" hint="empty = all events, supports wildcards like lease.*">
        <StringArrayInput value={value.events} onChange={v => set('events', v)} placeholder="lease.ack" />
      </Field>
      <Field label="Subnet Filter" hint="optional — only fire for these subnets">
        <StringArrayInput value={value.subnets} onChange={v => set('subnets', v)} placeholder="192.168.1.0/24" mono />
      </Field>
    </div>
  )
}

function WebhookEditor({ value, onChange, onRemove }: {
  value: WebhookHook
  onChange: (v: WebhookHook) => void
  onRemove: () => void
}) {
  const set = <K extends keyof WebhookHook>(k: K, v: WebhookHook[K]) => onChange({ ...value, [k]: v })
  return (
    <div className="border border-border/50 rounded-lg p-4 space-y-3 bg-surface/50">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-info">{value.name || 'New Webhook'}</span>
        <button type="button" onClick={onRemove} className="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
          <Trash2 className="w-3.5 h-3.5" />
        </button>
      </div>
      <FieldGrid>
        <Field label="Name"><TextInput value={value.name} onChange={v => set('name', v)} placeholder="slack-alerts" /></Field>
        <Field label="URL"><TextInput value={value.url} onChange={v => set('url', v)} placeholder="https://hooks.slack.com/..." mono /></Field>
        <Field label="Method">
          <Select value={value.method} onChange={v => set('method', v)} options={[
            { value: 'POST', label: 'POST' }, { value: 'PUT', label: 'PUT' }, { value: 'PATCH', label: 'PATCH' },
          ]} />
        </Field>
        <Field label="Template">
          <Select value={value.template} onChange={v => set('template', v)} options={[
            { value: '', label: 'Raw JSON (no template)' },
            { value: 'slack', label: 'Slack' },
            { value: 'teams', label: 'Microsoft Teams' },
          ]} />
        </Field>
        <Field label="Timeout"><TextInput value={value.timeout} onChange={v => set('timeout', v)} placeholder="5s" mono /></Field>
        <Field label="Retries"><NumberInput value={value.retries} onChange={v => set('retries', v)} min={0} max={10} /></Field>
        <Field label="Retry Backoff"><TextInput value={value.retry_backoff} onChange={v => set('retry_backoff', v)} placeholder="2s" mono /></Field>
        <Field label="HMAC Secret" hint="signs requests with X-Athena-Signature">
          <TextInput value={value.secret} onChange={v => set('secret', v)} placeholder="optional secret" />
        </Field>
      </FieldGrid>
      <Field label="Events" hint="empty = all events">
        <StringArrayInput value={value.events} onChange={v => set('events', v)} placeholder="conflict.detected" />
      </Field>
      <Field label="Custom Headers">
        <KeyValueInput value={value.headers || {}} onChange={v => set('headers', v)} keyPlaceholder="Header-Name" valuePlaceholder="value" />
      </Field>
    </div>
  )
}

export default function HooksSection({ value, onChange }: {
  value: HooksConfig
  onChange: (v: HooksConfig) => void
}) {
  const set = <K extends keyof HooksConfig>(key: K, val: HooksConfig[K]) =>
    onChange({ ...value, [key]: val })

  const scripts = value.script || []
  const webhooks = value.webhook || []

  return (
    <Section title="Event Hooks" description="Script and webhook hooks triggered by DHCP events" defaultOpen={scripts.length > 0 || webhooks.length > 0}>
      <FieldGrid>
        <Field label="Event Buffer Size" hint="events dropped if buffer fills">
          <NumberInput value={value.event_buffer_size} onChange={v => set('event_buffer_size', v)} min={100} />
        </Field>
        <Field label="Script Concurrency" hint="max simultaneous scripts">
          <NumberInput value={value.script_concurrency} onChange={v => set('script_concurrency', v)} min={1} max={32} />
        </Field>
        <Field label="Default Script Timeout">
          <TextInput value={value.script_timeout} onChange={v => set('script_timeout', v)} placeholder="10s" mono />
        </Field>
      </FieldGrid>

      {/* Script hooks */}
      <div className="pt-3 border-t border-border/50">
        <div className="flex items-center justify-between mb-3">
          <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Script Hooks ({scripts.length})</h4>
          <button
            type="button"
            onClick={() => set('script', [...scripts, emptyScript()])}
            className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors"
          >
            <Plus className="w-3 h-3" /> Add Script
          </button>
        </div>
        <div className="space-y-3">
          {scripts.map((s, i) => (
            <ScriptEditor
              key={i}
              value={s}
              onChange={v => { const next = [...scripts]; next[i] = v; set('script', next) }}
              onRemove={() => set('script', scripts.filter((_, idx) => idx !== i))}
            />
          ))}
          {scripts.length === 0 && <p className="text-xs text-text-muted italic">No script hooks configured</p>}
        </div>
      </div>

      {/* Webhook hooks */}
      <div className="pt-3 border-t border-border/50">
        <div className="flex items-center justify-between mb-3">
          <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Webhooks ({webhooks.length})</h4>
          <button
            type="button"
            onClick={() => set('webhook', [...webhooks, emptyWebhook()])}
            className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors"
          >
            <Plus className="w-3 h-3" /> Add Webhook
          </button>
        </div>
        <div className="space-y-3">
          {webhooks.map((w, i) => (
            <WebhookEditor
              key={i}
              value={w}
              onChange={v => { const next = [...webhooks]; next[i] = v; set('webhook', next) }}
              onRemove={() => set('webhook', webhooks.filter((_, idx) => idx !== i))}
            />
          ))}
          {webhooks.length === 0 && <p className="text-xs text-text-muted italic">No webhooks configured</p>}
        </div>
      </div>
    </Section>
  )
}
