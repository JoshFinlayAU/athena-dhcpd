# changelog

## v2.1.0

first release where HA actually replicates leases, the API can do TLS, and a
stack of conflict-detection and allocation races are gone. mostly bug fixes — if
you run HA or conflict detection, upgrade.

### high availability
- leases now replicate between peers. the peer plumbing existed but was never
  wired up, so after a failover the standby served from an empty lease store and
  handed out addresses that were already in use. lease changes now stream to the
  peer (asynchronously, off the DHCP path), apply with last-write-wins, and the
  active node bulk-syncs its whole table when the peer connects.
- dual-active is resolved deterministically. each node carries a promotion epoch
  and announces a claim when it goes active; the lower epoch steps down, ties go
  to the primary, so you don't end up with two actives — or, worse, none. this
  converges the cluster once the peers can talk again; it does not stop both
  nodes serving during a real network partition (that needs a quorum witness,
  which isn't here yet — divergent leases reconcile by last-write-wins on heal).
- the lease sequence counter is restored from the store on restart instead of
  resetting to 0, so it can't reissue numbers already replicated to the peer.

### security
- the API can serve TLS. the `[api.tls]` block was being silently ignored, so
  logins, the bearer token and the session cookie all went over plaintext. the
  session cookie's Secure flag is forced on when TLS is active.
- auth fails closed once setup is complete. an empty credential set used to grant
  admin to everyone; that's now allowed only during first-boot setup.
- login throttling: 5 failed attempts per client per minute trips a 5-minute
  lockout (429 + Retry-After).
- bearer/query token comparison is constant-time, and a failed CSPRNG read no
  longer produces a guessable session id.
- audit CSV export escapes spreadsheet formulas. DHCP-supplied fields (hostname,
  client-id, circuit-id) starting with `=`, `+`, `-`, `@` could execute when the
  export was opened in a spreadsheet.
- a spoofed RELEASE can no longer free another client's lease, and a REQUEST for
  an address leased to someone else is NAK'd instead of hijacking it.

### dhcp and allocation
- two clients can no longer be offered the same address. the requested-IP and
  conflict-probe paths claimed addresses non-atomically; allocation is now atomic
  (`AllocateSpecific` and the new `ReserveN`).
- expired leases return their address to the pool. they were deleted from the
  store but the pool bit was never cleared, so pools slowly leaked until restart.
- the per-MAC and global rate limiter is wired into DISCOVER/REQUEST handling
  (it existed but was never called). exposes `dhcp_rate_limited_total`.
- the packet handler's config, pool, detector and HA fields are mutex-guarded —
  config reload and failover raced reads from the listener goroutines.

### conflict detection
- ICMP probes each use their own socket. they shared one, so concurrent probes
  overwrote each other's deadline and read each other's replies, occasionally
  reporting a used address as clear.
- ARP probing reports itself unavailable instead of pretending to work. it was
  never actually implemented — it opened a placeholder socket and returned
  "clear" for every address — so local-subnet duplicate detection silently did
  nothing. the detector now falls back to ICMP, which works on local subnets too.

### dns proxy
- zone overrides keep working after a config reload. the reload keyed them
  differently from the lookup, so every override silently stopped matching.
- upstream responses whose question doesn't match the query are no longer cached.

### internals
- go 1.25 in CI and the Dockerfile (go.mod already required it; builds were
  pinned to 1.22 and would have failed on a clean runner).
- CI checks gofmt and runs staticcheck; the tree is clean for both.
- ddns retry backoff wakes on shutdown instead of blocking it.
- web dependencies bumped (dependabot npm group).
