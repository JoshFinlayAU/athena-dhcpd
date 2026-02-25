import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

export function Table({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn('overflow-x-auto rounded-xl border border-border', className)}>
      <table className="w-full text-sm">{children}</table>
    </div>
  )
}

export function THead({ children }: { children: ReactNode }) {
  return (
    <thead className="bg-surface-overlay border-b border-border">
      {children}
    </thead>
  )
}

export function TH({ children, className }: { children?: ReactNode; className?: string }) {
  return (
    <th className={cn('px-4 py-3 text-left text-xs font-semibold text-text-muted uppercase tracking-wider', className)}>
      {children}
    </th>
  )
}

export function TD({ children, className, mono }: { children?: ReactNode; className?: string; mono?: boolean }) {
  return (
    <td className={cn('px-4 py-3 whitespace-nowrap', mono && 'font-mono text-xs', className)}>
      {children}
    </td>
  )
}

export function TR({ children, className, onClick }: { children: ReactNode; className?: string; onClick?: () => void }) {
  return (
    <tr
      className={cn(
        'border-b border-border/50 last:border-0 transition-colors',
        onClick ? 'cursor-pointer hover:bg-surface-overlay' : 'hover:bg-surface-overlay/50',
        className
      )}
      onClick={onClick}
    >
      {children}
    </tr>
  )
}

export function EmptyRow({ cols, message }: { cols: number; message?: string }) {
  return (
    <tr>
      <td colSpan={cols} className="px-4 py-12 text-center text-text-muted">
        {message || 'No data'}
      </td>
    </tr>
  )
}
