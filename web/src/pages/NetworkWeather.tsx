import { useCallback } from 'react'
import { usePolling } from '@/hooks/useApi'
import { Card } from '@/components/Card'
import { CloudSun, AlertTriangle, CheckCircle, VolumeX, TrendingUp } from 'lucide-react'
import { v2GetWeather, type SubnetWeather } from '@/lib/api'

const statusConfig: Record<string, { color: string; icon: typeof CheckCircle; label: string }> = {
  normal:   { color: 'text-success', icon: CheckCircle, label: 'Normal' },
  elevated: { color: 'text-warning', icon: TrendingUp, label: 'Elevated' },
  alert:    { color: 'text-danger', icon: AlertTriangle, label: 'Alert' },
  silent:   { color: 'text-text-muted', icon: VolumeX, label: 'Silent' },
}

function formatTime(ts: string) {
  if (!ts) return 'never'
  try { return new Date(ts).toLocaleString() } catch { return ts }
}

export default function NetworkWeather() {
  const { data: weather, loading } = usePolling(useCallback(() => v2GetWeather(), []), 10000)

  return (
    <div className="p-6 max-w-7xl space-y-4">
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2">
          <CloudSun className="w-6 h-6" /> Network Weather
        </h1>
        <p className="text-sm text-text-secondary mt-0.5">
          Per-subnet DHCP activity baseline tracking and anomaly detection
        </p>
      </div>

      {loading && (!weather || weather.length === 0) && (
        <Card className="p-8 text-center text-sm text-text-muted">Loading weather data...</Card>
      )}

      {weather && weather.length === 0 && (
        <Card className="p-8 text-center">
          <CloudSun className="w-10 h-10 text-text-muted mx-auto mb-3 opacity-50" />
          <p className="text-sm font-medium text-text-secondary">No activity data yet</p>
          <p className="text-xs text-text-muted mt-1">Weather data is collected from DHCP lease events over time</p>
        </Card>
      )}

      {weather && weather.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {weather.map(w => <WeatherCard key={w.subnet} weather={w} />)}
        </div>
      )}
    </div>
  )
}

function WeatherCard({ weather: w }: { weather: SubnetWeather }) {
  const cfg = statusConfig[w.status] || statusConfig.normal
  const Icon = cfg.icon

  return (
    <Card className="p-4 space-y-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Icon className={`w-5 h-5 ${cfg.color}`} />
          <span className="font-semibold text-sm">{w.subnet}</span>
        </div>
        <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${cfg.color} bg-current/10`}>
          {cfg.label}
        </span>
      </div>

      {w.anomaly_reason && (
        <div className="px-3 py-2 rounded-lg bg-danger/10 text-xs text-danger">
          {w.anomaly_reason} (score: {w.anomaly_score})
        </div>
      )}

      <div className="grid grid-cols-2 gap-x-6 gap-y-2 text-xs">
        <div className="flex justify-between">
          <span className="text-text-muted">Current Rate</span>
          <span className="font-mono font-medium">{w.current_rate}/min</span>
        </div>
        <div className="flex justify-between">
          <span className="text-text-muted">Baseline Rate</span>
          <span className="font-mono">{w.baseline_rate}/min</span>
        </div>
        <div className="flex justify-between">
          <span className="text-text-muted">Std Dev</span>
          <span className="font-mono">{w.std_dev}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-text-muted">Anomaly Score</span>
          <span className="font-mono">{w.anomaly_score}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-text-muted">Known MACs</span>
          <span className="font-mono">{w.known_macs}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-text-muted">New MACs (window)</span>
          <span className="font-mono">{w.unknown_macs_recent}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-text-muted">Silent</span>
          <span className="font-mono">{w.silent_minutes}m</span>
        </div>
        <div className="flex justify-between">
          <span className="text-text-muted">Last Activity</span>
          <span>{formatTime(w.last_activity)}</span>
        </div>
      </div>
    </Card>
  )
}
