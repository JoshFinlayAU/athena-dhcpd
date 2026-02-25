import { useState, useCallback } from 'react'
import { RefreshCw, Search, Shield, ShieldOff, Database, Clock, AlertCircle, CheckCircle, Trash2 } from 'lucide-react'
import { Card } from '@/components/Card'
import { useApi } from '@/hooks/useApi'
import { getDNSStats, getDNSLists, refreshDNSLists, testDNSDomain, flushDNSCache } from '@/lib/api'
import type { DNSStats, DNSListStatus, DNSTestResult } from '@/lib/api'

function StatCard({ label, value, icon: Icon, color }: {
  label: string
  value: string | number
  icon: React.ElementType
  color: string
}) {
  return (
    <Card className="p-4 flex items-center gap-4">
      <div className={`w-10 h-10 rounded-xl flex items-center justify-center ${color}`}>
        <Icon className="w-5 h-5" />
      </div>
      <div>
        <p className="text-2xl font-bold tabular-nums">{value}</p>
        <p className="text-xs text-text-muted">{label}</p>
      </div>
    </Card>
  )
}

function formatTime(ts: string): string {
  if (!ts || ts === '0001-01-01T00:00:00Z') return 'Never'
  const d = new Date(ts)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  if (diff < 60000) return 'Just now'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`
  return d.toLocaleDateString()
}

function formatNumber(n: number): string {
  if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
  return n.toString()
}

export default function DNSFilteringPage() {
  const { data: stats, refetch: refetchStats } = useApi<DNSStats>(useCallback(() => getDNSStats(), []))
  const { data: listsData, refetch: refetchLists } = useApi(useCallback(() => getDNSLists(), []))
  const [testDomain, setTestDomain] = useState('')
  const [testResult, setTestResult] = useState<DNSTestResult | null>(null)
  const [testing, setTesting] = useState(false)
  const [refreshing, setRefreshing] = useState<string | null>(null)
  const [flushing, setFlushing] = useState(false)
  const [status, setStatus] = useState<{ type: 'success' | 'error'; message: string } | null>(null)

  const lists: DNSListStatus[] = listsData?.lists || []

  const handleTest = async () => {
    if (!testDomain.trim()) return
    setTesting(true)
    setTestResult(null)
    setStatus(null)
    try {
      const result = await testDNSDomain(testDomain.trim())
      setTestResult(result)
    } catch (e) {
      setStatus({ type: 'error', message: e instanceof Error ? e.message : 'Test failed' })
    } finally {
      setTesting(false)
    }
  }

  const handleRefresh = async (name?: string) => {
    setRefreshing(name || 'all')
    setStatus(null)
    try {
      await refreshDNSLists(name)
      setStatus({ type: 'success', message: name ? `Refreshed "${name}"` : 'All lists refreshed' })
      refetchLists()
      refetchStats()
    } catch (e) {
      setStatus({ type: 'error', message: e instanceof Error ? e.message : 'Refresh failed' })
    } finally {
      setRefreshing(null)
    }
  }

  const handleFlushCache = async () => {
    setFlushing(true)
    setStatus(null)
    try {
      await flushDNSCache()
      setStatus({ type: 'success', message: 'DNS cache flushed' })
      refetchStats()
    } catch (e) {
      setStatus({ type: 'error', message: e instanceof Error ? e.message : 'Flush failed' })
    } finally {
      setFlushing(false)
    }
  }

  return (
    <div className="p-6 space-y-6 max-w-7xl">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">DNS Filtering</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            Block ads, malware, and unwanted domains with filter lists
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={handleFlushCache} disabled={flushing}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors disabled:opacity-40">
            <Trash2 className="w-3.5 h-3.5" /> {flushing ? 'Flushing...' : 'Flush Cache'}
          </button>
          <button onClick={() => handleRefresh()} disabled={refreshing !== null}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg bg-accent hover:bg-accent-hover text-white disabled:opacity-40 transition-colors">
            <RefreshCw className={`w-3.5 h-3.5 ${refreshing === 'all' ? 'animate-spin' : ''}`} />
            {refreshing === 'all' ? 'Refreshing...' : 'Refresh All Lists'}
          </button>
        </div>
      </div>

      {/* Status */}
      {status && (
        <div className={`flex items-center gap-2 px-4 py-2.5 rounded-lg border text-sm ${
          status.type === 'success'
            ? 'border-success/30 bg-success/5 text-success'
            : 'border-danger/30 bg-danger/5 text-danger'
        }`}>
          {status.type === 'success' ? <CheckCircle className="w-4 h-4" /> : <AlertCircle className="w-4 h-4" />}
          <span className="text-xs">{status.message}</span>
        </div>
      )}

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <StatCard label="Blocked Domains" value={formatNumber(stats.blocked_domains)} icon={ShieldOff} color="bg-danger/10 text-danger" />
          <StatCard label="Filter Lists" value={stats.filter_lists} icon={Shield} color="bg-accent/10 text-accent" />
          <StatCard label="Cache Entries" value={formatNumber(stats.cache_entries)} icon={Database} color="bg-info/10 text-info" />
          <StatCard label="Zone Records" value={stats.zone_records} icon={Clock} color="bg-success/10 text-success" />
        </div>
      )}

      {/* Domain Test */}
      <Card className="p-5">
        <h3 className="text-sm font-semibold mb-3">Test Domain</h3>
        <p className="text-xs text-text-muted mb-3">Check if a domain would be blocked by your filter lists</p>
        <div className="flex gap-2">
          <div className="flex-1 relative">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
            <input
              type="text"
              value={testDomain}
              onChange={e => setTestDomain(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleTest()}
              placeholder="ads.example.com"
              className="w-full pl-9 pr-3 py-2.5 text-sm rounded-lg border border-border bg-surface hover:border-border-hover focus:border-accent focus:ring-1 focus:ring-accent/30 outline-none transition-colors font-mono"
            />
          </div>
          <button onClick={handleTest} disabled={testing || !testDomain.trim()}
            className="px-4 py-2.5 text-sm font-medium rounded-lg bg-accent hover:bg-accent-hover text-white disabled:opacity-40 transition-colors">
            {testing ? 'Testing...' : 'Test'}
          </button>
        </div>

        {testResult && (
          <div className={`mt-4 p-4 rounded-lg border ${
            testResult.blocked
              ? 'border-danger/30 bg-danger/5'
              : 'border-success/30 bg-success/5'
          }`}>
            <div className="flex items-center gap-2 mb-2">
              {testResult.blocked ? (
                <ShieldOff className="w-5 h-5 text-danger" />
              ) : (
                <CheckCircle className="w-5 h-5 text-success" />
              )}
              <span className={`text-sm font-semibold ${testResult.blocked ? 'text-danger' : 'text-success'}`}>
                {testResult.blocked ? 'BLOCKED' : 'ALLOWED'}
              </span>
              <span className="text-sm font-mono text-text-secondary">{testResult.domain}</span>
            </div>
            {testResult.blocked && (
              <div className="text-xs text-text-muted space-y-1 ml-7">
                <p>Action: <span className="font-mono text-text-secondary">{testResult.action}</span></p>
                <p>Matched by: <span className="font-semibold text-text-secondary">{testResult.list}</span></p>
              </div>
            )}
            {testResult.matches && testResult.matches.length > 0 && (
              <div className="mt-2 ml-7">
                <p className="text-[10px] text-text-muted uppercase tracking-wider mb-1">All matching lists:</p>
                <div className="flex flex-wrap gap-1.5">
                  {testResult.matches.map((m, i) => (
                    <span key={i} className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${
                      m.type === 'block' ? 'bg-danger/10 text-danger' : 'bg-success/10 text-success'
                    }`}>
                      {m.list} ({m.type})
                    </span>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </Card>

      {/* Filter Lists */}
      <div>
        <h3 className="text-sm font-semibold mb-3">Filter Lists</h3>
        {lists.length === 0 ? (
          <Card className="p-8 text-center">
            <Shield className="w-8 h-8 text-text-muted mx-auto mb-2" />
            <p className="text-sm text-text-muted">No filter lists configured</p>
            <p className="text-xs text-text-muted mt-1">Add blocklists in the DNS Proxy config section</p>
          </Card>
        ) : (
          <div className="space-y-3">
            {lists.map((list, i) => (
              <Card key={i} className="p-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className={`w-8 h-8 rounded-lg flex items-center justify-center ${
                      list.type === 'block' ? 'bg-danger/10' : 'bg-success/10'
                    }`}>
                      {list.type === 'block'
                        ? <ShieldOff className="w-4 h-4 text-danger" />
                        : <Shield className="w-4 h-4 text-success" />
                      }
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold">{list.name}</span>
                        <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${
                          list.type === 'block' ? 'bg-danger/10 text-danger' : 'bg-success/10 text-success'
                        }`}>{list.type}</span>
                        <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-surface-overlay text-text-muted font-medium">{list.format}</span>
                        <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-surface-overlay text-text-muted font-medium">{list.action}</span>
                        {!list.enabled && (
                          <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-warning/10 text-warning font-medium">disabled</span>
                        )}
                      </div>
                      <p className="text-[11px] text-text-muted font-mono mt-0.5 truncate max-w-lg">{list.url}</p>
                    </div>
                  </div>

                  <div className="flex items-center gap-4">
                    <div className="text-right">
                      <p className="text-sm font-bold tabular-nums">{formatNumber(list.domain_count)}</p>
                      <p className="text-[10px] text-text-muted">domains</p>
                    </div>
                    <div className="text-right min-w-[80px]">
                      <p className="text-xs text-text-secondary">{formatTime(list.last_refresh)}</p>
                      <p className="text-[10px] text-text-muted">last refresh</p>
                    </div>
                    <button
                      onClick={() => handleRefresh(list.name)}
                      disabled={refreshing !== null}
                      className="p-2 rounded-lg border border-border hover:bg-surface-overlay transition-colors disabled:opacity-40"
                      title={`Refresh ${list.name}`}
                    >
                      <RefreshCw className={`w-3.5 h-3.5 ${refreshing === list.name ? 'animate-spin' : ''}`} />
                    </button>
                  </div>
                </div>

                {list.last_error && (
                  <div className="mt-3 flex items-center gap-2 px-3 py-2 rounded-lg bg-danger/5 border border-danger/20">
                    <AlertCircle className="w-3.5 h-3.5 text-danger flex-shrink-0" />
                    <span className="text-xs text-danger truncate">{list.last_error}</span>
                  </div>
                )}

                <div className="mt-3 flex items-center gap-4 text-[10px] text-text-muted">
                  <span>Refresh: <span className="font-mono text-text-secondary">{list.refresh_interval}</span></span>
                  <span>Next: <span className="text-text-secondary">{formatTime(list.next_refresh)}</span></span>
                </div>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
