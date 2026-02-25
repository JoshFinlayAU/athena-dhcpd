const BASE = '/api/v1'

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    ...opts,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error || res.statusText)
  }
  return res.json()
}

export interface Lease {
  ip: string
  mac: string
  client_id: string
  hostname: string
  subnet: string
  pool: string
  state: string
  start: string
  expiry: string
  last_updated: string
  relay_info?: { giaddr: string; circuit_id: string; remote_id: string }
}

export interface Reservation {
  mac: string
  identifier: string
  ip: string
  hostname: string
  dns_servers?: string[]
  ddns_hostname?: string
}

export interface ConflictEntry {
  ip: string
  detected_at: string
  detection_method: string
  responder_mac: string
  hold_until: string
  subnet: string
  probe_count: number
  permanent: boolean
  resolved: boolean
}

export interface ConflictStats {
  total_active: number
  total_permanent: number
  total_resolved: number
  by_subnet: Record<string, number>
  by_method: Record<string, number>
}

export interface HAStatus {
  enabled: boolean
  role: string
  state: string
  peer_address: string
  peer_connected: boolean
  last_heartbeat: string
}

export interface HealthResponse {
  status: string
  uptime: number
  lease_count: number
  version: string
}

export interface LeaseListResponse {
  leases: Lease[]
  total: number
  page: number
  page_size: number
}

export interface DhcpEvent {
  type: string
  timestamp: string
  lease?: {
    ip: string
    mac: string
    hostname: string
    subnet: string
    fqdn?: string
  }
  conflict?: {
    ip: string
    subnet: string
    detection_method: string
    responder_mac: string
  }
  reason?: string
}

// Auth
export interface AuthUser {
  authenticated: boolean
  username?: string
  role?: string
  auth_required: boolean
}

export const getMe = () => request<AuthUser>('/auth/me')
export const login = (username: string, password: string) =>
  request<{ username: string; role: string }>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
export const logout = () =>
  request<{ status: string }>('/auth/logout', { method: 'POST' })

// Health
export const getHealth = () => request<HealthResponse>('/health')

// Leases
export const getLeases = (params?: string) =>
  request<LeaseListResponse>(`/leases${params ? `?${params}` : ''}`)
export const getLease = (ip: string) => request<Lease>(`/leases/${ip}`)
export const deleteLease = (ip: string) =>
  request<void>(`/leases/${ip}`, { method: 'DELETE' })

// Reservations
export const getReservations = () => request<Reservation[]>('/reservations')
export const createReservation = (r: Reservation) =>
  request<Reservation>('/reservations', { method: 'POST', body: JSON.stringify(r) })
export const updateReservation = (id: string, r: Reservation) =>
  request<Reservation>(`/reservations/${id}`, { method: 'PUT', body: JSON.stringify(r) })
export const deleteReservation = (id: string) =>
  request<void>(`/reservations/${id}`, { method: 'DELETE' })

// Conflicts
export const getConflicts = () => request<ConflictEntry[]>('/conflicts')
export const getConflict = (ip: string) => request<ConflictEntry>(`/conflicts/${ip}`)
export const clearConflict = (ip: string) =>
  request<void>(`/conflicts/${ip}`, { method: 'DELETE' })
export const excludeConflict = (ip: string) =>
  request<void>(`/conflicts/${ip}/exclude`, { method: 'POST' })
export const getConflictStats = () => request<ConflictStats>('/conflicts/stats')
export const getConflictHistory = () => request<ConflictEntry[]>('/conflicts/history')

// Config
export const getConfigRaw = async (): Promise<{ config: string }> => {
  const res = await fetch(`${BASE}/config/raw`, { credentials: 'include' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error || res.statusText)
  }
  const config = await res.text()
  return { config }
}
export const validateConfig = (config: string) =>
  request<{ valid: boolean; errors?: string[] }>('/config/validate', {
    method: 'POST', body: JSON.stringify({ config }),
  })
export const updateConfig = (config: string) =>
  request<void>('/config', { method: 'PUT', body: JSON.stringify({ config }) })

// HA
export const getHAStatus = () => request<HAStatus>('/ha/status')
export const triggerFailover = () =>
  request<void>('/ha/failover', { method: 'POST' })

// Events
export const getEvents = () => request<DhcpEvent[]>('/events')

// DNS Proxy
export interface DNSStats {
  zone_records: number
  cache_entries: number
  forwarders: number
  overrides: number
  domain: string
  filter_lists: number
  blocked_domains: number
}

export interface DNSListStatus {
  name: string
  url: string
  type: string
  format: string
  action: string
  enabled: boolean
  domain_count: number
  last_refresh: string
  last_error: string
  refresh_interval: string
  next_refresh: string
}

export interface DNSListsResponse {
  lists: DNSListStatus[]
  total_domains: number
}

export interface DNSTestResult {
  domain: string
  blocked: boolean
  action?: string
  list?: string
  matches?: { list: string; type: string }[]
}

export const getDNSStats = () => request<DNSStats>('/dns/stats')
export const getDNSLists = () => request<DNSListsResponse>('/dns/lists')
export const refreshDNSLists = (name?: string) =>
  request<{ status: string }>('/dns/lists/refresh', {
    method: 'POST',
    body: JSON.stringify(name ? { name } : {}),
  })
export const testDNSDomain = (domain: string) =>
  request<DNSTestResult>('/dns/lists/test', {
    method: 'POST',
    body: JSON.stringify({ domain }),
  })
export const flushDNSCache = () =>
  request<{ status: string }>('/dns/cache/flush', { method: 'POST' })
