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

export interface VIPEntry {
  ip: string
  cidr: number
  interface: string
  label?: string
}

export interface VIPEntryStatus {
  ip: string
  cidr: number
  interface: string
  label?: string
  held: boolean
  on_local: boolean
  acquired_at?: string
  error?: string
}

export interface VIPGroupStatus {
  configured: boolean
  active: boolean
  entries?: VIPEntryStatus[]
}

export interface HAStatus {
  enabled: boolean
  role: string
  state: string
  peer_address: string
  peer_connected: boolean
  last_heartbeat: string
  is_standby: boolean
  primary_url?: string
  vip?: VIPGroupStatus
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
    pool?: string
    state?: string
  }
  conflict?: {
    ip: string
    subnet: string
    detection_method: string
    responder_mac: string
  }
  ha?: {
    old_role: string
    new_role: string
    peer_state?: string
  }
  rogue?: {
    server_ip: string
    server_mac?: string
    offered_ip?: string
    interface?: string
    count: number
  }
  reason?: string
}

// Auth
export interface AuthUser {
  authenticated: boolean
  username?: string
  role?: string
  auth_required: boolean
  needs_user_setup?: boolean
}

export const getMe = () => request<AuthUser>('/auth/me')
export const login = (username: string, password: string) =>
  request<{ username: string; role: string }>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
export const createUser = (username: string, password: string, role: string = 'admin') =>
  request<{ username: string; role: string; status: string }>('/auth/users', {
    method: 'POST',
    body: JSON.stringify({ username, password, role }),
  })
export const listUsers = () =>
  request<{ users: { username: string; role: string }[] }>('/auth/users')
export const deleteUser = (username: string) =>
  request<{ status: string }>(`/auth/users/${username}`, { method: 'DELETE' })
export const changePassword = (username: string, password: string) =>
  request<{ status: string }>('/auth/users', {
    method: 'POST',
    body: JSON.stringify({ username, password, role: 'admin' }),
  })
export const logout = () =>
  request<{ status: string }>('/auth/logout', { method: 'POST' })

// Backup & Restore
export const exportBackup = async (): Promise<Blob> => {
  const res = await fetch(`${BASE}/backup`, { credentials: 'include' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error || res.statusText)
  }
  return res.blob()
}
export const importBackup = (data: string) =>
  request<{ status: string; sections: string[] }>('/backup/restore', {
    method: 'POST',
    body: data,
  })

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

// Floating VIPs
export const getVIPs = () => request<VIPEntry[]>('/vips')
export const setVIPs = (entries: VIPEntry[]) =>
  request<VIPEntry[]>('/vips', { method: 'PUT', body: JSON.stringify(entries) })
export const getVIPStatus = () => request<VIPGroupStatus>('/vips/status')

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

export interface DDNSZoneType {
  zone: string; method: string; server: string; tsig_name: string; tsig_algorithm: string; tsig_secret: string; api_key: string
}

export interface DDNSZoneOverrideType {
  subnet: string; forward_zone: string; reverse_zone: string; method: string; server: string; api_key: string; tsig_name: string; tsig_algorithm: string; tsig_secret: string
}

export interface DDNSConfigType {
  enabled: boolean
  allow_client_fqdn: boolean
  fallback_to_mac: boolean
  ttl: number
  update_on_renew: boolean
  conflict_policy: string
  use_dhcid: boolean
  forward: DDNSZoneType
  reverse: DDNSZoneType
  zone_override?: DDNSZoneOverrideType[]
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
  doh_tls?: { cert_file?: string; key_file?: string }
  zone_override?: { zone: string; nameserver: string; doh: boolean; doh_url: string }[]
  record?: { name: string; type: string; value: string; ttl: number }[]
  list?: { name: string; url: string; type: string; format: string; action: string; enabled: boolean; refresh_interval: string }[]
}

export interface ScriptHookType {
  name: string
  events: string[]
  command: string
  timeout: string
  subnets?: string[]
}

export interface WebhookHookType {
  name: string
  events: string[]
  url: string
  method: string
  headers?: Record<string, string>
  timeout: string
  retries: number
  retry_backoff: string
  secret?: string
  template?: string
}

export interface HooksConfigType {
  event_buffer_size: number
  script_concurrency: number
  script_timeout: string
  script?: ScriptHookType[]
  webhook?: WebhookHookType[]
}

export const v2GetHooks = () => request<HooksConfigType>('/config/hooks')
export const v2SetHooks = (cfg: HooksConfigType) =>
  request<{ status: string }>('/config/hooks', { method: 'PUT', body: JSON.stringify(cfg) })

export interface HostnameSanitisationConfig {
  enabled: boolean
  allow_regex: string
  deny_patterns: string[]
  dedup_suffix: boolean
  max_length: number
  strip_emoji: boolean
  lowercase: boolean
  fallback_template: string
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

export const v2GetHostnameSanitisation = () => request<HostnameSanitisationConfig>('/config/hostname-sanitisation')
export const v2SetHostnameSanitisation = (h: HostnameSanitisationConfig) =>
  request<HostnameSanitisationConfig>('/config/hostname-sanitisation', { method: 'PUT', body: JSON.stringify(h) })

// Audit log
export interface AuditRecord {
  id: number
  timestamp: string
  event: string
  ip: string
  mac: string
  client_id: string
  hostname: string
  fqdn: string
  subnet: string
  pool: string
  lease_start: number
  lease_expiry: number
  circuit_id: string
  remote_id: string
  giaddr: string
  server_id: string
  ha_role: string
  reason: string
}

export interface AuditQueryResult {
  count: number
  records: AuditRecord[]
}

export interface AuditQueryParams {
  ip?: string
  mac?: string
  event?: string
  from?: string
  to?: string
  at?: string
  limit?: number
}

export const v2QueryAudit = (params: AuditQueryParams) => {
  const qs = new URLSearchParams()
  if (params.ip) qs.set('ip', params.ip)
  if (params.mac) qs.set('mac', params.mac)
  if (params.event) qs.set('event', params.event)
  if (params.from) qs.set('from', params.from)
  if (params.to) qs.set('to', params.to)
  if (params.at) qs.set('at', params.at)
  if (params.limit) qs.set('limit', String(params.limit))
  return request<AuditQueryResult>(`/audit?${qs.toString()}`)
}

export const v2AuditStats = () => request<{ total_records: number }>('/audit/stats')

export const v2AuditExportURL = (params: AuditQueryParams) => {
  const qs = new URLSearchParams()
  if (params.ip) qs.set('ip', params.ip)
  if (params.mac) qs.set('mac', params.mac)
  if (params.event) qs.set('event', params.event)
  if (params.from) qs.set('from', params.from)
  if (params.to) qs.set('to', params.to)
  if (params.at) qs.set('at', params.at)
  if (params.limit) qs.set('limit', String(params.limit))
  return `${BASE}/audit/export?${qs.toString()}`
}

// Device fingerprints
export interface DeviceFingerprint {
  mac: string
  fingerprint_hash: string
  vendor_class: string
  param_list: string
  hostname: string
  oui: string
  device_type: string
  device_name: string
  os: string
  confidence: number
  source: string
  first_seen: string
  last_seen: string
}

export interface FingerprintStats {
  total_devices: number
  by_type: Record<string, number>
  by_os: Record<string, number>
  has_api_key: boolean
}

export interface FingerprintConfig {
  enabled: boolean
  fingerbank_api_key: string
  fingerbank_url: string
}

export const v2GetFingerprints = () => request<DeviceFingerprint[]>('/fingerprints')
export const v2GetFingerprint = (mac: string) => request<DeviceFingerprint>(`/fingerprints/${encodeURIComponent(mac)}`)
export const v2GetFingerprintStats = () => request<FingerprintStats>('/fingerprints/stats')
export const v2GetFingerprintConfig = () => request<FingerprintConfig>('/config/fingerprint')
export const v2SetFingerprintConfig = (cfg: FingerprintConfig) => request<FingerprintConfig>('/config/fingerprint', { method: 'PUT', body: JSON.stringify(cfg) })

// Rogue DHCP server detection
export interface RogueServer {
  server_ip: string
  server_mac: string
  last_offer_ip: string
  last_client_mac: string
  interface: string
  first_seen: string
  last_seen: string
  count: number
  acknowledged: boolean
}

export const v2GetRogueServers = () => request<RogueServer[]>('/rogue')
export const v2GetRogueStats = () => request<{ total: number; active: number }>('/rogue/stats')
export const v2AcknowledgeRogue = (serverIP: string) =>
  request<{ status: string }>('/rogue/acknowledge', { method: 'POST', body: JSON.stringify({ server_ip: serverIP }) })
export const v2RemoveRogue = (serverIP: string) =>
  request<{ status: string }>('/rogue/remove', { method: 'POST', body: JSON.stringify({ server_ip: serverIP }) })
export const v2ScanRogue = () =>
  request<{ status: string; servers_found: number; servers: RogueServer[]; total: number }>('/rogue/scan', { method: 'POST' })

// Topology
export interface TopologyDevice {
  mac: string
  ip: string
  hostname: string
  subnet: string
  first_seen: string
  last_seen: string
}

export interface TopologyPort {
  circuit_id: string
  label: string
  first_seen: string
  last_seen: string
  devices: TopologyDevice[]
}

export interface TopologySwitch {
  id: string
  remote_id: string
  giaddr: string
  label: string
  first_seen: string
  last_seen: string
  ports: Record<string, TopologyPort>
}

export interface TopologyStats {
  switches: number
  ports: number
  devices: number
}

export const v2GetTopology = () => request<TopologySwitch[]>('/topology')
export const v2GetTopologyStats = () => request<TopologyStats>('/topology/stats')
export const v2SetTopologyLabel = (switchId: string, portId: string, label: string) =>
  request<{ status: string }>('/topology/label', { method: 'POST', body: JSON.stringify({ switch_id: switchId, port_id: portId, label }) })

// Anomaly detection / Network Weather
export interface SubnetWeather {
  subnet: string
  current_rate: number
  baseline_rate: number
  std_dev: number
  known_macs: number
  unknown_macs_recent: number
  last_activity: string
  silent_minutes: number
  anomaly_score: number
  anomaly_reason: string
  status: string
}

export const v2GetWeather = () => request<SubnetWeather[]>('/anomaly/weather')

export const v2ImportTOML = (toml: string) =>
  request<{ status: string; subnets: number }>('/config/import', { method: 'POST', body: JSON.stringify({ toml }) })

// Port Automation
export interface PortAutoAction {
  type: 'webhook' | 'log' | 'tag'
  url?: string
  method?: string
  headers?: Record<string, string>
  tag?: string
  vlan?: number
}

export interface PortAutoRule {
  name: string
  enabled: boolean
  priority: number
  mac_patterns?: string[]
  subnets?: string[]
  circuit_ids?: string[]
  remote_ids?: string[]
  device_types?: string[]
  actions: PortAutoAction[]
}

export interface PortAutoLeaseContext {
  mac: string
  ip: string
  hostname?: string
  subnet?: string
  circuit_id?: string
  remote_id?: string
  device_type?: string
  vendor?: string
}

export interface PortAutoMatchResult {
  rule: string
  actions: PortAutoAction[]
}

export const v2GetPortAutoRules = () => request<PortAutoRule[]>('/portauto/rules')
export const v2SetPortAutoRules = (rules: PortAutoRule[]) =>
  request<{ status: string }>('/portauto/rules', { method: 'PUT', body: JSON.stringify(rules) })
export const v2TestPortAutoRules = (ctx: PortAutoLeaseContext) =>
  request<PortAutoMatchResult[]>('/portauto/test', { method: 'POST', body: JSON.stringify(ctx) })

// --- Setup Wizard API ---

export interface SetupStatus {
  needs_setup: boolean
}

export interface SetupHARequest {
  mode: 'standalone' | 'ha'
  role?: 'primary' | 'secondary'
  peer_address?: string
  listen_address?: string
  tls_enabled?: boolean
  tls_ca?: string
  tls_cert?: string
  tls_key?: string
}

export interface SetupConfigRequest {
  defaults?: DefaultsConfig
  subnets?: SubnetConfig[]
  conflict_detection?: ConflictDetectionConfig
  dns?: DNSConfigType
  ddns?: DDNSConfigType
}

export const getSetupStatus = () => request<SetupStatus>('/setup/status')
export const setupHA = (req: SetupHARequest) =>
  request<{ status: string; mode: string; role?: string }>('/setup/ha', { method: 'POST', body: JSON.stringify(req) })
export const setupConfig = (req: SetupConfigRequest) =>
  request<{ status: string }>('/setup/config', { method: 'POST', body: JSON.stringify(req) })
export const setupComplete = () =>
  request<{ status: string; message: string }>('/setup/complete', { method: 'POST' })
