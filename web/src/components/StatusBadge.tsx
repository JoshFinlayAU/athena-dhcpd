import { cn } from '@/lib/utils'

const variants: Record<string, string> = {
  active: 'bg-success/15 text-success border-success/30',
  offered: 'bg-info/15 text-info border-info/30',
  expired: 'bg-text-muted/15 text-text-muted border-text-muted/30',
  declined: 'bg-danger/15 text-danger border-danger/30',
  conflict: 'bg-danger/15 text-danger border-danger/30',
  permanent: 'bg-danger/15 text-danger border-danger/30',
  clear: 'bg-success/15 text-success border-success/30',
  resolved: 'bg-success/15 text-success border-success/30',
  connected: 'bg-success/15 text-success border-success/30',
  disconnected: 'bg-danger/15 text-danger border-danger/30',
  standby: 'bg-warning/15 text-warning border-warning/30',
  partner_up: 'bg-success/15 text-success border-success/30',
  partner_down: 'bg-danger/15 text-danger border-danger/30',
  recovery: 'bg-warning/15 text-warning border-warning/30',
}

export default function StatusBadge({ status, className }: { status: string; className?: string }) {
  const key = status.toLowerCase().replace(/\s+/g, '_')
  const variant = variants[key] || 'bg-surface-overlay text-text-secondary border-border'
  return (
    <span className={cn(
      'inline-flex items-center px-2 py-0.5 text-xs font-medium rounded-full border',
      variant, className
    )}>
      {status}
    </span>
  )
}
