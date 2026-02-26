const BASE = '/api/v2'

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

// DNS Query Log
export interface DNSQueryLogEntry {
  timestamp: string
  name: string
  type: string
  source: string
  status: string
  latency_ms: number
  answer?: string
  list_name?: string
  action?: string
}

export const getDNSQueryLog = (limit?: number) =>
  request<{ entries: DNSQueryLogEntry[]; total: number }>(`/dns/querylog${limit ? `?limit=${limit}` : ''}`)

// --- Config API (DB-backed CRUD) ---

// Subnet types matching Go config.SubnetConfig
export interface SubnetConfig {
  network: string
  interface?: string
  routers?: string[]
  dns_servers?: string[]
  domain_name?: string
  lease_time?: string
  renewal_time?: string
  rebind_time?: string
  ntp_servers?: string[]
  pool?: PoolConfig[]
  reservation?: ReservationConfig[]
  option?: OptionConfig[]
}

export interface PoolConfig {
  range_start: string
  range_end: string
  lease_time?: string
  match_circuit_id?: string
  match_remote_id?: string
  match_vendor_class?: string
  match_user_class?: string
}

export interface ReservationConfig {
  mac: string
  identifier?: string
  ip: string
  hostname?: string
  dns_servers?: string[]
  ddns_hostname?: string
}

export interface OptionConfig {
  code: number
  type: string
  value: unknown
}

export interface DefaultsConfig {
  lease_time: string
  renewal_time: string
  rebind_time: string
  dns_servers: string[]
  domain_name: string
}

export interface ConflictDetectionConfig {
  enabled: boolean
  probe_strategy: string
  probe_timeout: string
  max_probes_per_discover: number
  parallel_probe_count: number
  conflict_hold_time: string
  max_conflict_count: number
  probe_cache_ttl: string
  send_gratuitous_arp: boolean
  icmp_fallback: boolean
  probe_log_level: string
}

export interface HAConfigType {
  enabled: boolean
  role: string
  peer_address: string
  listen_address: string
  heartbeat_interval: string
  failover_timeout: string
  sync_batch_size: number
  tls: { enabled: boolean; cert_file: string; key_file: string; ca_file: string }
}

export interface HooksConfigType {
  event_buffer_size: number
  script_concurrency: number
  script_timeout: string
  script?: { name: string; events: string[]; command: string; timeout: string; subnets?: string[] }[]
  webhook?: { name: string; events: string[]; url: string; method: string; headers?: Record<string, string>; timeout: string; retries: number; retry_backoff: string; secret?: string; template?: string }[]
}

export interface DDNSConfigType {
  enabled: boolean
  allow_client_fqdn: boolean
  fallback_to_mac: boolean
  ttl: number
  update_on_renew: boolean
  conflict_policy: string
  use_dhcid: boolean
  forward: { zone: string; method: string; server: string; tsig_name: string; tsig_algorithm: string; tsig_secret: string; api_key: string }
  reverse: { zone: string; method: string; server: string; tsig_name: string; tsig_algorithm: string; tsig_secret: string; api_key: string }
}

export interface DNSConfigType {
  enabled: boolean
  listen_udp: string
  listen_doh: string
  domain: string
  ttl: number
  register_leases: boolean
  register_leases_ptr: boolean
  forwarders: string[]
  use_root_servers: boolean
  cache_size: number
  cache_ttl: string
  zone_override?: { zone: string; nameserver: string; doh: boolean; doh_url: string }[]
  record?: { name: string; type: string; value: string; ttl: number }[]
  list?: { name: string; url: string; type: string; format: string; action: string; enabled: boolean; refresh_interval: string }[]
}

// Subnets
export const v2GetSubnets = () => request<SubnetConfig[]>('/config/subnets')
export const v2CreateSubnet = (sub: SubnetConfig) =>
  request<SubnetConfig>('/config/subnets', { method: 'POST', body: JSON.stringify(sub) })
export const v2UpdateSubnet = (network: string, sub: SubnetConfig) =>
  request<SubnetConfig>(`/config/subnets/${encodeURIComponent(network)}`, { method: 'PUT', body: JSON.stringify(sub) })
export const v2DeleteSubnet = (network: string) =>
  request<{ status: string }>(`/config/subnets/${encodeURIComponent(network)}`, { method: 'DELETE' })

// Reservations
export const v2GetReservations = (network: string) =>
  request<ReservationConfig[]>(`/config/subnets/${encodeURIComponent(network)}/reservations`)
export const v2CreateReservation = (network: string, res: ReservationConfig) =>
  request<ReservationConfig>(`/config/subnets/${encodeURIComponent(network)}/reservations`, { method: 'POST', body: JSON.stringify(res) })
export const v2DeleteReservation = (network: string, mac: string) =>
  request<{ status: string }>(`/config/subnets/${encodeURIComponent(network)}/reservations/${encodeURIComponent(mac)}`, { method: 'DELETE' })
export const v2ImportReservations = (network: string, data: ReservationConfig[] | FormData) => {
  if (data instanceof FormData) {
    return fetch(`${BASE}/config/subnets/${encodeURIComponent(network)}/reservations/import`, {
      method: 'POST', body: data, credentials: 'include',
    }).then(r => r.json())
  }
  return request<{ imported: number; added: number; updated: number }>(
    `/config/subnets/${encodeURIComponent(network)}/reservations/import`,
    { method: 'POST', body: JSON.stringify(data) },
  )
}

// Singleton config sections
export const v2GetDefaults = () => request<DefaultsConfig>('/config/defaults')
export const v2SetDefaults = (d: DefaultsConfig) =>
  request<DefaultsConfig>('/config/defaults', { method: 'PUT', body: JSON.stringify(d) })

export const v2GetConflictConfig = () => request<ConflictDetectionConfig>('/config/conflict')
export const v2SetConflictConfig = (c: ConflictDetectionConfig) =>
  request<ConflictDetectionConfig>('/config/conflict', { method: 'PUT', body: JSON.stringify(c) })

export const v2GetHAConfig = () => request<HAConfigType>('/config/ha')
export const v2SetHAConfig = (h: HAConfigType) =>
  request<HAConfigType>('/config/ha', { method: 'PUT', body: JSON.stringify(h) })

export const v2GetHooksConfig = () => request<HooksConfigType>('/config/hooks')
export const v2SetHooksConfig = (h: HooksConfigType) =>
  request<HooksConfigType>('/config/hooks', { method: 'PUT', body: JSON.stringify(h) })

export const v2GetDDNSConfig = () => request<DDNSConfigType>('/config/ddns')
export const v2SetDDNSConfig = (d: DDNSConfigType) =>
  request<DDNSConfigType>('/config/ddns', { method: 'PUT', body: JSON.stringify(d) })

export const v2GetDNSConfig = () => request<DNSConfigType>('/config/dns')
export const v2SetDNSConfig = (d: DNSConfigType) =>
  request<DNSConfigType>('/config/dns', { method: 'PUT', body: JSON.stringify(d) })

export const v2ImportTOML = (toml: string) =>
  request<{ status: string; subnets: number }>('/config/import', { method: 'POST', body: JSON.stringify({ toml }) })
