import { useWS } from '@/lib/websocket'
import {
  Activity, Pause, Play, Trash2,
  Search, Send, CheckCircle2, RefreshCw, XCircle, Clock,
  ShieldAlert, ShieldCheck, ShieldX, ShieldOff,
  ArrowRightLeft, ServerCrash, Radio, AlertTriangle,
  type LucideIcon,
} from 'lucide-react'
import { useState, useRef, useEffect } from 'react'
import { formatDate, timeAgo } from '@/lib/utils'
import type { DhcpEvent } from '@/lib/api'

// ── Event metadata ──────────────────────────────────────────────────

interface EventMeta {
  icon: LucideIcon
  label: string
  color: string      // tailwind text color class
  bg: string          // tailwind bg color class for badge
  category: string
}

const eventMeta: Record<string, EventMeta> = {
  'lease.discover':     { icon: Search,        label: 'Discover',           color: 'text-info',        bg: 'bg-info/15 text-info',             category: 'lease' },
  'lease.offer':        { icon: Send,          label: 'Offer',              color: 'text-accent',      bg: 'bg-accent/15 text-accent',         category: 'lease' },
  'lease.ack':          { icon: CheckCircle2,  label: 'Acknowledge',        color: 'text-success',     bg: 'bg-success/15 text-success',       category: 'lease' },
  'lease.renew':        { icon: RefreshCw,     label: 'Renew',              color: 'text-success',     bg: 'bg-success/15 text-success',       category: 'lease' },
  'lease.nak':          { icon: XCircle,       label: 'NAK',                color: 'text-danger',      bg: 'bg-danger/15 text-danger',         category: 'lease' },
  'lease.release':      { icon: Clock,         label: 'Release',            color: 'text-warning',     bg: 'bg-warning/15 text-warning',       category: 'lease' },
  'lease.decline':      { icon: XCircle,       label: 'Decline',            color: 'text-danger',      bg: 'bg-danger/15 text-danger',         category: 'lease' },
  'lease.expire':       { icon: Clock,         label: 'Expire',             color: 'text-text-muted',  bg: 'bg-surface-overlay text-text-muted', category: 'lease' },
  'conflict.detected':  { icon: ShieldAlert,   label: 'Conflict Detected',  color: 'text-danger',      bg: 'bg-danger/15 text-danger',         category: 'conflict' },
  'conflict.decline':   { icon: ShieldX,       label: 'Conflict Decline',   color: 'text-danger',      bg: 'bg-danger/15 text-danger',         category: 'conflict' },
  'conflict.resolved':  { icon: ShieldCheck,   label: 'Conflict Resolved',  color: 'text-success',     bg: 'bg-success/15 text-success',       category: 'conflict' },
  'conflict.permanent': { icon: ShieldOff,     label: 'Conflict Permanent', color: 'text-danger',      bg: 'bg-danger/15 text-danger',         category: 'conflict' },
  'ha.failover':        { icon: ArrowRightLeft,label: 'HA Failover',        color: 'text-warning',     bg: 'bg-warning/15 text-warning',       category: 'ha' },
  'ha.peer_up':         { icon: CheckCircle2,  label: 'Peer Connected',     color: 'text-success',     bg: 'bg-success/15 text-success',       category: 'ha' },
  'ha.peer_down':       { icon: ServerCrash,   label: 'Peer Disconnected',  color: 'text-danger',      bg: 'bg-danger/15 text-danger',         category: 'ha' },
  'ha.sync_complete':   { icon: RefreshCw,     label: 'HA Sync Complete',   color: 'text-info',        bg: 'bg-info/15 text-info',             category: 'ha' },
  'rogue.detected':     { icon: ServerCrash,   label: 'Rogue Server',       color: 'text-danger',      bg: 'bg-danger/15 text-danger',         category: 'rogue' },
  'rogue.resolved':     { icon: ShieldCheck,   label: 'Rogue Resolved',     color: 'text-success',     bg: 'bg-success/15 text-success',       category: 'rogue' },
  'anomaly.detected':   { icon: AlertTriangle, label: 'Anomaly',            color: 'text-warning',     bg: 'bg-warning/15 text-warning',       category: 'anomaly' },
}

const defaultMeta: EventMeta = {
  icon: Radio, label: 'Event', color: 'text-text-secondary', bg: 'bg-surface-overlay text-text-secondary', category: 'other',
}

function getMeta(type: string): EventMeta {
  return eventMeta[type] || defaultMeta
}

// ── Main component ──────────────────────────────────────────────────

export default function Events() {
  const { events, connected } = useWS()
  const [paused, setPaused] = useState(false)
  const [filter, setFilter] = useState('')
  const [cleared, setCleared] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)

  const displayed = events.slice(cleared)
  const filtered = filter
    ? displayed.filter(e => (e.type || '').startsWith(filter))
    : displayed

  // Auto-scroll to top when new events arrive (unless paused)
  useEffect(() => {
    if (!paused && listRef.current) {
      listRef.current.scrollTop = 0
    }
  }, [events.length, paused])

  // Count by category for filter badges
  const counts = displayed.reduce<Record<string, number>>((acc, e) => {
    const cat = getMeta(e.type).category
    acc[cat] = (acc[cat] || 0) + 1
    return acc
  }, {})

  return (
    <div className="p-6 space-y-4 max-w-7xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Events</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            Live event stream · {filtered.length} events
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setCleared(events.length)}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
          >
            <Trash2 className="w-3.5 h-3.5" /> Clear
          </button>
          <button
            onClick={() => setPaused(!paused)}
            className={`flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border transition-colors ${
              paused ? 'border-warning/50 bg-warning/10 text-warning' : 'border-border hover:bg-surface-overlay'
            }`}
          >
            {paused ? <Play className="w-3.5 h-3.5" /> : <Pause className="w-3.5 h-3.5" />}
            {paused ? 'Resume' : 'Pause'}
          </button>
        </div>
      </div>

      {/* Filter bar */}
      <div className="flex items-center gap-2 flex-wrap">
        <FilterChip label="All" count={displayed.length} active={filter === ''} onClick={() => setFilter('')} />
        <FilterChip label="Lease" count={counts.lease || 0} active={filter === 'lease.'} onClick={() => setFilter(filter === 'lease.' ? '' : 'lease.')} />
        <FilterChip label="Conflict" count={counts.conflict || 0} active={filter === 'conflict.'} onClick={() => setFilter(filter === 'conflict.' ? '' : 'conflict.')} />
        <FilterChip label="HA" count={counts.ha || 0} active={filter === 'ha.'} onClick={() => setFilter(filter === 'ha.' ? '' : 'ha.')} />
        <FilterChip label="Rogue" count={counts.rogue || 0} active={filter === 'rogue.'} onClick={() => setFilter(filter === 'rogue.' ? '' : 'rogue.')} />
        <FilterChip label="Anomaly" count={counts.anomaly || 0} active={filter === 'anomaly.'} onClick={() => setFilter(filter === 'anomaly.' ? '' : 'anomaly.')} />

        <div className="ml-auto">
          <div className={`flex items-center gap-1.5 px-3 py-1.5 rounded-full text-[11px] font-medium ${
            connected ? 'bg-success/10 text-success' : 'bg-danger/10 text-danger'
          }`}>
            <div className={`w-1.5 h-1.5 rounded-full ${connected ? 'bg-success animate-pulse' : 'bg-danger'}`} />
            {connected ? (paused ? 'Paused' : 'Live') : 'Disconnected'}
          </div>
        </div>
      </div>

      {/* Event stream */}
      <div className="rounded-xl border border-border bg-surface-raised overflow-hidden">
        <div ref={listRef} className="max-h-[calc(100vh-260px)] overflow-y-auto">
          {filtered.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-text-muted">
              <Activity className="w-8 h-8 mb-2 opacity-30" />
              <p className="text-sm">Waiting for events...</p>
            </div>
          ) : (
            <table className="w-full">
              <thead className="sticky top-0 bg-surface-raised border-b border-border z-10">
                <tr className="text-[11px] text-text-muted uppercase tracking-wider">
                  <th className="text-left px-4 py-2.5 font-medium w-10"></th>
                  <th className="text-left px-2 py-2.5 font-medium w-40">Event</th>
                  <th className="text-left px-2 py-2.5 font-medium">Details</th>
                  <th className="text-right px-4 py-2.5 font-medium w-32">Time</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border/30">
                {filtered.map((evt, i) => (
                  <EventRow key={i} event={evt} />
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

// ── Filter chip ─────────────────────────────────────────────────────

function FilterChip({ label, count, active, onClick }: { label: string; count: number; active: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-medium transition-colors ${
        active
          ? 'bg-accent/15 text-accent border border-accent/30'
          : 'bg-surface-raised border border-border hover:bg-surface-overlay text-text-secondary'
      }`}
    >
      {label}
      {count > 0 && (
        <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${
          active ? 'bg-accent/20' : 'bg-surface-overlay'
        }`}>
          {count}
        </span>
      )}
    </button>
  )
}

// ── Event row ───────────────────────────────────────────────────────

function EventRow({ event }: { event: DhcpEvent }) {
  const meta = getMeta(event.type)
  const Icon = meta.icon

  return (
    <tr className="hover:bg-surface-overlay/50 transition-colors group">
      {/* Icon */}
      <td className="px-4 py-2.5">
        <div className={`w-7 h-7 rounded-lg flex items-center justify-center ${meta.bg}`}>
          <Icon className="w-3.5 h-3.5" />
        </div>
      </td>

      {/* Type badge */}
      <td className="px-2 py-2.5">
        <span className={`text-xs font-semibold ${meta.color}`}>
          {meta.label}
        </span>
        <div className="text-[10px] text-text-muted mt-0.5 font-mono">
          {event.type}
        </div>
      </td>

      {/* Details */}
      <td className="px-2 py-2.5">
        <EventDetails event={event} />
      </td>

      {/* Time */}
      <td className="px-4 py-2.5 text-right">
        <div className="text-[11px] text-text-secondary">{timeAgo(event.timestamp)}</div>
        <div className="text-[10px] text-text-muted">{formatDate(event.timestamp)}</div>
      </td>
    </tr>
  )
}

// ── Event detail renderer ───────────────────────────────────────────

function EventDetails({ event }: { event: DhcpEvent }) {
  if (event.lease) {
    const l = event.lease
    return (
      <div className="flex items-center gap-2 flex-wrap">
        {l.ip && <Tag label={l.ip} variant="ip" />}
        {l.mac && <Tag label={l.mac} variant="mac" />}
        {l.hostname && <Tag label={l.hostname} variant="host" />}
        {l.subnet && <span className="text-[10px] text-text-muted">{l.subnet}</span>}
      </div>
    )
  }

  if (event.conflict) {
    const c = event.conflict
    return (
      <div className="flex items-center gap-2 flex-wrap">
        {c.ip && <Tag label={c.ip} variant="ip" />}
        {c.detection_method && <Tag label={c.detection_method} variant="method" />}
        {c.responder_mac && <Tag label={c.responder_mac} variant="mac" />}
      </div>
    )
  }

  if (event.ha) {
    const h = event.ha
    return (
      <div className="flex items-center gap-2 flex-wrap">
        {h.old_role && h.new_role && (
          <span className="text-xs">
            <span className="text-text-muted">{h.old_role}</span>
            <span className="text-text-muted mx-1">→</span>
            <span className="font-medium text-text-primary">{h.new_role}</span>
          </span>
        )}
        {event.reason && <Tag label={event.reason} variant="reason" />}
      </div>
    )
  }

  if (event.rogue) {
    const r = event.rogue
    return (
      <div className="flex items-center gap-2 flex-wrap">
        {r.server_ip && <Tag label={r.server_ip} variant="ip" />}
        {r.server_mac && <Tag label={r.server_mac} variant="mac" />}
        {r.offered_ip && <span className="text-[10px] text-text-muted">offered {r.offered_ip}</span>}
        {r.count > 1 && <Tag label={`×${r.count}`} variant="method" />}
      </div>
    )
  }

  if (event.reason) {
    return <span className="text-xs text-text-secondary">{event.reason}</span>
  }

  return <span className="text-xs text-text-muted">—</span>
}

// ── Tag pill ────────────────────────────────────────────────────────

const tagStyles: Record<string, string> = {
  ip:     'bg-accent/10 text-accent',
  mac:    'bg-info/10 text-info',
  host:   'bg-success/10 text-success',
  method: 'bg-warning/10 text-warning',
  reason: 'bg-surface-overlay text-text-secondary',
}

function Tag({ label, variant }: { label: string; variant: string }) {
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-[11px] font-mono ${tagStyles[variant] || tagStyles.reason}`}>
      {label}
    </span>
  )
}
