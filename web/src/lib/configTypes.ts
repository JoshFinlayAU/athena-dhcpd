// Mirrors internal/config/config.go structs for the visual config editor

export interface Config {
  server: ServerConfig
  conflict_detection: ConflictDetectionConfig
  ha: HAConfig
  hooks: HooksConfig
  ddns: DDNSConfig
  dns: DNSProxyConfig
  subnet: SubnetConfig[]
  defaults: DefaultsConfig
  api: APIConfig
}

export interface ServerConfig {
  interface: string
  bind_address: string
  server_id: string
  log_level: string
  lease_db: string
  pid_file: string
  rate_limit: RateLimitConfig
}

export interface RateLimitConfig {
  enabled: boolean
  max_discovers_per_second: number
  max_per_mac_per_second: number
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

export interface HAConfig {
  enabled: boolean
  role: string
  peer_address: string
  listen_address: string
  heartbeat_interval: string
  failover_timeout: string
  sync_batch_size: number
  tls: HATLSConfig
}

export interface HATLSConfig {
  enabled: boolean
  cert_file: string
  key_file: string
  ca_file: string
}

export interface HooksConfig {
  event_buffer_size: number
  script_concurrency: number
  script_timeout: string
  script: ScriptHook[]
  webhook: WebhookHook[]
}

export interface ScriptHook {
  name: string
  events: string[]
  command: string
  timeout: string
  subnets: string[]
}

export interface WebhookHook {
  name: string
  events: string[]
  url: string
  method: string
  headers: Record<string, string>
  timeout: string
  retries: number
  retry_backoff: string
  secret: string
  template: string
}

export interface DDNSConfig {
  enabled: boolean
  allow_client_fqdn: boolean
  fallback_to_mac: boolean
  ttl: number
  update_on_renew: boolean
  conflict_policy: string
  use_dhcid: boolean
  forward: DDNSZoneConfig
  reverse: DDNSZoneConfig
  zone_override: DDNSZoneOverride[]
}

export interface DDNSZoneConfig {
  zone: string
  method: string
  server: string
  tsig_name: string
  tsig_algorithm: string
  tsig_secret: string
  api_key: string
}

export interface DDNSZoneOverride {
  subnet: string
  forward_zone: string
  reverse_zone: string
  method: string
  server: string
  api_key: string
  tsig_name: string
  tsig_algorithm: string
  tsig_secret: string
}

export interface DNSProxyConfig {
  enabled: boolean
  listen_udp: string
  listen_doh: string
  doh_tls: { cert_file: string; key_file: string }
  domain: string
  ttl: number
  register_leases: boolean
  register_leases_ptr: boolean
  forwarders: string[]
  use_root_servers: boolean
  cache_size: number
  cache_ttl: string
  zone_override: DNSZoneOverrideConfig[]
  record: DNSStaticRecordConfig[]
  list: DNSListConfig[]
}

export interface DNSZoneOverrideConfig {
  zone: string
  nameserver: string
  doh: boolean
  doh_url: string
}

export interface DNSStaticRecordConfig {
  name: string
  type: string
  value: string
  ttl: number
}

export interface DNSListConfig {
  name: string
  url: string
  type: string
  format: string
  action: string
  enabled: boolean
  refresh_interval: string
}

export interface SubnetConfig {
  network: string
  routers: string[]
  dns_servers: string[]
  domain_name: string
  lease_time: string
  renewal_time: string
  rebind_time: string
  ntp_servers: string[]
  pool: PoolConfig[]
  reservation: ReservationConfig[]
  option: OptionConfig[]
}

export interface PoolConfig {
  range_start: string
  range_end: string
  lease_time: string
  match_circuit_id: string
  match_remote_id: string
  match_vendor_class: string
  match_user_class: string
}

export interface ReservationConfig {
  mac: string
  identifier: string
  ip: string
  hostname: string
  dns_servers: string[]
  ddns_hostname: string
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

export interface APIConfig {
  enabled: boolean
  listen: string
  web_ui: boolean
  auth: APIAuthConfig
  tls: APITLSConfig
  session: SessionConfig
}

export interface APIAuthConfig {
  auth_token: string
  users: UserConfig[]
}

export interface UserConfig {
  username: string
  password_hash: string
  role: string
}

export interface APITLSConfig {
  enabled: boolean
  cert_file: string
  key_file: string
}

export interface SessionConfig {
  cookie_name: string
  expiry: string
  secure: boolean
}

// Deep default factory
export function defaultConfig(): Config {
  return {
    server: {
      interface: 'eth0',
      bind_address: '0.0.0.0:67',
      server_id: '',
      log_level: 'info',
      lease_db: '/var/lib/athena-dhcpd/leases.db',
      pid_file: '/var/run/athena-dhcpd.pid',
      rate_limit: { enabled: false, max_discovers_per_second: 100, max_per_mac_per_second: 5 },
    },
    conflict_detection: {
      enabled: false,
      probe_strategy: 'sequential',
      probe_timeout: '500ms',
      max_probes_per_discover: 3,
      parallel_probe_count: 3,
      conflict_hold_time: '1h',
      max_conflict_count: 3,
      probe_cache_ttl: '10s',
      send_gratuitous_arp: false,
      icmp_fallback: false,
      probe_log_level: 'debug',
    },
    ha: {
      enabled: false,
      role: 'primary',
      peer_address: '',
      listen_address: '0.0.0.0:8067',
      heartbeat_interval: '1s',
      failover_timeout: '10s',
      sync_batch_size: 100,
      tls: { enabled: false, cert_file: '', key_file: '', ca_file: '' },
    },
    hooks: {
      event_buffer_size: 10000,
      script_concurrency: 4,
      script_timeout: '10s',
      script: [],
      webhook: [],
    },
    ddns: {
      enabled: false,
      allow_client_fqdn: false,
      fallback_to_mac: false,
      ttl: 300,
      update_on_renew: false,
      conflict_policy: 'overwrite',
      use_dhcid: false,
      forward: { zone: '', method: 'rfc2136', server: '', tsig_name: '', tsig_algorithm: 'hmac-sha256', tsig_secret: '', api_key: '' },
      reverse: { zone: '', method: 'rfc2136', server: '', tsig_name: '', tsig_algorithm: 'hmac-sha256', tsig_secret: '', api_key: '' },
      zone_override: [],
    },
    dns: {
      enabled: false,
      listen_udp: '0.0.0.0:53',
      listen_doh: '',
      doh_tls: { cert_file: '', key_file: '' },
      domain: '',
      ttl: 60,
      register_leases: true,
      register_leases_ptr: true,
      forwarders: [],
      use_root_servers: false,
      cache_size: 10000,
      cache_ttl: '5m',
      zone_override: [],
      record: [],
      list: [],
    },
    subnet: [],
    defaults: {
      lease_time: '12h',
      renewal_time: '6h',
      rebind_time: '10h30m',
      dns_servers: [],
      domain_name: '',
    },
    api: {
      enabled: false,
      listen: '0.0.0.0:8067',
      web_ui: false,
      auth: { auth_token: '', users: [] },
      tls: { enabled: false, cert_file: '', key_file: '' },
      session: { cookie_name: 'athena_session', expiry: '24h', secure: false },
    },
  }
}

// Deep merge parsed TOML into default config to fill missing fields
export function mergeWithDefaults(parsed: Record<string, unknown>): Config {
  const def = defaultConfig()
  return deepMerge(def, parsed) as Config
}

function deepMerge(target: unknown, source: unknown): unknown {
  if (source === null || source === undefined) return target
  if (Array.isArray(source)) return source
  if (typeof source === 'object' && typeof target === 'object' && !Array.isArray(target)) {
    const result: Record<string, unknown> = { ...(target as Record<string, unknown>) }
    for (const [key, val] of Object.entries(source as Record<string, unknown>)) {
      if (key in result && typeof result[key] === 'object' && typeof val === 'object' && !Array.isArray(val) && !Array.isArray(result[key])) {
        result[key] = deepMerge(result[key], val)
      } else {
        result[key] = val
      }
    }
    return result
  }
  return source
}
