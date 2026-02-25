import type { APIConfig, UserConfig } from '@/lib/configTypes'
import { Section, FieldGrid, Field, TextInput, Toggle, Select } from '@/components/FormFields'
import { Plus, Trash2 } from 'lucide-react'

function emptyUser(): UserConfig {
  return { username: '', password_hash: '', role: 'viewer' }
}

export default function APISection({ value, onChange }: {
  value: APIConfig
  onChange: (v: APIConfig) => void
}) {
  const set = <K extends keyof APIConfig>(key: K, val: APIConfig[K]) =>
    onChange({ ...value, [key]: val })

  const users = value.auth?.users || []

  return (
    <Section title="API & Web UI" description="HTTP API server, authentication, and embedded web interface">
      <Toggle
        checked={value.enabled}
        onChange={v => set('enabled', v)}
        label="Enable API Server"
        description="REST API + Web UI on the configured listen address"
      />

      {value.enabled && (
        <>
          <FieldGrid>
            <Field label="Listen Address" hint="ip:port">
              <TextInput value={value.listen} onChange={v => set('listen', v)} placeholder="0.0.0.0:8080" mono />
            </Field>
          </FieldGrid>

          <Toggle
            checked={value.web_ui}
            onChange={v => set('web_ui', v)}
            label="Enable Web UI"
            description="Serve the embedded React management interface"
          />

          {/* Auth */}
          <div className="pt-3 border-t border-border/50">
            <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider mb-3">Authentication</h4>
            <Field label="API Bearer Token" hint="for programmatic API access">
              <TextInput value={value.auth?.auth_token || ''} onChange={v => set('auth', { ...value.auth, auth_token: v })} placeholder="my-secret-token" />
            </Field>

            <div className="mt-4">
              <div className="flex items-center justify-between mb-3">
                <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Web UI Users ({users.length})</h4>
                <button type="button" onClick={() => set('auth', { ...value.auth, users: [...users, emptyUser()] })}
                  className="flex items-center gap-1 text-xs text-accent hover:text-accent-hover transition-colors">
                  <Plus className="w-3 h-3" /> Add User
                </button>
              </div>
              <div className="space-y-3">
                {users.map((u, i) => (
                  <div key={i} className="border border-border/50 rounded-lg p-4 bg-surface/50">
                    <div className="flex items-center justify-between mb-3">
                      <span className="text-xs font-semibold text-accent">{u.username || 'New User'}</span>
                      <button type="button" onClick={() => set('auth', { ...value.auth, users: users.filter((_, idx) => idx !== i) })}
                        className="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors">
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                    <FieldGrid>
                      <Field label="Username">
                        <TextInput value={u.username} onChange={v => { const n = [...users]; n[i] = { ...u, username: v }; set('auth', { ...value.auth, users: n }) }} placeholder="admin" />
                      </Field>
                      <Field label="Password Hash" hint="bcrypt">
                        <TextInput value={u.password_hash} onChange={v => { const n = [...users]; n[i] = { ...u, password_hash: v }; set('auth', { ...value.auth, users: n }) }} placeholder="$2y$10$..." mono />
                      </Field>
                      <Field label="Role">
                        <Select value={u.role} onChange={v => { const n = [...users]; n[i] = { ...u, role: v }; set('auth', { ...value.auth, users: n }) }} options={[
                          { value: 'admin', label: 'Admin — full read/write' },
                          { value: 'viewer', label: 'Viewer — read-only' },
                        ]} />
                      </Field>
                    </FieldGrid>
                  </div>
                ))}
                {users.length === 0 && <p className="text-xs text-text-muted italic">No users configured — API is unauthenticated (not recommended for production)</p>}
              </div>
            </div>
          </div>

          {/* TLS */}
          <div className="pt-3 border-t border-border/50">
            <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider mb-3">TLS</h4>
            <Toggle
              checked={value.tls?.enabled || false}
              onChange={v => set('tls', { ...value.tls, enabled: v })}
              label="Enable HTTPS"
              description="Serve the API over TLS"
            />
            {value.tls?.enabled && (
              <FieldGrid>
                <Field label="Certificate File">
                  <TextInput value={value.tls.cert_file} onChange={v => set('tls', { ...value.tls, cert_file: v })} placeholder="/etc/athena-dhcpd/tls/api.crt" mono />
                </Field>
                <Field label="Key File">
                  <TextInput value={value.tls.key_file} onChange={v => set('tls', { ...value.tls, key_file: v })} placeholder="/etc/athena-dhcpd/tls/api.key" mono />
                </Field>
              </FieldGrid>
            )}
          </div>

          {/* Session */}
          <div className="pt-3 border-t border-border/50">
            <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider mb-3">Session</h4>
            <FieldGrid>
              <Field label="Cookie Name">
                <TextInput value={value.session?.cookie_name || ''} onChange={v => set('session', { ...value.session, cookie_name: v })} placeholder="athena_session" />
              </Field>
              <Field label="Session Expiry">
                <TextInput value={value.session?.expiry || ''} onChange={v => set('session', { ...value.session, expiry: v })} placeholder="24h" mono />
              </Field>
            </FieldGrid>
            <div className="mt-3">
              <Toggle
                checked={value.session?.secure || false}
                onChange={v => set('session', { ...value.session, secure: v })}
                label="Secure Cookie"
                description="Set the Secure flag — enable when using TLS"
              />
            </div>
          </div>
        </>
      )}
    </Section>
  )
}
