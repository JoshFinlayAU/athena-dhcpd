// Multi-format DHCP reservation parser.
// Supports: CSV, JSON, ISC dhcpd, dnsmasq, Kea, and MikroTik formats.

import type { ReservationConfig } from './api'

export type ImportFormat = 'auto' | 'csv' | 'json' | 'isc' | 'dnsmasq' | 'kea' | 'mikrotik'

export interface ParseResult {
  reservations: ReservationConfig[]
  format: ImportFormat
  errors: string[]
}

const MAC_RE = /([0-9a-fA-F]{2}[:\-]){5}[0-9a-fA-F]{2}/
const IP_RE = /\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b/

function normalizeMac(mac: string): string {
  return mac.replace(/-/g, ':').toLowerCase()
}

// --- Auto-detect format ---

export function detectFormat(text: string): ImportFormat {
  const trimmed = text.trim()
  if (trimmed.startsWith('[') || trimmed.startsWith('{')) return 'json'
  if (trimmed.includes('host ') && trimmed.includes('hardware ethernet')) return 'isc'
  if (trimmed.includes('"Dhcp4"') || trimmed.includes('"reservations"')) return 'kea'
  if (/^dhcp-host=/m.test(trimmed)) return 'dnsmasq'
  if (/\/ip dhcp-server lease/m.test(trimmed) || /add address=/m.test(trimmed)) return 'mikrotik'
  // Default to CSV if it has commas and looks tabular
  if (trimmed.includes(',')) return 'csv'
  return 'csv'
}

// --- Main entry point ---

export function parseReservations(text: string, format: ImportFormat = 'auto'): ParseResult {
  const detected = format === 'auto' ? detectFormat(text) : format
  switch (detected) {
    case 'csv': return parseCSV(text, detected)
    case 'json': return parseJSON(text, detected)
    case 'isc': return parseISC(text, detected)
    case 'dnsmasq': return parseDnsmasq(text, detected)
    case 'kea': return parseKea(text, detected)
    case 'mikrotik': return parseMikroTik(text, detected)
    default: return parseCSV(text, 'csv')
  }
}

// --- CSV ---
// Accepts: mac,ip,hostname  or  mac,ip  or  with header row

function parseCSV(text: string, format: ImportFormat): ParseResult {
  const errors: string[] = []
  const reservations: ReservationConfig[] = []
  const lines = text.trim().split('\n').filter(l => l.trim())

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (!line || line.startsWith('#')) continue

    // Split on comma or tab
    const parts = line.includes('\t') ? line.split('\t') : line.split(',')
    const fields = parts.map(f => f.trim().replace(/^["']|["']$/g, ''))

    // Skip header row
    if (i === 0 && /^(mac|MAC|hardware|hostname)/i.test(fields[0])) continue

    if (fields.length < 2) {
      errors.push(`Line ${i + 1}: need at least 2 fields (mac, ip)`)
      continue
    }

    // Try to find MAC and IP in any order
    let mac = '', ip = '', hostname = ''
    for (const f of fields) {
      if (!mac && MAC_RE.test(f)) mac = normalizeMac(f.match(MAC_RE)![0])
      else if (!ip && IP_RE.test(f)) ip = f.match(IP_RE)![1]
      else if (!hostname && f && !MAC_RE.test(f) && !IP_RE.test(f)) hostname = f
    }

    if (!mac || !ip) {
      errors.push(`Line ${i + 1}: could not find MAC and IP`)
      continue
    }

    reservations.push({ mac, ip, hostname: hostname || undefined })
  }

  return { reservations, format, errors }
}

// --- JSON ---
// Accepts: array of { mac, ip, hostname? } objects

function parseJSON(text: string, format: ImportFormat): ParseResult {
  const errors: string[] = []
  try {
    let data = JSON.parse(text.trim())

    // Handle Kea-style wrapper
    if (data && !Array.isArray(data) && data.reservations) {
      data = data.reservations
    }
    if (!Array.isArray(data)) {
      return { reservations: [], format, errors: ['JSON must be an array of reservation objects'] }
    }

    const reservations: ReservationConfig[] = []
    for (let i = 0; i < data.length; i++) {
      const r = data[i]
      const mac = r.mac || r.MAC || r['hw-address'] || r.hardware_address || ''
      const ip = r.ip || r.IP || r['ip-address'] || r.address || ''
      const hostname = r.hostname || r.Hostname || r.name || ''

      if (!mac || !ip) {
        errors.push(`Item ${i}: missing mac or ip`)
        continue
      }
      reservations.push({
        mac: normalizeMac(mac),
        ip,
        hostname: hostname || undefined,
        identifier: r.identifier || r.client_id || undefined,
        ddns_hostname: r.ddns_hostname || undefined,
      })
    }
    return { reservations, format, errors }
  } catch (e) {
    return { reservations: [], format, errors: [`Invalid JSON: ${e instanceof Error ? e.message : String(e)}`] }
  }
}

// --- ISC dhcpd ---
// host name { hardware ethernet aa:bb:cc:dd:ee:ff; fixed-address 10.0.0.1; }

function parseISC(text: string, format: ImportFormat): ParseResult {
  const errors: string[] = []
  const reservations: ReservationConfig[] = []

  // Match host blocks (multi-line)
  const hostBlockRe = /host\s+(\S+)\s*\{([^}]+)\}/gi
  let match
  while ((match = hostBlockRe.exec(text)) !== null) {
    const name = match[1]
    const body = match[2]

    const macMatch = body.match(/hardware\s+ethernet\s+([0-9a-fA-F:.\-]+)/i)
    const ipMatch = body.match(/fixed-address\s+([0-9.]+)/i)

    if (!macMatch || !ipMatch) {
      errors.push(`host "${name}": missing hardware ethernet or fixed-address`)
      continue
    }

    reservations.push({
      mac: normalizeMac(macMatch[1]),
      ip: ipMatch[1],
      hostname: name.replace(/[_]/g, '-'),
    })
  }

  if (reservations.length === 0 && errors.length === 0) {
    errors.push('No host blocks found. Expected: host name { hardware ethernet ...; fixed-address ...; }')
  }

  return { reservations, format, errors }
}

// --- dnsmasq ---
// dhcp-host=aa:bb:cc:dd:ee:ff,10.0.0.1,hostname
// dhcp-host=aa:bb:cc:dd:ee:ff,set:tag,10.0.0.1,hostname,24h

function parseDnsmasq(text: string, format: ImportFormat): ParseResult {
  const errors: string[] = []
  const reservations: ReservationConfig[] = []
  const lines = text.trim().split('\n')

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (!line || line.startsWith('#')) continue

    const dhcpHostMatch = line.match(/^dhcp-host=(.+)$/i)
    if (!dhcpHostMatch) continue

    const parts = dhcpHostMatch[1].split(',').map(s => s.trim())

    let mac = '', ip = '', hostname = ''
    for (const p of parts) {
      if (!mac && MAC_RE.test(p)) mac = normalizeMac(p.match(MAC_RE)![0])
      else if (!ip && IP_RE.test(p)) ip = p.match(IP_RE)![1]
      else if (!hostname && p && !p.startsWith('set:') && !p.startsWith('tag:') && !/^\d+[smhd]$/.test(p) && !p.includes('*')) {
        hostname = p
      }
    }

    if (!mac || !ip) {
      errors.push(`Line ${i + 1}: could not extract MAC and IP from dhcp-host`)
      continue
    }

    reservations.push({ mac, ip, hostname: hostname || undefined })
  }

  if (reservations.length === 0 && errors.length === 0) {
    errors.push('No dhcp-host= lines found')
  }

  return { reservations, format, errors }
}

// --- Kea DHCP4 ---
// { "Dhcp4": { "subnet4": [{ "reservations": [{ "hw-address": "...", "ip-address": "..." }] }] } }

function parseKea(text: string, format: ImportFormat): ParseResult {
  const errors: string[] = []
  try {
    const data = JSON.parse(text.trim())
    const reservations: ReservationConfig[] = []

    // Try top-level reservations array
    let resArray: any[] = []
    if (Array.isArray(data)) {
      resArray = data
    } else if (data.reservations) {
      resArray = data.reservations
    } else if (data.Dhcp4) {
      // Full Kea config — extract from all subnet4 entries
      const subnets = data.Dhcp4?.subnet4 || data.Dhcp4?.['subnet4'] || []
      for (const sub of subnets) {
        if (sub.reservations) resArray.push(...sub.reservations)
      }
      // Also check global reservations
      if (data.Dhcp4.reservations) resArray.push(...data.Dhcp4.reservations)
    }

    for (let i = 0; i < resArray.length; i++) {
      const r = resArray[i]
      const mac = r['hw-address'] || r.mac || ''
      const ip = r['ip-address'] || r.ip || ''
      const hostname = r.hostname || ''

      if (!mac || !ip) {
        errors.push(`Reservation ${i}: missing hw-address or ip-address`)
        continue
      }
      reservations.push({
        mac: normalizeMac(mac),
        ip,
        hostname: hostname || undefined,
      })
    }

    if (reservations.length === 0 && errors.length === 0) {
      errors.push('No reservations found in Kea config')
    }

    return { reservations, format, errors }
  } catch (e) {
    return { reservations: [], format, errors: [`Invalid JSON: ${e instanceof Error ? e.message : String(e)}`] }
  }
}

// --- MikroTik ---
// /ip dhcp-server lease
// add address=10.0.0.1 mac-address=AA:BB:CC:DD:EE:FF comment=hostname server=dhcp1

function parseMikroTik(text: string, format: ImportFormat): ParseResult {
  const errors: string[] = []
  const reservations: ReservationConfig[] = []
  const lines = text.trim().split('\n')

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (!line || line.startsWith('#') || line.startsWith('/')) continue

    if (!line.startsWith('add ')) continue

    const addrMatch = line.match(/address=([0-9.]+)/)
    const macMatch = line.match(/mac-address=([0-9a-fA-F:]+)/i)
    const commentMatch = line.match(/comment=(\S+)/)

    if (!addrMatch || !macMatch) {
      errors.push(`Line ${i + 1}: missing address= or mac-address=`)
      continue
    }

    reservations.push({
      mac: normalizeMac(macMatch[1]),
      ip: addrMatch[1],
      hostname: commentMatch?.[1] || undefined,
    })
  }

  if (reservations.length === 0 && errors.length === 0) {
    errors.push('No "add address=... mac-address=..." lines found')
  }

  return { reservations, format, errors }
}

// --- Format descriptions for UI ---

export const FORMAT_INFO: Record<ImportFormat, { label: string; example: string }> = {
  auto: { label: 'Auto-detect', example: '' },
  csv: {
    label: 'CSV',
    example: 'mac,ip,hostname\naa:bb:cc:dd:ee:ff,192.168.1.10,server1\n11:22:33:44:55:66,192.168.1.11,printer',
  },
  json: {
    label: 'JSON',
    example: '[{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.10","hostname":"server1"}]',
  },
  isc: {
    label: 'ISC dhcpd',
    example: 'host server1 {\n  hardware ethernet aa:bb:cc:dd:ee:ff;\n  fixed-address 192.168.1.10;\n}',
  },
  dnsmasq: {
    label: 'dnsmasq',
    example: 'dhcp-host=aa:bb:cc:dd:ee:ff,192.168.1.10,server1',
  },
  kea: {
    label: 'Kea DHCP4',
    example: '{"reservations":[{"hw-address":"aa:bb:cc:dd:ee:ff","ip-address":"192.168.1.10"}]}',
  },
  mikrotik: {
    label: 'MikroTik',
    example: 'add address=192.168.1.10 mac-address=AA:BB:CC:DD:EE:FF comment=server1',
  },
}
