import { useState, useCallback } from 'react'
import { useApi } from '@/hooks/useApi'
import { Card } from '@/components/Card'
import { GitBranch, Router, Cable, Monitor, Clock } from 'lucide-react'
import {
  v2GetTopology, v2GetTopologyStats,
  type TopologySwitch, type TopologyPort, type TopologyDevice,
} from '@/lib/api'

function formatTime(ts: string) {
  try { return new Date(ts).toLocaleString() } catch { return ts }
}

export default function Topology() {
  const { data: tree, loading } = useApi(useCallback(() => v2GetTopology(), []))
  const { data: stats } = useApi(useCallback(() => v2GetTopologyStats(), []))

  return (
    <div className="p-6 max-w-7xl space-y-4">
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2">
          <GitBranch className="w-6 h-6" /> Network Topology
        </h1>
        <p className="text-sm text-text-secondary mt-0.5">
          Learned from Option 82 relay agent data (switch → port → device)
        </p>
      </div>

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-3 gap-3">
          <Card className="p-4 flex items-center gap-3">
            <Router className="w-5 h-5 text-accent" />
            <div>
              <div className="text-xl font-bold">{stats.switches}</div>
              <div className="text-xs text-text-muted">Switches / Relays</div>
            </div>
          </Card>
          <Card className="p-4 flex items-center gap-3">
            <Cable className="w-5 h-5 text-warning" />
            <div>
              <div className="text-xl font-bold">{stats.ports}</div>
              <div className="text-xs text-text-muted">Ports</div>
            </div>
          </Card>
          <Card className="p-4 flex items-center gap-3">
            <Monitor className="w-5 h-5 text-success" />
            <div>
              <div className="text-xl font-bold">{stats.devices}</div>
              <div className="text-xs text-text-muted">Devices</div>
            </div>
          </Card>
        </div>
      )}

      {/* Tree */}
      {loading && <Card className="p-8 text-center text-sm text-text-muted">Loading topology...</Card>}
      {tree && tree.length === 0 && (
        <Card className="p-8 text-center">
          <GitBranch className="w-10 h-10 text-text-muted mx-auto mb-3 opacity-50" />
          <p className="text-sm font-medium text-text-secondary">No topology data yet</p>
          <p className="text-xs text-text-muted mt-1">Topology is learned from DHCP relay agent Option 82 data</p>
        </Card>
      )}
      {tree && tree.map(sw => <SwitchCard key={sw.id} sw={sw} />)}
    </div>
  )
}

function SwitchCard({ sw }: { sw: TopologySwitch }) {
  const [expanded, setExpanded] = useState(true)
  const portEntries = Object.entries(sw.ports || {}).sort(([a], [b]) => a.localeCompare(b))

  return (
    <Card className="overflow-hidden">
      <div
        className="flex items-center justify-between px-4 py-3 bg-surface-raised cursor-pointer hover:bg-surface-overlay transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex items-center gap-3">
          <Router className="w-5 h-5 text-accent" />
          <div>
            <div className="text-sm font-semibold">{sw.label || sw.id}</div>
            <div className="text-xs text-text-muted space-x-3">
              {sw.remote_id && <span>Remote ID: <span className="font-mono">{sw.remote_id}</span></span>}
              {sw.giaddr && <span>GIAddr: <span className="font-mono">{sw.giaddr}</span></span>}
              <span>{portEntries.length} ports</span>
            </div>
          </div>
        </div>
        <div className="flex items-center gap-1 text-xs text-text-muted">
          <Clock className="w-3 h-3" /> {formatTime(sw.last_seen)}
        </div>
      </div>

      {expanded && (
        <div className="divide-y divide-border">
          {portEntries.map(([key, port]) => (
            <PortRow key={key} portKey={key} port={port} />
          ))}
        </div>
      )}
    </Card>
  )
}

function PortRow({ portKey, port }: { portKey: string; port: TopologyPort }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div>
      <div
        className="flex items-center justify-between px-4 py-2.5 pl-10 cursor-pointer hover:bg-surface/50 transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex items-center gap-2">
          <Cable className="w-4 h-4 text-warning" />
          <span className="text-xs font-mono font-medium">{port.label || portKey}</span>
          {port.label && <span className="text-[10px] text-text-muted font-mono">({portKey})</span>}
          <span className="text-[10px] text-text-muted">{port.devices?.length || 0} devices</span>
        </div>
        <span className="text-xs text-text-muted">{formatTime(port.last_seen)}</span>
      </div>

      {expanded && port.devices && port.devices.length > 0 && (
        <div className="pl-16 pr-4 pb-2 space-y-1">
          {port.devices.map(dev => <DeviceItem key={dev.mac} device={dev} />)}
        </div>
      )}
    </div>
  )
}

function DeviceItem({ device: d }: { device: TopologyDevice }) {
  return (
    <div className="flex items-center justify-between py-1.5 px-3 rounded-md bg-surface/30 text-xs">
      <div className="flex items-center gap-3">
        <Monitor className="w-3.5 h-3.5 text-success" />
        <span className="font-mono">{d.mac}</span>
        <span className="font-mono text-text-secondary">{d.ip}</span>
        {d.hostname && <span className="text-text-secondary">{d.hostname}</span>}
      </div>
      <div className="text-text-muted">
        {d.subnet && <span className="mr-3">{d.subnet}</span>}
        {formatTime(d.last_seen)}
      </div>
    </div>
  )
}
