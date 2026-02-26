import { useState, useCallback, useMemo } from 'react'
import { useApi } from '@/hooks/useApi'
import { Card } from '@/components/Card'
import { TextInput } from '@/components/FormFields'
import { Table, THead, TH, TD, TR, EmptyRow } from '@/components/Table'
import { Fingerprint, Search, Monitor, Smartphone, Printer, Wifi, Camera, HelpCircle, Server } from 'lucide-react'
import {
  v2GetFingerprints, v2GetFingerprintStats,
  type DeviceFingerprint,
} from '@/lib/api'

const typeIcons: Record<string, typeof Monitor> = {
  computer: Monitor,
  phone: Smartphone,
  printer: Printer,
  network: Wifi,
  camera: Camera,
  embedded: Server,
}

function deviceIcon(type_: string) {
  const Icon = typeIcons[type_] || HelpCircle
  return <Icon className="w-4 h-4" />
}

function confidenceBadge(confidence: number) {
  const color = confidence >= 80 ? 'text-success' : confidence >= 50 ? 'text-warning' : 'text-text-muted'
  return <span className={`text-xs font-medium ${color}`}>{confidence}%</span>
}

function formatTime(ts: string) {
  try { return new Date(ts).toLocaleString() } catch { return ts }
}

export default function Fingerprints() {
  const { data: devices, loading } = useApi(useCallback(() => v2GetFingerprints(), []))
  const { data: stats } = useApi(useCallback(() => v2GetFingerprintStats(), []))
  const [filter, setFilter] = useState('')

  const filtered = useMemo(() => {
    if (!devices) return []
    if (!filter) return devices
    const q = filter.toLowerCase()
    return devices.filter(d =>
      d.mac.toLowerCase().includes(q) ||
      d.hostname.toLowerCase().includes(q) ||
      d.device_type.toLowerCase().includes(q) ||
      d.os.toLowerCase().includes(q) ||
      d.vendor_class.toLowerCase().includes(q) ||
      (d.device_name || '').toLowerCase().includes(q)
    )
  }, [devices, filter])

  return (
    <div className="p-6 max-w-7xl space-y-4">
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2">
          <Fingerprint className="w-6 h-6" /> Device Fingerprints
        </h1>
        <p className="text-sm text-text-secondary mt-0.5">
          DHCP fingerprinting and device classification
          {stats && <span className="text-text-muted ml-1">({stats.total_devices} devices known)</span>}
        </p>
      </div>

      {/* Stats cards */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-3">
          {Object.entries(stats.by_type)
            .sort(([, a], [, b]) => b - a)
            .map(([type_, count]) => (
              <Card key={type_} className="p-3 flex items-center gap-2">
                {deviceIcon(type_)}
                <div>
                  <div className="text-lg font-bold">{count}</div>
                  <div className="text-xs text-text-muted capitalize">{type_}</div>
                </div>
              </Card>
            ))}
        </div>
      )}

      {/* Filter */}
      <Card className="p-3">
        <div className="flex items-center gap-2">
          <Search className="w-4 h-4 text-text-muted" />
          <TextInput
            value={filter}
            onChange={setFilter}
            placeholder="Filter by MAC, hostname, OS, vendor class, device type..."
          />
          <span className="text-xs text-text-muted whitespace-nowrap">{filtered.length} devices</span>
        </div>
      </Card>

      {/* Device table */}
      <Card className="overflow-hidden">
        <Table>
          <THead>
            <tr>
              <TH>Device</TH>
              <TH>MAC</TH>
              <TH>Hostname</TH>
              <TH>OS</TH>
              <TH>Vendor Class</TH>
              <TH>Confidence</TH>
              <TH>Last Seen</TH>
            </tr>
          </THead>
          <tbody>
            {filtered.length === 0 ? (
              <EmptyRow cols={7} message={loading ? 'Loading...' : 'No devices found'} />
            ) : (
              filtered.map(d => <DeviceRow key={d.mac} device={d} />)
            )}
          </tbody>
        </Table>
      </Card>
    </div>
  )
}

function DeviceRow({ device: d }: { device: DeviceFingerprint }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <>
      <TR onClick={() => setExpanded(!expanded)} className="cursor-pointer">
        <TD>
          <div className="flex items-center gap-2">
            {deviceIcon(d.device_type)}
            <div>
              <div className="text-xs font-medium capitalize">{d.device_type}</div>
              {d.device_name && <div className="text-[10px] text-text-muted">{d.device_name}</div>}
            </div>
          </div>
        </TD>
        <TD><span className="font-mono text-xs">{d.mac}</span></TD>
        <TD><span className="text-xs">{d.hostname || '-'}</span></TD>
        <TD><span className="text-xs">{d.os || '-'}</span></TD>
        <TD><span className="text-xs font-mono truncate max-w-[200px] block">{d.vendor_class || '-'}</span></TD>
        <TD>{confidenceBadge(d.confidence)}</TD>
        <TD><span className="text-xs text-text-muted">{formatTime(d.last_seen)}</span></TD>
      </TR>
      {expanded && (
        <tr>
          <td colSpan={7} className="px-4 py-3 bg-surface/50 border-b border-border">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-xs">
              <div><span className="text-text-muted">OUI:</span> {d.oui || '-'}</div>
              <div><span className="text-text-muted">Fingerprint Hash:</span> <span className="font-mono">{d.fingerprint_hash}</span></div>
              <div><span className="text-text-muted">Param List:</span> <span className="font-mono">{d.param_list || '-'}</span></div>
              <div><span className="text-text-muted">Source:</span> {d.source}</div>
              <div><span className="text-text-muted">First Seen:</span> {formatTime(d.first_seen)}</div>
              <div><span className="text-text-muted">Last Seen:</span> {formatTime(d.last_seen)}</div>
            </div>
          </td>
        </tr>
      )}
    </>
  )
}
