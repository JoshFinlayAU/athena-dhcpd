import { cn } from '@/lib/utils'
import { Plus, X } from 'lucide-react'
import { type ReactNode } from 'react'

// --- Layout ---

export function Section({ title, description, children, defaultOpen = true }: {
  title: string
  description?: string
  children: ReactNode
  defaultOpen?: boolean
}) {
  return (
    <details open={defaultOpen} className="group border border-border rounded-xl bg-surface-raised overflow-hidden">
      <summary className="flex items-center justify-between px-5 py-4 cursor-pointer select-none hover:bg-surface-overlay/50 transition-colors">
        <div>
          <h3 className="text-sm font-semibold">{title}</h3>
          {description && <p className="text-xs text-text-muted mt-0.5">{description}</p>}
        </div>
        <span className="text-text-muted text-xs group-open:rotate-90 transition-transform">▶</span>
      </summary>
      <div className="px-5 pb-5 pt-2 space-y-4 border-t border-border/50">
        {children}
      </div>
    </details>
  )
}

export function FieldGrid({ children }: { children: ReactNode }) {
  return <div className="grid grid-cols-1 md:grid-cols-2 gap-4">{children}</div>
}

export function FieldRow({ children }: { children: ReactNode }) {
  return <div className="space-y-1.5">{children}</div>
}

// --- Basic Fields ---

export function Label({ children, htmlFor, hint }: { children: ReactNode; htmlFor?: string; hint?: string }) {
  return (
    <label htmlFor={htmlFor} className="flex items-baseline gap-2 text-xs font-medium text-text-secondary">
      <span>{children}</span>
      {hint && <span className="text-text-muted font-normal">({hint})</span>}
    </label>
  )
}

export function TextInput({ id, value, onChange, placeholder, mono, className }: {
  id?: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
  mono?: boolean
  className?: string
}) {
  return (
    <input
      id={id}
      type="text"
      value={value}
      onChange={e => onChange(e.target.value)}
      placeholder={placeholder}
      className={cn(
        'w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface hover:border-border-hover focus:border-accent focus:ring-1 focus:ring-accent/30 outline-none transition-colors',
        mono && 'font-mono',
        className
      )}
    />
  )
}

export function NumberInput({ id, value, onChange, min, max, placeholder }: {
  id?: string
  value: number
  onChange: (v: number) => void
  min?: number
  max?: number
  placeholder?: string
}) {
  return (
    <input
      id={id}
      type="number"
      value={value || ''}
      onChange={e => onChange(parseInt(e.target.value) || 0)}
      min={min}
      max={max}
      placeholder={placeholder}
      className="w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface hover:border-border-hover focus:border-accent focus:ring-1 focus:ring-accent/30 outline-none transition-colors font-mono tabular-nums"
    />
  )
}

export function Toggle({ id, checked, onChange, label, description }: {
  id?: string
  checked: boolean
  onChange: (v: boolean) => void
  label: string
  description?: string
}) {
  return (
    <label htmlFor={id} className="flex items-start gap-3 cursor-pointer group">
      <div className="relative mt-0.5 flex-shrink-0">
        <input
          id={id}
          type="checkbox"
          checked={checked}
          onChange={e => onChange(e.target.checked)}
          className="sr-only peer"
        />
        <div className="w-9 h-5 rounded-full bg-border peer-checked:bg-accent transition-colors" />
        <div className="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-text-muted peer-checked:bg-white peer-checked:translate-x-4 transition-all" />
      </div>
      <div className="flex-1 min-w-0">
        <span className="text-sm font-medium">{label}</span>
        {description && <p className="text-xs text-text-muted mt-0.5">{description}</p>}
      </div>
    </label>
  )
}

export function Select({ id, value, onChange, options, placeholder }: {
  id?: string
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  placeholder?: string
}) {
  return (
    <select
      id={id}
      value={value}
      onChange={e => onChange(e.target.value)}
      className="w-full px-3 py-2 text-sm rounded-lg border border-border bg-surface hover:border-border-hover focus:border-accent focus:ring-1 focus:ring-accent/30 outline-none transition-colors appearance-none"
    >
      {placeholder && <option value="">{placeholder}</option>}
      {options.map(o => (
        <option key={o.value} value={o.value}>{o.label}</option>
      ))}
    </select>
  )
}

// --- Compound Fields ---

export function Field({ label, hint, htmlFor, children }: {
  label: string
  hint?: string
  htmlFor?: string
  children: ReactNode
}) {
  return (
    <FieldRow>
      <Label htmlFor={htmlFor} hint={hint}>{label}</Label>
      {children}
    </FieldRow>
  )
}

export function StringArrayInput({ value, onChange, placeholder, mono }: {
  value: string[]
  onChange: (v: string[]) => void
  placeholder?: string
  mono?: boolean
}) {
  const add = () => onChange([...value, ''])
  const remove = (i: number) => onChange(value.filter((_, idx) => idx !== i))
  const update = (i: number, v: string) => {
    const next = [...value]
    next[i] = v
    onChange(next)
  }

  return (
    <div className="space-y-2">
      {value.map((item, i) => (
        <div key={i} className="flex items-center gap-2">
          <TextInput
            value={item}
            onChange={v => update(i, v)}
            placeholder={placeholder}
            mono={mono}
            className="flex-1"
          />
          <button
            type="button"
            onClick={() => remove(i)}
            className="p-1.5 rounded-lg text-text-muted hover:text-danger hover:bg-danger/10 transition-colors"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={add}
        className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover transition-colors"
      >
        <Plus className="w-3 h-3" /> Add
      </button>
    </div>
  )
}

export function KeyValueInput({ value, onChange, keyPlaceholder, valuePlaceholder }: {
  value: Record<string, string>
  onChange: (v: Record<string, string>) => void
  keyPlaceholder?: string
  valuePlaceholder?: string
}) {
  const entries = Object.entries(value)
  const add = () => onChange({ ...value, '': '' })
  const remove = (key: string) => {
    const next = { ...value }
    delete next[key]
    onChange(next)
  }
  const updateEntry = (oldKey: string, newKey: string, newVal: string) => {
    const next: Record<string, string> = {}
    for (const [k, v] of Object.entries(value)) {
      if (k === oldKey) {
        next[newKey] = newVal
      } else {
        next[k] = v
      }
    }
    onChange(next)
  }

  return (
    <div className="space-y-2">
      {entries.map(([k, v], i) => (
        <div key={i} className="flex items-center gap-2">
          <TextInput
            value={k}
            onChange={nk => updateEntry(k, nk, v)}
            placeholder={keyPlaceholder || 'Key'}
            className="flex-1"
          />
          <TextInput
            value={v}
            onChange={nv => updateEntry(k, k, nv)}
            placeholder={valuePlaceholder || 'Value'}
            className="flex-1"
          />
          <button
            type="button"
            onClick={() => remove(k)}
            className="p-1.5 rounded-lg text-text-muted hover:text-danger hover:bg-danger/10 transition-colors"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={add}
        className="flex items-center gap-1.5 text-xs text-accent hover:text-accent-hover transition-colors"
      >
        <Plus className="w-3 h-3" /> Add
      </button>
    </div>
  )
}
