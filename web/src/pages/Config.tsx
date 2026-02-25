import { useState, useCallback, useEffect, useRef } from 'react'
import { Save, RotateCcw, CheckCircle, AlertCircle, SlidersHorizontal, FileCode } from 'lucide-react'
import { Card } from '@/components/Card'
import { useApi } from '@/hooks/useApi'
import { getConfigRaw, validateConfig, updateConfig } from '@/lib/api'
import { parse, stringify } from 'smol-toml'
import type { Config } from '@/lib/configTypes'
import { mergeWithDefaults } from '@/lib/configTypes'
import { cn } from '@/lib/utils'

import ServerSection from '@/components/config/ServerSection'
import ConflictSection from '@/components/config/ConflictSection'
import HASection from '@/components/config/HASection'
import HooksSection from '@/components/config/HooksSection'
import DDNSSection from '@/components/config/DDNSSection'
import DNSSection from '@/components/config/DNSSection'
import DefaultsSection from '@/components/config/DefaultsSection'
import APISection from '@/components/config/APISection'
import SubnetsSection from '@/components/config/SubnetsSection'

type Tab = 'visual' | 'toml'

export default function ConfigPage() {
  const { data, loading } = useApi(useCallback(() => getConfigRaw(), []))
  const [tab, setTab] = useState<Tab>('visual')
  const [config, setConfig] = useState<Config | null>(null)
  const [rawContent, setRawContent] = useState<string | null>(null)
  const [status, setStatus] = useState<{ type: 'success' | 'error'; message: string } | null>(null)
  const [saving, setSaving] = useState(false)
  const [parseError, setParseError] = useState<string | null>(null)
  const initialToml = useRef<string>('')

  // Parse incoming TOML into structured config
  useEffect(() => {
    if (!data?.config) return
    initialToml.current = data.config
    try {
      const parsed = parse(data.config) as Record<string, unknown>
      setConfig(mergeWithDefaults(parsed))
      setParseError(null)
    } catch (e) {
      setParseError(e instanceof Error ? e.message : 'Failed to parse TOML')
    }
  }, [data])

  // Serialize config back to TOML
  const configToToml = (cfg: Config): string => {
    try {
      // Clean up empty arrays and objects before serializing
      const clean = JSON.parse(JSON.stringify(cfg, (_k, v) => {
        if (v === '') return undefined
        if (Array.isArray(v) && v.length === 0) return undefined
        if (typeof v === 'object' && v !== null && !Array.isArray(v) && Object.keys(v).length === 0) return undefined
        return v
      }))
      return stringify(clean)
    } catch {
      return '# Error serializing config\n'
    }
  }

  // Current TOML text (from visual editor or raw editor)
  const getCurrentToml = (): string => {
    if (tab === 'toml' && rawContent !== null) return rawContent
    if (config) return configToToml(config)
    return data?.config || ''
  }

  // Check if modified
  const isModified = (): boolean => {
    const current = getCurrentToml()
    return current !== initialToml.current
  }

  // Switch tabs — sync state between visual and raw
  const switchTab = (newTab: Tab) => {
    if (newTab === tab) return
    if (newTab === 'toml') {
      // Visual → TOML: serialize current config state
      if (config) setRawContent(configToToml(config))
    } else {
      // TOML → Visual: parse raw content
      const toml = rawContent ?? data?.config ?? ''
      try {
        const parsed = parse(toml) as Record<string, unknown>
        setConfig(mergeWithDefaults(parsed))
        setParseError(null)
      } catch (e) {
        setParseError(e instanceof Error ? e.message : 'Failed to parse TOML')
      }
    }
    setTab(newTab)
  }

  const handleValidate = async () => {
    setStatus(null)
    try {
      const result = await validateConfig(getCurrentToml())
      if (result.valid) {
        setStatus({ type: 'success', message: 'Configuration is valid' })
      } else {
        setStatus({ type: 'error', message: result.errors?.join('\n') || 'Validation failed' })
      }
    } catch (e) {
      setStatus({ type: 'error', message: e instanceof Error ? e.message : 'Validation error' })
    }
  }

  const handleSave = async () => {
    setSaving(true)
    setStatus(null)
    try {
      const toml = getCurrentToml()
      await updateConfig(toml)
      initialToml.current = toml
      setRawContent(null)
      setStatus({ type: 'success', message: 'Configuration saved. Send SIGHUP to reload.' })
    } catch (e) {
      setStatus({ type: 'error', message: e instanceof Error ? e.message : 'Save failed' })
    } finally {
      setSaving(false)
    }
  }

  const handleReset = () => {
    if (!data?.config) return
    try {
      const parsed = parse(data.config) as Record<string, unknown>
      setConfig(mergeWithDefaults(parsed))
      setRawContent(null)
      setParseError(null)
    } catch (e) {
      setParseError(e instanceof Error ? e.message : 'Failed to parse TOML')
    }
    setStatus(null)
  }

  const modified = isModified()

  return (
    <div className="p-6 space-y-4 max-w-7xl">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Configuration</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            {tab === 'visual' ? 'Edit all settings with structured forms' : 'Edit raw TOML configuration'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {modified && (
            <button onClick={handleReset}
              className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors">
              <RotateCcw className="w-3.5 h-3.5" /> Reset
            </button>
          )}
          <button onClick={handleValidate}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors">
            <CheckCircle className="w-3.5 h-3.5" /> Validate
          </button>
          <button onClick={handleSave} disabled={saving || !modified}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg bg-accent hover:bg-accent-hover text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors">
            <Save className="w-3.5 h-3.5" /> {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>

      {/* Tab switcher */}
      <div className="flex items-center gap-1 p-1 bg-surface-raised rounded-xl border border-border w-fit">
        <button
          onClick={() => switchTab('visual')}
          className={cn(
            'flex items-center gap-2 px-4 py-2 text-xs font-medium rounded-lg transition-all',
            tab === 'visual' ? 'bg-accent text-white shadow-sm' : 'text-text-secondary hover:text-text-primary hover:bg-surface-overlay'
          )}
        >
          <SlidersHorizontal className="w-3.5 h-3.5" /> Visual Editor
        </button>
        <button
          onClick={() => switchTab('toml')}
          className={cn(
            'flex items-center gap-2 px-4 py-2 text-xs font-medium rounded-lg transition-all',
            tab === 'toml' ? 'bg-accent text-white shadow-sm' : 'text-text-secondary hover:text-text-primary hover:bg-surface-overlay'
          )}
        >
          <FileCode className="w-3.5 h-3.5" /> Raw TOML
        </button>
      </div>

      {/* Status bar */}
      {status && (
        <div className={`flex items-center gap-2 px-4 py-2.5 rounded-lg border text-sm ${
          status.type === 'success'
            ? 'border-success/30 bg-success/5 text-success'
            : 'border-danger/30 bg-danger/5 text-danger'
        }`}>
          {status.type === 'success' ? <CheckCircle className="w-4 h-4" /> : <AlertCircle className="w-4 h-4" />}
          <pre className="whitespace-pre-wrap text-xs">{status.message}</pre>
        </div>
      )}

      {/* Parse error */}
      {parseError && tab === 'visual' && (
        <div className="flex items-center gap-2 px-4 py-2.5 rounded-lg border border-danger/30 bg-danger/5 text-danger text-sm">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <div>
            <p className="text-xs font-medium">TOML parse error — switch to Raw TOML to fix</p>
            <pre className="whitespace-pre-wrap text-xs mt-1 opacity-75">{parseError}</pre>
          </div>
        </div>
      )}

      {/* Content */}
      {loading ? (
        <Card className="p-8 text-center text-text-muted">Loading configuration...</Card>
      ) : tab === 'visual' && config && !parseError ? (
        <div className="space-y-4">
          <ServerSection value={config.server} onChange={v => setConfig({ ...config, server: v })} />
          <DefaultsSection value={config.defaults} onChange={v => setConfig({ ...config, defaults: v })} />
          <SubnetsSection value={config.subnet || []} onChange={v => setConfig({ ...config, subnet: v })} />
          <ConflictSection value={config.conflict_detection} onChange={v => setConfig({ ...config, conflict_detection: v })} />
          <HASection value={config.ha} onChange={v => setConfig({ ...config, ha: v })} />
          <DNSSection value={config.dns} onChange={v => setConfig({ ...config, dns: v })} />
          <DDNSSection value={config.ddns} onChange={v => setConfig({ ...config, ddns: v })} />
          <HooksSection value={config.hooks} onChange={v => setConfig({ ...config, hooks: v })} />
          <APISection value={config.api} onChange={v => setConfig({ ...config, api: v })} />
        </div>
      ) : (
        <Card className="p-0 overflow-hidden">
          <textarea
            value={tab === 'toml' ? (rawContent ?? data?.config ?? '') : (data?.config ?? '')}
            onChange={e => setRawContent(e.target.value)}
            spellCheck={false}
            className="w-full h-[calc(100vh-340px)] p-4 bg-transparent text-sm font-mono text-text-primary resize-none focus:outline-none leading-relaxed"
            placeholder="# TOML configuration..."
          />
        </Card>
      )}

      {modified && (
        <p className="text-xs text-warning">Unsaved changes</p>
      )}
    </div>
  )
}
