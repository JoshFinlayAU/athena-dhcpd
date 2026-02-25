import { useState, useCallback } from 'react'
import { Save, RotateCcw, CheckCircle, AlertCircle } from 'lucide-react'
import { Card } from '@/components/Card'
import { useApi } from '@/hooks/useApi'
import { getConfigRaw, validateConfig, updateConfig } from '@/lib/api'

export default function Config() {
  const { data, loading } = useApi(useCallback(() => getConfigRaw(), []))
  const [content, setContent] = useState<string | null>(null)
  const [status, setStatus] = useState<{ type: 'success' | 'error'; message: string } | null>(null)
  const [saving, setSaving] = useState(false)

  const text = content ?? data?.config ?? ''
  const modified = content !== null && content !== data?.config

  const handleValidate = async () => {
    setStatus(null)
    try {
      const result = await validateConfig(text)
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
      await updateConfig(text)
      setStatus({ type: 'success', message: 'Configuration saved. Send SIGHUP to reload.' })
      setContent(null)
    } catch (e) {
      setStatus({ type: 'error', message: e instanceof Error ? e.message : 'Save failed' })
    } finally {
      setSaving(false)
    }
  }

  const handleReset = () => {
    setContent(null)
    setStatus(null)
  }

  return (
    <div className="p-6 space-y-4 max-w-7xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Configuration</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            Edit server configuration (TOML)
          </p>
        </div>
        <div className="flex items-center gap-2">
          {modified && (
            <button
              onClick={handleReset}
              className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
            >
              <RotateCcw className="w-3.5 h-3.5" /> Reset
            </button>
          )}
          <button
            onClick={handleValidate}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
          >
            <CheckCircle className="w-3.5 h-3.5" /> Validate
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !modified}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg bg-accent hover:bg-accent-hover text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            <Save className="w-3.5 h-3.5" /> {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
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

      {/* Editor */}
      <Card className="p-0 overflow-hidden">
        {loading ? (
          <div className="p-8 text-center text-text-muted">Loading configuration...</div>
        ) : (
          <textarea
            value={text}
            onChange={e => setContent(e.target.value)}
            spellCheck={false}
            className="w-full h-[calc(100vh-300px)] p-4 bg-transparent text-sm font-mono text-text-primary resize-none focus:outline-none leading-relaxed"
            placeholder="# TOML configuration..."
          />
        )}
      </Card>

      {modified && (
        <p className="text-xs text-warning">Unsaved changes</p>
      )}
    </div>
  )
}
