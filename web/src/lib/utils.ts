import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// toMs converts a value to milliseconds — handles ISO strings, Unix seconds, and ms timestamps
function toMs(d: string | number | Date): number {
  if (d instanceof Date) return d.getTime()
  if (typeof d === 'string') {
    // Try parsing as ISO 8601 / RFC 3339 first (Go time.Time marshals to this)
    const parsed = Date.parse(d)
    if (!isNaN(parsed)) return parsed
    // Fall back to numeric interpretation
    const n = Number(d)
    if (!isNaN(n)) return n < 1e12 ? n * 1000 : n
    return NaN
  }
  // Unix seconds are < ~3.2e10 (year 2970), JS ms timestamps are > 1e12
  return d < 1e12 ? d * 1000 : d
}

export function formatDate(d: string | number | Date): string {
  const ms = toMs(d)
  if (isNaN(ms)) return '—'
  const date = new Date(ms)
  return date.toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

export function timeAgo(d: string | number | Date): string {
  const now = Date.now()
  const then = toMs(d)
  if (isNaN(then)) return '—'
  const diff = now - then
  const seconds = Math.floor(Math.abs(diff) / 1000)
  const future = diff < 0

  let label: string
  if (seconds < 60) label = `${seconds}s`
  else if (seconds < 3600) label = `${Math.floor(seconds / 60)}m`
  else if (seconds < 86400) label = `${Math.floor(seconds / 3600)}h`
  else label = `${Math.floor(seconds / 86400)}d`

  return future ? `in ${label}` : `${label} ago`
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
  return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`
}
