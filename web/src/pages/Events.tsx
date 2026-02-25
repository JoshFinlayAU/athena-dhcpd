import { useWS } from '@/lib/websocket'
import { Activity, Pause, Play, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { formatDate } from '@/lib/utils'
import type { DhcpEvent } from '@/lib/api'

const typeColors: Record<string, string> = {
  'lease.discover': 'text-info',
  'lease.offer': 'text-accent',
  'lease.ack': 'text-success',
  'lease.renew': 'text-success',
  'lease.release': 'text-warning',
  'lease.decline': 'text-danger',
  'lease.expire': 'text-text-muted',
  'lease.nak': 'text-danger',
  'conflict.detected': 'text-danger',
  'conflict.decline': 'text-danger',
  'conflict.resolved': 'text-success',
  'conflict.permanent': 'text-danger',
}

export default function Events() {
  const { events, connected } = useWS()
  const [paused, setPaused] = useState(false)
  const [filter, setFilter] = useState('')
  const [cleared, setCleared] = useState(0)

  const displayed = events.slice(cleared)
  const filtered = filter
    ? displayed.filter(e => e.type.includes(filter))
    : displayed

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
          <select
            value={filter}
            onChange={e => setFilter(e.target.value)}
            className="px-3 py-2 text-xs rounded-lg border border-border bg-surface-raised text-text-primary focus:outline-none focus:border-accent"
          >
            <option value="">All Events</option>
            <option value="lease.">Lease Events</option>
            <option value="conflict.">Conflict Events</option>
          </select>
          <button
            onClick={() => setCleared(events.length)}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
          >
            <Trash2 className="w-3.5 h-3.5" /> Clear
          </button>
          <button
            onClick={() => setPaused(!paused)}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-surface-overlay transition-colors"
          >
            {paused ? <Play className="w-3.5 h-3.5" /> : <Pause className="w-3.5 h-3.5" />}
            {paused ? 'Resume' : 'Pause'}
          </button>
        </div>
      </div>

      {/* Connection indicator */}
      <div className={`flex items-center gap-2 px-4 py-2.5 rounded-lg border ${
        connected ? 'border-success/30 bg-success/5' : 'border-danger/30 bg-danger/5'
      }`}>
        <div className={`w-2 h-2 rounded-full ${connected ? 'bg-success animate-pulse' : 'bg-danger'}`} />
        <span className="text-xs font-medium">
          {connected
            ? paused ? 'Connected — stream paused' : 'Connected — streaming live events'
            : 'Disconnected — attempting reconnection...'
          }
        </span>
      </div>

      {/* Event stream */}
      <div className="rounded-xl border border-border bg-surface-raised overflow-hidden">
        <div className="divide-y divide-border/50 max-h-[calc(100vh-280px)] overflow-y-auto">
          {filtered.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-text-muted">
              <Activity className="w-8 h-8 mb-2 opacity-30" />
              <p className="text-sm">Waiting for events...</p>
            </div>
          ) : (
            (paused ? filtered : filtered).map((evt, i) => (
              <EventRow key={i} event={evt} />
            ))
          )}
        </div>
      </div>
    </div>
  )
}

function EventRow({ event }: { event: DhcpEvent }) {
  const color = typeColors[event.type] || 'text-text-secondary'
  return (
    <div className="flex items-start gap-4 px-4 py-3 hover:bg-surface-overlay/50 transition-colors">
      <div className="flex-shrink-0 mt-0.5">
        <div className={`w-2 h-2 rounded-full ${color.replace('text-', 'bg-')}`} />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className={`text-xs font-semibold ${color}`}>
            {event.type}
          </span>
          <span className="text-[10px] text-text-muted">
            {formatDate(event.timestamp)}
          </span>
        </div>
        <div className="text-xs text-text-secondary mt-0.5 font-mono">
          {event.lease && (
            <span>
              {event.lease.ip}
              {event.lease.mac && ` · ${event.lease.mac}`}
              {event.lease.hostname && ` · ${event.lease.hostname}`}
            </span>
          )}
          {event.conflict && (
            <span>
              {event.conflict.ip} · {event.conflict.detection_method}
              {event.conflict.responder_mac && ` · ${event.conflict.responder_mac}`}
            </span>
          )}
          {event.reason && <span className="text-danger"> · {event.reason}</span>}
        </div>
      </div>
    </div>
  )
}
