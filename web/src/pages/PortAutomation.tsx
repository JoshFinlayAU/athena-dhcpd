import { useState, useCallback } from 'react'
import { useApi } from '@/hooks/useApi'
import { Card } from '@/components/Card'
import { Zap, Plus, Trash2, Save, TestTube, ChevronDown, ChevronRight, ToggleLeft, ToggleRight } from 'lucide-react'
import {
  v2GetPortAutoRules, v2SetPortAutoRules, v2TestPortAutoRules,
  type PortAutoRule, type PortAutoAction, type PortAutoLeaseContext, type PortAutoMatchResult,
} from '@/lib/api'

const emptyRule: PortAutoRule = {
  name: '',
  enabled: true,
  priority: 0,
  mac_patterns: [],
  subnets: [],
  circuit_ids: [],
  remote_ids: [],
  device_types: [],
  actions: [{ type: 'log' }],
}

const emptyTestCtx: PortAutoLeaseContext = {
  mac: '',
  ip: '',
  hostname: '',
  subnet: '',
  circuit_id: '',
  remote_id: '',
  device_type: '',
  vendor: '',
}

export default function PortAutomation() {
  const { data: savedRules, refetch } = useApi(useCallback(() => v2GetPortAutoRules(), []))
  const [rules, setRules] = useState<PortAutoRule[] | null>(null)
  const [status, setStatus] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)
  const [saving, setSaving] = useState(false)
  const [expanded, setExpanded] = useState<Set<number>>(new Set())
  const [showTest, setShowTest] = useState(false)
  const [testCtx, setTestCtx] = useState<PortAutoLeaseContext>(emptyTestCtx)
  const [testResults, setTestResults] = useState<PortAutoMatchResult[] | null>(null)
  const [testing, setTesting] = useState(false)
  const [dirty, setDirty] = useState(false)

  // Use local state if edited, otherwise saved rules
  const currentRules = rules ?? savedRules ?? []

  const showStatus = (type: 'success' | 'error', msg: string) => {
    setStatus({ type, msg })
    setTimeout(() => setStatus(null), 4000)
  }

  const updateRules = (newRules: PortAutoRule[]) => {
    setRules(newRules)
    setDirty(true)
  }

  const toggle = (idx: number) => {
    setExpanded(prev => {
      const next = new Set(prev)
      next.has(idx) ? next.delete(idx) : next.add(idx)
      return next
    })
  }

  const addRule = () => {
    const newRules = [...currentRules, { ...emptyRule, name: `rule-${currentRules.length + 1}` }]
    updateRules(newRules)
    setExpanded(prev => new Set(prev).add(newRules.length - 1))
  }

  const removeRule = (idx: number) => {
    updateRules(currentRules.filter((_, i) => i !== idx))
    setExpanded(prev => {
      const next = new Set<number>()
      prev.forEach(i => { if (i < idx) next.add(i); else if (i > idx) next.add(i - 1) })
      return next
    })
  }

  const updateRule = (idx: number, patch: Partial<PortAutoRule>) => {
    updateRules(currentRules.map((r, i) => i === idx ? { ...r, ...patch } : r))
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      await v2SetPortAutoRules(currentRules)
      showStatus('success', `Saved ${currentRules.length} rule${currentRules.length !== 1 ? 's' : ''}`)
      setDirty(false)
      refetch()
    } catch (e) {
      showStatus('error', e instanceof Error ? e.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  const handleTest = async () => {
    setTesting(true)
    setTestResults(null)
    try {
      const results = await v2TestPortAutoRules(testCtx)
      setTestResults(results)
    } catch (e) {
      showStatus('error', e instanceof Error ? e.message : 'Test failed')
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="p-6 max-w-5xl space-y-4">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Zap className="w-6 h-6" /> Port Automation
          </h1>
          <p className="text-sm text-text-secondary mt-0.5">
            DHCP-driven rules for VLAN assignment, webhooks, and tagging based on MAC, option 82, fingerprint, or subnet
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => setShowTest(!showTest)}
            className="flex items-center gap-1.5 px-4 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
          >
            <TestTube className="w-3.5 h-3.5" />
            Test
          </button>
          <button
            onClick={addRule}
            className="flex items-center gap-1.5 px-4 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
          >
            <Plus className="w-3.5 h-3.5" />
            Add Rule
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !dirty}
            className="flex items-center gap-1.5 px-4 py-2 text-xs font-medium rounded-lg bg-accent text-white hover:bg-accent-hover disabled:opacity-50 transition-colors"
          >
            <Save className="w-3.5 h-3.5" />
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>

      {status && (
        <div className={`px-4 py-2.5 rounded-lg text-sm ${status.type === 'success' ? 'bg-success/15 text-success' : 'bg-danger/15 text-danger'}`}>
          {status.msg}
        </div>
      )}

      {dirty && (
        <div className="px-4 py-2.5 rounded-lg text-sm bg-warning/15 text-warning">
          Unsaved changes
        </div>
      )}

      {/* Test Panel */}
      {showTest && (
        <Card className="p-4 space-y-3">
          <h3 className="text-sm font-semibold">Test Rules</h3>
          <p className="text-xs text-text-muted">Enter a sample lease context to see which rules match</p>
          <div className="grid grid-cols-4 gap-2">
            {(['mac', 'ip', 'hostname', 'subnet', 'circuit_id', 'remote_id', 'device_type', 'vendor'] as const).map(field => (
              <input
                key={field}
                placeholder={field}
                value={testCtx[field] || ''}
                onChange={e => setTestCtx({ ...testCtx, [field]: e.target.value })}
                className="px-2.5 py-1.5 text-xs rounded-lg bg-surface-base border border-border text-text-primary placeholder:text-text-muted"
              />
            ))}
          </div>
          <div className="flex items-center gap-3">
            <button
              onClick={handleTest}
              disabled={testing || !testCtx.mac || !testCtx.ip}
              className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg bg-accent text-white hover:bg-accent-hover disabled:opacity-50 transition-colors"
            >
              <TestTube className="w-3 h-3" />
              {testing ? 'Testing...' : 'Run Test'}
            </button>
            {testResults !== null && (
              <span className="text-xs text-text-muted">
                {testResults.length === 0 ? 'No rules matched' : `${testResults.length} rule${testResults.length !== 1 ? 's' : ''} matched`}
              </span>
            )}
          </div>
          {testResults && testResults.length > 0 && (
            <div className="space-y-1">
              {testResults.map((r, i) => (
                <div key={i} className="px-3 py-2 rounded-lg bg-success/10 text-xs">
                  <span className="font-medium text-success">{r.rule}</span>
                  <span className="text-text-muted ml-2">
                    → {r.actions.map(a => a.type === 'tag' ? `tag:${a.tag} vlan:${a.vlan}` : a.type).join(', ')}
                  </span>
                </div>
              ))}
            </div>
          )}
        </Card>
      )}

      {/* Rules */}
      {currentRules.length === 0 ? (
        <Card className="p-8 text-center">
          <Zap className="w-10 h-10 text-text-muted mx-auto mb-3 opacity-50" />
          <p className="text-sm font-medium text-text-secondary">No automation rules configured</p>
          <p className="text-xs text-text-muted mt-1">Add a rule to get started</p>
        </Card>
      ) : (
        <div className="space-y-2">
          {currentRules.map((rule, idx) => (
            <RuleCard
              key={idx}
              rule={rule}
              idx={idx}
              expanded={expanded.has(idx)}
              onToggle={() => toggle(idx)}
              onUpdate={(patch) => updateRule(idx, patch)}
              onRemove={() => removeRule(idx)}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function RuleCard({ rule, idx, expanded, onToggle, onUpdate, onRemove }: {
  rule: PortAutoRule
  idx: number
  expanded: boolean
  onToggle: () => void
  onUpdate: (patch: Partial<PortAutoRule>) => void
  onRemove: () => void
}) {
  return (
    <Card className="overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-3 cursor-pointer hover:bg-surface-overlay/50 transition-colors" onClick={onToggle}>
        {expanded ? <ChevronDown className="w-4 h-4 text-text-muted" /> : <ChevronRight className="w-4 h-4 text-text-muted" />}
        <button
          onClick={e => { e.stopPropagation(); onUpdate({ enabled: !rule.enabled }) }}
          className="flex-shrink-0"
          title={rule.enabled ? 'Disable' : 'Enable'}
        >
          {rule.enabled
            ? <ToggleRight className="w-5 h-5 text-success" />
            : <ToggleLeft className="w-5 h-5 text-text-muted" />
          }
        </button>
        <span className={`text-sm font-medium ${rule.enabled ? 'text-text-primary' : 'text-text-muted'}`}>
          {rule.name || `Rule ${idx + 1}`}
        </span>
        <span className="text-xs text-text-muted ml-auto mr-2">
          priority {rule.priority} · {rule.actions.length} action{rule.actions.length !== 1 ? 's' : ''}
        </span>
        <button
          onClick={e => { e.stopPropagation(); onRemove() }}
          className="p-1 rounded hover:bg-surface-overlay text-text-muted hover:text-danger transition-colors"
          title="Delete rule"
        >
          <Trash2 className="w-3.5 h-3.5" />
        </button>
      </div>

      {/* Body */}
      {expanded && (
        <div className="border-t border-border px-4 py-4 space-y-4">
          {/* Basic fields */}
          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-muted mb-1">Name</label>
              <input
                value={rule.name}
                onChange={e => onUpdate({ name: e.target.value })}
                className="w-full px-2.5 py-1.5 text-xs rounded-lg bg-surface-base border border-border text-text-primary"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-muted mb-1">Priority</label>
              <input
                type="number"
                value={rule.priority}
                onChange={e => onUpdate({ priority: parseInt(e.target.value) || 0 })}
                className="w-full px-2.5 py-1.5 text-xs rounded-lg bg-surface-base border border-border text-text-primary"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-muted mb-1">Enabled</label>
              <select
                value={rule.enabled ? 'true' : 'false'}
                onChange={e => onUpdate({ enabled: e.target.value === 'true' })}
                className="w-full px-2.5 py-1.5 text-xs rounded-lg bg-surface-base border border-border text-text-primary"
              >
                <option value="true">Yes</option>
                <option value="false">No</option>
              </select>
            </div>
          </div>

          {/* Match criteria */}
          <div>
            <h4 className="text-xs font-semibold text-text-secondary mb-2">Match Criteria (all non-empty must match)</h4>
            <div className="grid grid-cols-2 gap-3">
              <PatternField label="MAC Patterns (regex)" value={rule.mac_patterns} onChange={v => onUpdate({ mac_patterns: v })} />
              <PatternField label="Subnets (CIDR)" value={rule.subnets} onChange={v => onUpdate({ subnets: v })} />
              <PatternField label="Circuit IDs (regex)" value={rule.circuit_ids} onChange={v => onUpdate({ circuit_ids: v })} />
              <PatternField label="Remote IDs (regex)" value={rule.remote_ids} onChange={v => onUpdate({ remote_ids: v })} />
              <PatternField label="Device Types" value={rule.device_types} onChange={v => onUpdate({ device_types: v })} />
            </div>
          </div>

          {/* Actions */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <h4 className="text-xs font-semibold text-text-secondary">Actions</h4>
              <button
                onClick={() => onUpdate({ actions: [...rule.actions, { type: 'log' }] })}
                className="text-xs text-accent hover:text-accent-hover transition-colors"
              >
                + Add Action
              </button>
            </div>
            <div className="space-y-2">
              {rule.actions.map((action, ai) => (
                <ActionEditor
                  key={ai}
                  action={action}
                  onUpdate={patch => {
                    const newActions = [...rule.actions]
                    newActions[ai] = { ...action, ...patch }
                    onUpdate({ actions: newActions })
                  }}
                  onRemove={() => onUpdate({ actions: rule.actions.filter((_, i) => i !== ai) })}
                />
              ))}
            </div>
          </div>
        </div>
      )}
    </Card>
  )
}

function PatternField({ label, value, onChange }: {
  label: string
  value?: string[]
  onChange: (v: string[]) => void
}) {
  const str = (value || []).join(', ')
  return (
    <div>
      <label className="block text-xs font-medium text-text-muted mb-1">{label}</label>
      <input
        value={str}
        onChange={e => {
          const v = e.target.value
          onChange(v ? v.split(',').map(s => s.trim()).filter(Boolean) : [])
        }}
        placeholder="comma-separated"
        className="w-full px-2.5 py-1.5 text-xs rounded-lg bg-surface-base border border-border text-text-primary placeholder:text-text-muted"
      />
    </div>
  )
}

function ActionEditor({ action, onUpdate, onRemove }: {
  action: PortAutoAction
  onUpdate: (patch: Partial<PortAutoAction>) => void
  onRemove: () => void
}) {
  return (
    <div className="flex items-start gap-2 p-3 rounded-lg bg-surface-base border border-border">
      <select
        value={action.type}
        onChange={e => onUpdate({ type: e.target.value as PortAutoAction['type'] })}
        className="px-2 py-1.5 text-xs rounded-lg bg-surface-raised border border-border text-text-primary"
      >
        <option value="log">Log</option>
        <option value="webhook">Webhook</option>
        <option value="tag">Tag/VLAN</option>
      </select>

      {action.type === 'webhook' && (
        <div className="flex-1 grid grid-cols-2 gap-2">
          <input
            value={action.url || ''}
            onChange={e => onUpdate({ url: e.target.value })}
            placeholder="URL"
            className="px-2.5 py-1.5 text-xs rounded-lg bg-surface-raised border border-border text-text-primary placeholder:text-text-muted col-span-2"
          />
          <select
            value={action.method || 'POST'}
            onChange={e => onUpdate({ method: e.target.value })}
            className="px-2 py-1.5 text-xs rounded-lg bg-surface-raised border border-border text-text-primary"
          >
            <option value="POST">POST</option>
            <option value="PUT">PUT</option>
            <option value="PATCH">PATCH</option>
          </select>
        </div>
      )}

      {action.type === 'tag' && (
        <div className="flex-1 flex gap-2">
          <input
            value={action.tag || ''}
            onChange={e => onUpdate({ tag: e.target.value })}
            placeholder="Tag name"
            className="flex-1 px-2.5 py-1.5 text-xs rounded-lg bg-surface-raised border border-border text-text-primary placeholder:text-text-muted"
          />
          <input
            type="number"
            value={action.vlan || ''}
            onChange={e => onUpdate({ vlan: parseInt(e.target.value) || 0 })}
            placeholder="VLAN"
            className="w-20 px-2.5 py-1.5 text-xs rounded-lg bg-surface-raised border border-border text-text-primary placeholder:text-text-muted"
          />
        </div>
      )}

      {action.type === 'log' && (
        <span className="text-xs text-text-muted self-center">Logs rule match to server output</span>
      )}

      <button
        onClick={onRemove}
        className="p-1 rounded hover:bg-surface-overlay text-text-muted hover:text-danger transition-colors flex-shrink-0"
        title="Remove action"
      >
        <Trash2 className="w-3 h-3" />
      </button>
    </div>
  )
}
