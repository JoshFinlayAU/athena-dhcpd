import { createContext, useContext, useEffect, useRef, useState, useCallback, type ReactNode } from 'react'
import type { DhcpEvent } from './api'

interface WSContextValue {
  connected: boolean
  events: DhcpEvent[]
  lastEvent: DhcpEvent | null
  subscribe: (handler: (evt: DhcpEvent) => void) => () => void
}

const WSContext = createContext<WSContextValue>({
  connected: false,
  events: [],
  lastEvent: null,
  subscribe: () => () => {},
})

const MAX_EVENTS = 200

export function WSProvider({ children }: { children: ReactNode }) {
  const [connected, setConnected] = useState(false)
  const [events, setEvents] = useState<DhcpEvent[]>([])
  const [lastEvent, setLastEvent] = useState<DhcpEvent | null>(null)
  const handlersRef = useRef<Set<(evt: DhcpEvent) => void>>(new Set())
  const esRef = useRef<EventSource | null>(null)

  const connect = useCallback(() => {
    // SSE via native EventSource — works over plain HTTP, auto-reconnects
    const es = new EventSource(`/api/v1/events/stream`)
    esRef.current = es

    es.onopen = () => setConnected(true)
    es.onerror = () => {
      setConnected(false)
      // EventSource auto-reconnects, no manual retry needed
    }
    es.onmessage = (msg) => {
      try {
        const evt: DhcpEvent = JSON.parse(msg.data)
        setLastEvent(evt)
        setEvents((prev) => [evt, ...prev].slice(0, MAX_EVENTS))
        handlersRef.current.forEach((h) => h(evt))
      } catch { /* ignore malformed */ }
    }
  }, [])

  useEffect(() => {
    connect()
    return () => {
      esRef.current?.close()
    }
  }, [connect])

  const subscribe = useCallback((handler: (evt: DhcpEvent) => void) => {
    handlersRef.current.add(handler)
    return () => { handlersRef.current.delete(handler) }
  }, [])

  return (
    <WSContext.Provider value={{ connected, events, lastEvent, subscribe }}>
      {children}
    </WSContext.Provider>
  )
}

export function useWS() {
  return useContext(WSContext)
}

export function useWSEvent(handler: (evt: DhcpEvent) => void) {
  const { subscribe } = useWS()
  useEffect(() => subscribe(handler), [subscribe, handler])
}
