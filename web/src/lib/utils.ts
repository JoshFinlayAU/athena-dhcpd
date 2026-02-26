import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// toMs converts a value to milliseconds — handles Unix seconds (< 1e12) vs ms timestamps
function toMs(d: string | number | Date): number {
  if (d instanceof Date) return d.getTime()
  const n = typeof d === 'string' ? Number(d) : d
  // Unix seconds are < ~3.2e10 (year 2970), JS ms timestamps are > 1e12
  return n < 1e12 ? n * 1000 : n
}

export function formatDate(d: string | number | Date): string {
  const date = new Date(toMs(d))
  return date.toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

export function timeAgo(d: string | number | Date): string {
  const now = Date.now()
  const then = toMs(d)
  const seconds = Math.floor((now - then) / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
  return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`
}
