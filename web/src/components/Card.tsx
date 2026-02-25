import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn('rounded-xl border border-border bg-surface-raised p-5', className)}>
      {children}
    </div>
  )
}

export function StatCard({ label, value, sub, icon: Icon, color }: {
  label: string
  value: string | number
  sub?: string
  icon: React.ComponentType<{ className?: string }>
  color?: string
}) {
  return (
    <Card className="flex items-start gap-4">
      <div className={cn('p-2.5 rounded-lg', color || 'bg-accent/15')}>
        <Icon className={cn('w-5 h-5', color ? '' : 'text-accent')} />
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium text-text-muted uppercase tracking-wider">{label}</p>
        <p className="text-2xl font-bold mt-0.5 tabular-nums">{value}</p>
        {sub && <p className="text-xs text-text-secondary mt-0.5">{sub}</p>}
      </div>
    </Card>
  )
}
