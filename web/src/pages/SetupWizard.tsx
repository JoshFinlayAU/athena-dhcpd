import { useState, useCallback } from 'react'
import { Server, Shield, Network, ChevronRight, ChevronLeft, Check, Upload, Eye, EyeOff, Loader2, AlertCircle, FileUp, X, FileText, Trash2 } from 'lucide-react'
import { Card } from '@/components/Card'
import { usePolling } from '@/hooks/useApi'
import {
  setupHA, setupConfig, setupComplete, getHAStatus,
  type SetupHARequest, type SubnetConfig, type PoolConfig, type DefaultsConfig,
  type ReservationConfig, type ConflictDetectionConfig, type DNSConfigType,
} from '@/lib/api'
import { parseReservations, detectFormat, FORMAT_INFO, type ImportFormat, type ParseResult } from '@/lib/reservationParser'

type Step = 'mode' | 'ha-role' | 'ha-config' | 'ha-standby-wait' | 'dhcp' | 'review'

const emptySubnet: SubnetConfig = {
  network: '', routers: [], dns_servers: [], domain_name: '',
  lease_time: '', pool: [{ range_start: '', range_end: '' }], reservation: [],
}

const emptyDefaults: DefaultsConfig = {
  lease_time: '24h', renewal_time: '12h', rebind_time: '21h',
  dns_servers: [], domain_name: '',
}

export default function SetupWizard() {
  const [step, setStep] = useState<Step>('mode')
  const [mode, setMode] = useState<'standalone' | 'ha'>('standalone')
  const [role, setRole] = useState<'primary' | 'secondary'>('primary')
  const [haConfig, setHaConfig] = useState<SetupHARequest>({
    mode: 'ha', peer_address: '', listen_address: '0.0.0.0:8068',
    tls_enabled: false, tls_ca: '', tls_cert: '', tls_key: '',
  })
  const [subnets, setSubnets] = useState<SubnetConfig[]>([{ ...emptySubnet }])
  const [defaults, setDefaults] = useState<DefaultsConfig>({ ...emptyDefaults })
  const [conflict, setConflict] = useState<ConflictDetectionConfig>({
    enabled: true, probe_strategy: 'sequential', probe_timeout: '500ms',
    max_probes_per_discover: 3, parallel_probe_count: 2, conflict_hold_time: '1h0m0s',
    max_conflict_count: 3, probe_cache_ttl: '10s', send_gratuitous_arp: true, icmp_fallback: true,
    probe_log_level: 'debug',
  })
  const [dns, setDns] = useState<DNSConfigType>({
    enabled: false, listen_udp: '0.0.0.0:53', listen_doh: '', domain: 'local', ttl: 60,
    register_leases: true, register_leases_ptr: true, forwarders: ['8.8.8.8:53', '1.1.1.1:53'],
    use_root_servers: false, cache_size: 10000, cache_ttl: '5m0s',
  })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  // HA status polling (only active during standby wait)
  const { data: haStatus } = usePolling(
    useCallback(() => getHAStatus(), []),
    step === 'ha-standby-wait' ? 2000 : null
  )

  const next = () => {
    setError('')
    if (step === 'mode') {
      if (mode === 'ha') setStep('ha-role')
      else setStep('dhcp')
    } else if (step === 'ha-role') {
      setStep('ha-config')
    } else if (step === 'ha-config') {
      handleSaveHA()
    } else if (step === 'dhcp') {
      setStep('review')
    }
  }

  const back = () => {
    setError('')
    if (step === 'ha-role') setStep('mode')
    else if (step === 'ha-config') setStep('ha-role')
    else if (step === 'dhcp') setStep(mode === 'ha' ? 'ha-config' : 'mode')
    else if (step === 'review') setStep('dhcp')
  }

  const handleSaveHA = async () => {
    setSaving(true)
    setError('')
    try {
      const req: SetupHARequest = { ...haConfig, mode: 'ha', role }
      await setupHA(req)
      if (role === 'secondary') {
        setStep('ha-standby-wait')
      } else {
        setStep('dhcp')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save HA config')
    } finally {
      setSaving(false)
    }
  }

  const handleFinish = async () => {
    setSaving(true)
    setError('')
    try {
      if (mode === 'standalone') {
        await setupHA({ mode: 'standalone' })
      }

      // Save DHCP config + conflict detection + DNS
      const cleanSubnets = subnets.filter(s => s.network)
      await setupConfig({
        defaults,
        subnets: cleanSubnets,
        conflict_detection: conflict,
        dns: dns.enabled ? dns : undefined,
      })

      // Mark setup complete
      await setupComplete()

      // Reload the page — server will restart with full services
      setTimeout(() => window.location.reload(), 2000)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to complete setup')
      setSaving(false)
    }
  }

  const handleStandbyComplete = async () => {
    setSaving(true)
    setError('')
    try {
      await setupComplete()
      setTimeout(() => window.location.reload(), 2000)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to complete setup')
      setSaving(false)
    }
  }

  return (
    <div className="min-h-screen bg-surface-base flex items-center justify-center p-6">
      <div className="w-full max-w-2xl">
        {/* Header */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-accent/15 mb-4">
            <Server className="w-8 h-8 text-accent" />
          </div>
          <h1 className="text-3xl font-bold">athena-dhcpd</h1>
          <p className="text-text-muted mt-2">Initial Setup</p>
        </div>

        {/* Progress */}
        <StepIndicator current={step} mode={mode} role={role} />

        {/* Error */}
        {error && (
          <div className="mb-4 p-3 rounded-lg bg-danger/10 border border-danger/20 flex items-center gap-2 text-sm text-danger">
            <AlertCircle className="w-4 h-4 flex-shrink-0" />
            {error}
          </div>
        )}

        {/* Steps */}
        {step === 'mode' && (
          <ModeStep mode={mode} onChange={setMode} />
        )}

        {step === 'ha-role' && (
          <RoleStep role={role} onChange={setRole} />
        )}

        {step === 'ha-config' && (
          <HAConfigStep config={haConfig} onChange={setHaConfig} role={role} />
        )}

        {step === 'ha-standby-wait' && (
          <StandbyWaitStep
            haStatus={haStatus}
            onComplete={handleStandbyComplete}
            saving={saving}
          />
        )}

        {step === 'dhcp' && (
          <DHCPStep
            subnets={subnets}
            onSubnetsChange={setSubnets}
            defaults={defaults}
            onDefaultsChange={setDefaults}
            conflict={conflict}
            onConflictChange={setConflict}
            dns={dns}
            onDnsChange={setDns}
          />
        )}

        {step === 'review' && (
          <ReviewStep mode={mode} role={role} subnets={subnets} defaults={defaults} haConfig={haConfig} conflict={conflict} dns={dns} />
        )}

        {/* Navigation */}
        {step !== 'ha-standby-wait' && (
          <div className="flex justify-between mt-6">
            <button
              onClick={back}
              disabled={step === 'mode'}
              className="flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
            >
              <ChevronLeft className="w-4 h-4" /> Back
            </button>

            {step === 'review' ? (
              <button
                onClick={handleFinish}
                disabled={saving}
                className="flex items-center gap-1.5 px-6 py-2.5 text-sm font-medium rounded-lg bg-success hover:bg-success/90 text-white transition-colors disabled:opacity-50"
              >
                {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Check className="w-4 h-4" />}
                {saving ? 'Starting...' : 'Finish Setup'}
              </button>
            ) : (
              <button
                onClick={next}
                disabled={saving}
                className="flex items-center gap-1.5 px-5 py-2.5 text-sm font-medium rounded-lg bg-accent hover:bg-accent-hover text-white transition-colors disabled:opacity-50"
              >
                {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : null}
                {step === 'ha-config' ? (saving ? 'Saving...' : 'Save & Continue') : 'Continue'}
                {!saving && <ChevronRight className="w-4 h-4" />}
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

// --- Step indicator ---

function StepIndicator({ current, mode, role }: { current: Step; mode: string; role: string }) {
  const steps: { key: Step; label: string }[] = [
    { key: 'mode', label: 'Mode' },
  ]
  if (mode === 'ha') {
    steps.push({ key: 'ha-role', label: 'Role' })
    steps.push({ key: 'ha-config', label: 'HA Config' })
    if (role === 'secondary') {
      steps.push({ key: 'ha-standby-wait', label: 'Sync' })
    }
  }
  if (role !== 'secondary' || mode !== 'ha') {
    steps.push({ key: 'dhcp', label: 'DHCP' })
    steps.push({ key: 'review', label: 'Review' })
  }

  const currentIdx = steps.findIndex(s => s.key === current)

  return (
    <div className="flex items-center justify-center gap-1 mb-6">
      {steps.map((s, i) => (
        <div key={s.key} className="flex items-center gap-1">
          <div className={`w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold transition-colors ${
            i < currentIdx ? 'bg-success text-white' :
            i === currentIdx ? 'bg-accent text-white' :
            'bg-surface-overlay text-text-muted'
          }`}>
            {i < currentIdx ? <Check className="w-3.5 h-3.5" /> : i + 1}
          </div>
          <span className={`text-xs font-medium hidden sm:inline ${
            i === currentIdx ? 'text-text-primary' : 'text-text-muted'
          }`}>{s.label}</span>
          {i < steps.length - 1 && <div className="w-6 h-px bg-border mx-1" />}
        </div>
      ))}
    </div>
  )
}

// --- Step: Mode selection ---

function ModeStep({ mode, onChange }: { mode: string; onChange: (m: 'standalone' | 'ha') => void }) {
  return (
    <Card className="space-y-4">
      <h2 className="text-lg font-semibold">Deployment Mode</h2>
      <p className="text-sm text-text-muted">
        Choose how this DHCP server will operate. You can change this later.
      </p>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <OptionCard
          selected={mode === 'standalone'}
          onClick={() => onChange('standalone')}
          icon={<Server className="w-6 h-6" />}
          title="Standalone"
          desc="Single server, no failover"
        />
        <OptionCard
          selected={mode === 'ha'}
          onClick={() => onChange('ha')}
          icon={<Shield className="w-6 h-6" />}
          title="High Availability"
          desc="Two nodes with automatic failover"
        />
      </div>
    </Card>
  )
}

// --- Step: HA Role ---

function RoleStep({ role, onChange }: { role: string; onChange: (r: 'primary' | 'secondary') => void }) {
  return (
    <Card className="space-y-4">
      <h2 className="text-lg font-semibold">HA Role</h2>
      <p className="text-sm text-text-muted">
        Which role does this node play in the HA pair?
      </p>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <OptionCard
          selected={role === 'primary'}
          onClick={() => onChange('primary')}
          icon={<Server className="w-6 h-6" />}
          title="Primary"
          desc="Active node — serves DHCP and holds the config"
        />
        <OptionCard
          selected={role === 'secondary'}
          onClick={() => onChange('secondary')}
          icon={<Shield className="w-6 h-6" />}
          title="Standby"
          desc="Receives config from primary — takes over on failure"
        />
      </div>
    </Card>
  )
}

// --- Step: HA Config ---

function HAConfigStep({ config, onChange, role }: {
  config: SetupHARequest; onChange: (c: SetupHARequest) => void; role: string
}) {
  const set = (k: keyof SetupHARequest, v: string | boolean) => onChange({ ...config, [k]: v })
  const [showKey, setShowKey] = useState(false)

  const readFile = (field: 'tls_ca' | 'tls_cert' | 'tls_key') => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.pem,.crt,.key,.cert'
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      const reader = new FileReader()
      reader.onload = () => set(field, reader.result as string)
      reader.readAsText(file)
    }
    input.click()
  }

  return (
    <Card className="space-y-5">
      <h2 className="text-lg font-semibold">HA Peer Configuration</h2>
      <p className="text-sm text-text-muted">
        {role === 'primary'
          ? 'Configure how this primary connects to the standby node.'
          : 'Configure how this standby connects to the primary node.'}
      </p>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label className="block text-xs font-medium text-text-muted mb-1">Peer Address</label>
          <input
            value={config.peer_address}
            onChange={e => set('peer_address', e.target.value)}
            placeholder="10.0.0.2:8068"
            className="w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface font-mono focus:outline-none focus:border-accent transition-colors"
          />
          <p className="text-[11px] text-text-muted mt-1">
            {role === 'primary' ? 'IP:port of the standby node' : 'IP:port of the primary node'}
          </p>
        </div>
        <div>
          <label className="block text-xs font-medium text-text-muted mb-1">Listen Address</label>
          <input
            value={config.listen_address}
            onChange={e => set('listen_address', e.target.value)}
            placeholder="0.0.0.0:8068"
            className="w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface font-mono focus:outline-none focus:border-accent transition-colors"
          />
          <p className="text-[11px] text-text-muted mt-1">Address this node listens on for HA</p>
        </div>
      </div>

      {/* TLS toggle */}
      <div className="pt-3 border-t border-border/50">
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.tls_enabled ?? false}
            onChange={e => set('tls_enabled', e.target.checked)}
            className="rounded border-border"
          />
          <span className="text-sm font-medium">Enable TLS encryption</span>
        </label>
      </div>

      {config.tls_enabled && (
        <div className="space-y-3 pl-1">
          <TLSField label="CA Certificate" value={config.tls_ca ?? ''} field="tls_ca"
            onChange={v => set('tls_ca', v)} onUpload={() => readFile('tls_ca')} />
          <TLSField label="Client Certificate" value={config.tls_cert ?? ''} field="tls_cert"
            onChange={v => set('tls_cert', v)} onUpload={() => readFile('tls_cert')} />
          <div className="relative">
            <TLSField label="Client Key" value={config.tls_key ?? ''} field="tls_key"
              onChange={v => set('tls_key', v)} onUpload={() => readFile('tls_key')}
              hidden={!showKey} />
            <button
              type="button"
              onClick={() => setShowKey(!showKey)}
              className="absolute top-0 right-0 p-1 text-text-muted hover:text-text-primary"
              title={showKey ? 'Hide' : 'Show'}
            >
              {showKey ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
            </button>
          </div>
        </div>
      )}
    </Card>
  )
}

function TLSField({ label, value, onChange, onUpload, hidden }: {
  label: string; value: string; field: string
  onChange: (v: string) => void; onUpload: () => void; hidden?: boolean
}) {
  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <label className="text-xs font-medium text-text-muted">{label}</label>
        <button type="button" onClick={onUpload}
          className="flex items-center gap-1 text-[11px] text-accent hover:text-accent-hover">
          <Upload className="w-3 h-3" /> Upload file
        </button>
      </div>
      <textarea
        value={hidden ? (value ? '••• (content set)' : '') : value}
        onChange={e => onChange(e.target.value)}
        readOnly={hidden && !!value}
        placeholder="Paste PEM content or upload a file..."
        rows={3}
        className="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface font-mono focus:outline-none focus:border-accent transition-colors resize-none"
      />
    </div>
  )
}

// --- Step: Standby wait ---

function StandbyWaitStep({ haStatus, onComplete, saving }: {
  haStatus: any; onComplete: () => void; saving: boolean
}) {
  const connected = haStatus?.peer_connected
  const state = haStatus?.state || 'waiting...'
  const lastError = haStatus?.last_error

  return (
    <Card className="space-y-4 text-center">
      <h2 className="text-lg font-semibold">Waiting for Primary</h2>
      <p className="text-sm text-text-muted">
        This node is configured as standby. It will receive its configuration from the primary node once connected.
      </p>

      <div className="py-6">
        <div className={`inline-flex items-center justify-center w-20 h-20 rounded-full mb-4 ${
          connected ? 'bg-success/15' : 'bg-warning/15'
        }`}>
          {connected
            ? <Check className="w-10 h-10 text-success" />
            : <Loader2 className="w-10 h-10 text-warning animate-spin" />}
        </div>
        <div className="space-y-1">
          <p className="text-sm font-medium">
            Peer: {connected ? <span className="text-success">Connected</span> : <span className="text-warning">Connecting...</span>}
          </p>
          <p className="text-xs text-text-muted font-mono">State: {state}</p>
          {!connected && lastError && (
            <p className="text-xs text-danger font-mono mt-2">{lastError}</p>
          )}
          {!connected && !lastError && haStatus?.enabled === false && (
            <p className="text-xs text-text-muted mt-2">Initializing connection...</p>
          )}
        </div>
      </div>

      {connected && (
        <button
          onClick={onComplete}
          disabled={saving}
          className="px-6 py-2.5 text-sm font-medium rounded-lg bg-success hover:bg-success/90 text-white transition-colors disabled:opacity-50"
        >
          {saving ? <Loader2 className="w-4 h-4 animate-spin inline mr-2" /> : null}
          {saving ? 'Starting...' : 'Complete Setup'}
        </button>
      )}
    </Card>
  )
}

// --- Step: DHCP config ---

function DHCPStep({ subnets, onSubnetsChange, defaults, onDefaultsChange, conflict, onConflictChange, dns, onDnsChange }: {
  subnets: SubnetConfig[]; onSubnetsChange: (s: SubnetConfig[]) => void
  defaults: DefaultsConfig; onDefaultsChange: (d: DefaultsConfig) => void
  conflict: ConflictDetectionConfig; onConflictChange: (c: ConflictDetectionConfig) => void
  dns: DNSConfigType; onDnsChange: (d: DNSConfigType) => void
}) {
  const updateSubnet = (i: number, k: keyof SubnetConfig, v: unknown) => {
    const n = [...subnets]
    n[i] = { ...n[i], [k]: v }
    onSubnetsChange(n)
  }

  const updatePool = (si: number, pi: number, k: keyof PoolConfig, v: string) => {
    const n = [...subnets]
    const pools = [...(n[si].pool || [])]
    pools[pi] = { ...pools[pi], [k]: v }
    n[si] = { ...n[si], pool: pools }
    onSubnetsChange(n)
  }

  const addSubnet = () => onSubnetsChange([...subnets, { ...emptySubnet, pool: [{ range_start: '', range_end: '' }] }])
  const removeSubnet = (i: number) => onSubnetsChange(subnets.filter((_, idx) => idx !== i))

  const addPool = (si: number) => {
    const n = [...subnets]
    n[si] = { ...n[si], pool: [...(n[si].pool || []), { range_start: '', range_end: '' }] }
    onSubnetsChange(n)
  }

  return (
    <Card className="space-y-5">
      <h2 className="text-lg font-semibold">DHCP Configuration</h2>

      {/* Global defaults */}
      <div className="p-4 rounded-lg border border-border/50 bg-surface-overlay/30 space-y-3">
        <h3 className="text-sm font-semibold text-text-muted">Global Defaults</h3>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <WizInput label="Lease Time" value={defaults.lease_time} mono
            onChange={v => onDefaultsChange({ ...defaults, lease_time: v })} placeholder="24h" />
          <WizInput label="Renewal Time (T1)" value={defaults.renewal_time} mono
            onChange={v => onDefaultsChange({ ...defaults, renewal_time: v })} placeholder="12h" />
          <WizInput label="Rebind Time (T2)" value={defaults.rebind_time} mono
            onChange={v => onDefaultsChange({ ...defaults, rebind_time: v })} placeholder="21h" />
        </div>
        <WizInput label="Domain Name" value={defaults.domain_name} mono
          onChange={v => onDefaultsChange({ ...defaults, domain_name: v })} placeholder="example.local" />
        <div>
          <label className="block text-xs font-medium text-text-muted mb-1">DNS Servers</label>
          <input
            value={defaults.dns_servers.join(', ')}
            onChange={e => onDefaultsChange({ ...defaults, dns_servers: e.target.value.split(',').map(s => s.trim()).filter(Boolean) })}
            placeholder="8.8.8.8, 8.8.4.4"
            className="w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface font-mono focus:outline-none focus:border-accent transition-colors"
          />
        </div>
      </div>

      {/* Subnets */}
      {subnets.map((sub, si) => (
        <div key={si} className="p-4 rounded-lg border border-accent/20 bg-accent/5 space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold flex items-center gap-2">
              <Network className="w-4 h-4 text-accent" />
              Subnet {si + 1}
            </h3>
            {subnets.length > 1 && (
              <button onClick={() => removeSubnet(si)} className="text-xs text-danger hover:text-danger/80">Remove</button>
            )}
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <WizInput label="Network (CIDR)" value={sub.network} mono
              onChange={v => updateSubnet(si, 'network', v)} placeholder="192.168.1.0/24" />
            <WizInput label="Interface" value={sub.interface ?? ''} mono
              onChange={v => updateSubnet(si, 'interface', v)} placeholder="eth0 (optional)" />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-muted mb-1">Routers (gateways)</label>
            <input
              value={(sub.routers || []).join(', ')}
              onChange={e => updateSubnet(si, 'routers', e.target.value.split(',').map(s => s.trim()).filter(Boolean))}
              placeholder="192.168.1.1"
              className="w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface font-mono focus:outline-none focus:border-accent transition-colors"
            />
          </div>

          {/* Pools */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-text-muted">Address Pools</span>
              <button onClick={() => addPool(si)} className="text-[11px] text-accent hover:text-accent-hover">+ Add Pool</button>
            </div>
            {(sub.pool || []).map((p, pi) => (
              <div key={pi} className="grid grid-cols-2 gap-2">
                <WizInput label={`Pool ${pi + 1} Start`} value={p.range_start} mono
                  onChange={v => updatePool(si, pi, 'range_start', v)} placeholder="192.168.1.100" />
                <WizInput label={`Pool ${pi + 1} End`} value={p.range_end} mono
                  onChange={v => updatePool(si, pi, 'range_end', v)} placeholder="192.168.1.200" />
              </div>
            ))}
          </div>

          {/* Reservations */}
          <ReservationSection
            reservations={sub.reservation || []}
            onChange={res => updateSubnet(si, 'reservation', res)}
          />
        </div>
      ))}

      <button
        onClick={addSubnet}
        className="w-full py-2.5 rounded-lg border-2 border-dashed border-border hover:border-accent/50 text-text-muted hover:text-accent text-sm transition-colors"
      >
        + Add another subnet
      </button>

      {/* Conflict Detection */}
      <div className="p-4 rounded-lg border border-border/50 bg-surface-overlay/30 space-y-3">
        <label className="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" checked={conflict.enabled} onChange={e => onConflictChange({ ...conflict, enabled: e.target.checked })}
            className="rounded border-border accent-accent" />
          <span className="text-sm font-semibold">IP Conflict Detection</span>
        </label>
        <p className="text-xs text-text-muted">Probe candidate IPs via ARP/ICMP before every DHCPOFFER to prevent conflicts</p>
        {conflict.enabled && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
            <WizInput label="Probe Timeout" value={conflict.probe_timeout} mono
              onChange={v => onConflictChange({ ...conflict, probe_timeout: v })} placeholder="500ms" />
            <div>
              <label className="block text-xs font-medium text-text-muted mb-1">Probe Strategy</label>
              <select value={conflict.probe_strategy}
                onChange={e => onConflictChange({ ...conflict, probe_strategy: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface focus:outline-none focus:border-accent">
                <option value="sequential">Sequential</option>
                <option value="parallel">Parallel</option>
              </select>
            </div>
          </div>
        )}
      </div>

      {/* DNS Proxy */}
      <div className="p-4 rounded-lg border border-border/50 bg-surface-overlay/30 space-y-3">
        <label className="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" checked={dns.enabled} onChange={e => onDnsChange({ ...dns, enabled: e.target.checked })}
            className="rounded border-border accent-accent" />
          <span className="text-sm font-semibold">Built-in DNS Proxy</span>
        </label>
        <p className="text-xs text-text-muted">Resolve DHCP clients by hostname, cache upstream queries, and filter domains</p>
        {dns.enabled && (
          <div className="space-y-3 pt-1">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <WizInput label="Listen Address" value={dns.listen_udp || ''} mono
                onChange={v => onDnsChange({ ...dns, listen_udp: v })} placeholder="0.0.0.0:53" />
              <WizInput label="Domain" value={dns.domain || ''} mono
                onChange={v => onDnsChange({ ...dns, domain: v })} placeholder="local" />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-muted mb-1">Upstream Forwarders</label>
              <input
                value={(dns.forwarders || []).join(', ')}
                onChange={e => onDnsChange({ ...dns, forwarders: e.target.value.split(',').map(s => s.trim()).filter(Boolean) })}
                placeholder="8.8.8.8:53, 1.1.1.1:53"
                className="w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface font-mono focus:outline-none focus:border-accent transition-colors"
              />
            </div>
            <div className="flex flex-wrap gap-4">
              <label className="flex items-center gap-1.5 text-xs cursor-pointer">
                <input type="checkbox" checked={dns.register_leases} onChange={e => onDnsChange({ ...dns, register_leases: e.target.checked })}
                  className="rounded border-border accent-accent" />
                Register lease hostnames
              </label>
              <label className="flex items-center gap-1.5 text-xs cursor-pointer">
                <input type="checkbox" checked={dns.register_leases_ptr} onChange={e => onDnsChange({ ...dns, register_leases_ptr: e.target.checked })}
                  className="rounded border-border accent-accent" />
                PTR records
              </label>
            </div>
          </div>
        )}
      </div>
    </Card>
  )
}

// --- Step: Review ---

function ReviewStep({ mode, role, subnets, defaults, haConfig, conflict, dns }: {
  mode: string; role: string; subnets: SubnetConfig[]; defaults: DefaultsConfig; haConfig: SetupHARequest
  conflict: ConflictDetectionConfig; dns: DNSConfigType
}) {
  const cleanSubnets = subnets.filter(s => s.network)

  return (
    <Card className="space-y-4">
      <h2 className="text-lg font-semibold">Review Configuration</h2>
      <p className="text-sm text-text-muted">Please review your settings before applying.</p>

      <div className="space-y-3">
        <ReviewSection title="Deployment">
          <ReviewItem label="Mode" value={mode === 'ha' ? `HA — ${role}` : 'Standalone'} />
          {mode === 'ha' && (
            <>
              <ReviewItem label="Peer Address" value={haConfig.peer_address || '—'} mono />
              <ReviewItem label="Listen Address" value={haConfig.listen_address || '—'} mono />
              <ReviewItem label="TLS" value={haConfig.tls_enabled ? 'Enabled' : 'Disabled'} />
            </>
          )}
        </ReviewSection>

        <ReviewSection title="Defaults">
          <ReviewItem label="Lease Time" value={defaults.lease_time} />
          <ReviewItem label="DNS Servers" value={defaults.dns_servers.join(', ') || '—'} mono />
          <ReviewItem label="Domain" value={defaults.domain_name || '—'} />
        </ReviewSection>

        {cleanSubnets.map((sub, i) => (
          <ReviewSection key={i} title={`Subnet: ${sub.network}`}>
            <ReviewItem label="Routers" value={(sub.routers || []).join(', ') || '—'} mono />
            {(sub.pool || []).map((p, pi) => (
              <ReviewItem key={pi} label={`Pool ${pi + 1}`} value={`${p.range_start} — ${p.range_end}`} mono />
            ))}
            {(sub.reservation || []).length > 0 && (
              <ReviewItem label="Reservations" value={`${(sub.reservation || []).length}`} />
            )}
          </ReviewSection>
        ))}

        <ReviewSection title="Conflict Detection">
          <ReviewItem label="Enabled" value={conflict.enabled ? 'Yes' : 'No'} />
          {conflict.enabled && (
            <>
              <ReviewItem label="Strategy" value={conflict.probe_strategy} />
              <ReviewItem label="Probe Timeout" value={conflict.probe_timeout} mono />
            </>
          )}
        </ReviewSection>

        {dns.enabled && (
          <ReviewSection title="DNS Proxy">
            <ReviewItem label="Listen" value={dns.listen_udp || '—'} mono />
            <ReviewItem label="Domain" value={dns.domain || '—'} />
            <ReviewItem label="Forwarders" value={(dns.forwarders || []).join(', ') || '—'} mono />
            <ReviewItem label="Register Leases" value={dns.register_leases ? 'Yes' : 'No'} />
          </ReviewSection>
        )}
      </div>
    </Card>
  )
}

function ReviewSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="p-3 rounded-lg bg-surface-overlay/50 border border-border/30">
      <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider mb-2">{title}</h4>
      <div className="space-y-1">{children}</div>
    </div>
  )
}

function ReviewItem({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex justify-between text-sm">
      <span className="text-text-muted">{label}</span>
      <span className={mono ? 'font-mono' : ''}>{value}</span>
    </div>
  )
}

// --- Reservation import section ---

function ReservationSection({ reservations, onChange }: {
  reservations: ReservationConfig[]; onChange: (r: ReservationConfig[]) => void
}) {
  const [showImport, setShowImport] = useState(false)

  const removeRes = (i: number) => onChange(reservations.filter((_, idx) => idx !== i))

  const handleImported = (imported: ReservationConfig[]) => {
    // Merge: skip duplicates by MAC
    const existing = new Set(reservations.map(r => r.mac.toLowerCase()))
    const newOnes = imported.filter(r => !existing.has(r.mac.toLowerCase()))
    onChange([...reservations, ...newOnes])
    setShowImport(false)
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-text-muted">Reservations</span>
        <div className="flex items-center gap-2">
          <button onClick={() => setShowImport(true)}
            className="flex items-center gap-1 text-[11px] text-accent hover:text-accent-hover">
            <FileUp className="w-3 h-3" /> Import
          </button>
          <button onClick={() => onChange([...reservations, { mac: '', ip: '', hostname: '' }])}
            className="text-[11px] text-accent hover:text-accent-hover">+ Add</button>
        </div>
      </div>

      {reservations.length > 0 && (
        <div className="space-y-1.5">
          {/* Header */}
          <div className="grid grid-cols-[1fr_1fr_1fr_24px] gap-2 px-1">
            <span className="text-[10px] font-semibold text-text-muted uppercase">MAC</span>
            <span className="text-[10px] font-semibold text-text-muted uppercase">IP</span>
            <span className="text-[10px] font-semibold text-text-muted uppercase">Hostname</span>
            <span />
          </div>
          {reservations.map((r, i) => (
            <div key={i} className="grid grid-cols-[1fr_1fr_1fr_24px] gap-2 items-center">
              <input value={r.mac} placeholder="aa:bb:cc:dd:ee:ff"
                onChange={e => { const n = [...reservations]; n[i] = { ...n[i], mac: e.target.value }; onChange(n) }}
                className="px-2 py-1.5 text-xs rounded border border-border bg-surface font-mono focus:outline-none focus:border-accent" />
              <input value={r.ip} placeholder="192.168.1.10"
                onChange={e => { const n = [...reservations]; n[i] = { ...n[i], ip: e.target.value }; onChange(n) }}
                className="px-2 py-1.5 text-xs rounded border border-border bg-surface font-mono focus:outline-none focus:border-accent" />
              <input value={r.hostname || ''} placeholder="hostname"
                onChange={e => { const n = [...reservations]; n[i] = { ...n[i], hostname: e.target.value }; onChange(n) }}
                className="px-2 py-1.5 text-xs rounded border border-border bg-surface font-mono focus:outline-none focus:border-accent" />
              <button onClick={() => removeRes(i)} className="text-text-muted hover:text-danger p-0.5" title="Remove">
                <Trash2 className="w-3.5 h-3.5" />
              </button>
            </div>
          ))}
          <p className="text-[10px] text-text-muted">{reservations.length} reservation{reservations.length !== 1 ? 's' : ''}</p>
        </div>
      )}

      {showImport && (
        <ImportReservationsModal
          onImport={handleImported}
          onClose={() => setShowImport(false)}
        />
      )}
    </div>
  )
}

// --- Import modal ---

function ImportReservationsModal({ onImport, onClose }: {
  onImport: (r: ReservationConfig[]) => void; onClose: () => void
}) {
  const [text, setText] = useState('')
  const [format, setFormat] = useState<ImportFormat>('auto')
  const [result, setResult] = useState<ParseResult | null>(null)
  const [dragOver, setDragOver] = useState(false)

  const doParse = (input: string, fmt: ImportFormat) => {
    if (!input.trim()) { setResult(null); return }
    setResult(parseReservations(input, fmt))
  }

  const handleTextChange = (v: string) => {
    setText(v)
    doParse(v, format)
  }

  const handleFormatChange = (f: ImportFormat) => {
    setFormat(f)
    doParse(text, f)
  }

  const handleFile = (file: File) => {
    const reader = new FileReader()
    reader.onload = () => {
      const content = reader.result as string
      setText(content)
      // Auto-detect format from file content
      const detected = detectFormat(content)
      setFormat(detected)
      doParse(content, detected)
    }
    reader.readAsText(file)
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer.files[0]
    if (file) handleFile(file)
  }

  const handleFileInput = () => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.csv,.json,.conf,.txt,.cfg,.rsc'
    input.onchange = () => {
      const file = input.files?.[0]
      if (file) handleFile(file)
    }
    input.click()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="bg-surface-base border border-border rounded-xl shadow-xl w-full max-w-2xl max-h-[80vh] flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-border">
          <h3 className="text-sm font-semibold flex items-center gap-2">
            <FileText className="w-4 h-4 text-accent" />
            Import Reservations
          </h3>
          <button onClick={onClose} className="text-text-muted hover:text-text-primary">
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Body */}
        <div className="p-4 space-y-4 overflow-y-auto flex-1">
          {/* Format selector */}
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-xs font-medium text-text-muted">Format:</span>
            {(Object.keys(FORMAT_INFO) as ImportFormat[]).map(f => (
              <button key={f} onClick={() => handleFormatChange(f)}
                className={`px-2.5 py-1 text-[11px] font-medium rounded-full transition-colors ${
                  format === f
                    ? 'bg-accent text-white'
                    : 'bg-surface-overlay text-text-muted hover:text-text-primary'
                }`}>
                {FORMAT_INFO[f].label}
              </button>
            ))}
          </div>

          {/* Example hint */}
          {format !== 'auto' && FORMAT_INFO[format].example && (
            <div className="p-2.5 rounded-lg bg-surface-overlay/50 border border-border/30">
              <p className="text-[10px] font-semibold text-text-muted uppercase mb-1">Example</p>
              <pre className="text-[11px] font-mono text-text-muted whitespace-pre-wrap">{FORMAT_INFO[format].example}</pre>
            </div>
          )}

          {/* Input area */}
          <div
            onDragOver={e => { e.preventDefault(); setDragOver(true) }}
            onDragLeave={() => setDragOver(false)}
            onDrop={handleDrop}
            className={`relative rounded-lg border-2 border-dashed transition-colors ${
              dragOver ? 'border-accent bg-accent/5' : 'border-border'
            }`}
          >
            <textarea
              value={text}
              onChange={e => handleTextChange(e.target.value)}
              placeholder="Paste reservation data here, or drag & drop a file..."
              rows={10}
              className="w-full px-3 py-2.5 text-xs font-mono bg-transparent rounded-lg focus:outline-none resize-none"
            />
            {!text && (
              <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
                <div className="text-center">
                  <FileUp className="w-8 h-8 text-text-muted/30 mx-auto mb-2" />
                  <p className="text-xs text-text-muted/50">Drag & drop or paste</p>
                </div>
              </div>
            )}
          </div>

          <button onClick={handleFileInput}
            className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover font-medium">
            <Upload className="w-3.5 h-3.5" /> Upload file
          </button>

          {/* Parse results */}
          {result && (
            <div className="space-y-2">
              {result.errors.length > 0 && (
                <div className="p-2.5 rounded-lg bg-danger/10 border border-danger/20">
                  <p className="text-[11px] font-semibold text-danger mb-1">
                    {result.errors.length} warning{result.errors.length !== 1 ? 's' : ''}
                  </p>
                  <ul className="space-y-0.5">
                    {result.errors.slice(0, 5).map((e, i) => (
                      <li key={i} className="text-[10px] text-danger/80">{e}</li>
                    ))}
                    {result.errors.length > 5 && (
                      <li className="text-[10px] text-danger/60">...and {result.errors.length - 5} more</li>
                    )}
                  </ul>
                </div>
              )}

              {result.reservations.length > 0 && (
                <div className="p-2.5 rounded-lg bg-success/10 border border-success/20">
                  <p className="text-[11px] font-semibold text-success mb-1.5">
                    {result.reservations.length} reservation{result.reservations.length !== 1 ? 's' : ''} found
                    <span className="font-normal text-success/70 ml-1">(format: {FORMAT_INFO[result.format].label})</span>
                  </p>
                  {/* Preview table */}
                  <div className="max-h-40 overflow-y-auto">
                    <table className="w-full text-[10px] font-mono">
                      <thead>
                        <tr className="text-left text-text-muted">
                          <th className="pb-1 pr-2">MAC</th>
                          <th className="pb-1 pr-2">IP</th>
                          <th className="pb-1">Hostname</th>
                        </tr>
                      </thead>
                      <tbody>
                        {result.reservations.slice(0, 20).map((r, i) => (
                          <tr key={i} className="text-text-primary/80">
                            <td className="pr-2 py-0.5">{r.mac}</td>
                            <td className="pr-2 py-0.5">{r.ip}</td>
                            <td className="py-0.5">{r.hostname || '—'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                    {result.reservations.length > 20 && (
                      <p className="text-[10px] text-text-muted mt-1">
                        ...and {result.reservations.length - 20} more
                      </p>
                    )}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between p-4 border-t border-border">
          <button onClick={onClose}
            className="px-4 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors">
            Cancel
          </button>
          <button
            onClick={() => result && onImport(result.reservations)}
            disabled={!result || result.reservations.length === 0}
            className="flex items-center gap-1.5 px-4 py-2 text-xs font-medium rounded-lg bg-accent hover:bg-accent-hover text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Check className="w-3.5 h-3.5" />
            Import {result?.reservations.length || 0} reservations
          </button>
        </div>
      </div>
    </div>
  )
}

// --- Shared components ---

function OptionCard({ selected, onClick, icon, title, desc }: {
  selected: boolean; onClick: () => void; icon: React.ReactNode; title: string; desc: string
}) {
  return (
    <button
      onClick={onClick}
      className={`p-4 rounded-xl border-2 text-left transition-all ${
        selected
          ? 'border-accent bg-accent/5 shadow-sm'
          : 'border-border hover:border-accent/30 bg-surface'
      }`}
    >
      <div className={`mb-2 ${selected ? 'text-accent' : 'text-text-muted'}`}>{icon}</div>
      <p className="font-semibold text-sm">{title}</p>
      <p className="text-xs text-text-muted mt-0.5">{desc}</p>
    </button>
  )
}

function WizInput({ label, value, onChange, placeholder, mono }: {
  label: string; value: string; onChange: (v: string) => void; placeholder?: string; mono?: boolean
}) {
  return (
    <div>
      <label className="block text-xs font-medium text-text-muted mb-1">{label}</label>
      <input
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        className={`w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface focus:outline-none focus:border-accent transition-colors ${mono ? 'font-mono' : ''}`}
      />
    </div>
  )
}
