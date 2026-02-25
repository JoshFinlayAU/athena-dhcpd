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
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const connect = useCallback(() => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${proto}//${window.location.host}/api/v1/events/stream`)
    wsRef.current = ws

    ws.onopen = () => setConnected(true)
    ws.onclose = () => {
      setConnected(false)
      reconnectRef.current = setTimeout(connect, 3000)
    }
    ws.onerror = () => ws.close()
    ws.onmessage = (msg) => {
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
      if (reconnectRef.current) clearTimeout(reconnectRef.current)
      wsRef.current?.close()
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
