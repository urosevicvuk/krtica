# krtica — Design

Public distillation of krtica's design reference. Section numbers (§) and
principle numbers (P#) are stable and cited in code comments and reviews.
When implementation questions arise, this document decides; when it is
wrong, it changes first, with a decision-log entry (§19).

## §1 Identity & positioning

krtica is a self-hosted reverse tunnel: an agent behind NAT/CGNAT dials one
persistent, encrypted, multiplexed connection out to a public server, which
routes public ingress back down the tunnel to local services.
Agent = the mole; server = the molehill.

**The lane (what krtica is NOT):** not HTTP-only (L7 is a mode); not a
private mesh/VPN; not an in-cluster load balancer; not an API gateway; not
P2P/hole-punching (server-relayed only); not a hosted SaaS. It coexists
with Cloudflare Tunnel (HTTP public exposure) and Tailscale (private
access); krtica's slice is *arbitrary-protocol public exposure*.

**Primary use cases:** expose homelab TCP/UDP services without opening
router ports; game servers / DNS / custom protocols that HTTP tunnels won't
carry; edge failover across multiple nodes; GitOps-declared tunnels;
blackbox "is my endpoint up from outside" telemetry.

## §2 Design principles

- **P1 — Stability under load is the product.** Bounded goroutines,
  backpressure, graceful degradation — never unbounded resource growth.
  Proven by benchmark, not asserted (§18).
- **P2 — Protocol-agnostic data plane.** Move bytes; TCP and UDP are both
  first-class. HTTP/L7 is an optional mode, never the identity.
- **P3 — Respect the transport.** No naive TCP-over-TCP meltdown: yamux
  over TCP+TLS by default, QUIC for lossy paths, UDP carried as datagrams.
- **P4 — Kubernetes-native, not Kubernetes-dependent.** Core runs anywhere
  as plain binaries; the CRD (§13) is a layer, not the substance.
- **P5 — GitOps-first.** Tunnels are declarative objects reconciled on
  boot/SIGHUP.
- **P6 — Dynamic by default.** Clean control API (gRPC + REST) from the
  start; add/remove/inspect live.
- **P7 — Observability built in.** Per-tunnel Prometheus metrics and
  blackbox endpoint health from Phase 1.
- **P8 — Secure by default.** Mandatory per-service tokens, encryption
  always on, no silent plaintext path.
- **P9 — Minimalism with a sharp edge.** Single static binary, few
  dependencies; every feature justifies itself against the lane.
- **P10 —** internal workflow principle (lives outside the repo).

## §3 System overview

```
        PUBLIC INTERNET
              │
      ┌───────▼─────────────────────────────┐
      │  krtica-server  (the molehill)      │  VPS w/ public IP
      │  ─ public listeners (:443, :2222…)  │
      │  ─ ROUTER: ingress → agent → target │  §6
      │  ─ edge load balancer + health      │  §7
      │  ─ TLS edge / ACME (L7 mode)        │  §9
      │  ─ control API (gRPC/REST)          │  §11
      └───────▲─────────────────────────────┘
              │
   ONE persistent, multiplexed, encrypted
   connection — the TUNNEL — dug outward     §4, §5
              │
      ┌───────┴─────────────────────────────┐
      │  krtica-agent  (the mole)           │  behind NAT/CGNAT
      │  ─ dials server, holds tunnel open  │
      │  ─ demuxes streams → local targets  │
      │  ─ optional: runs as k8s controller │  §13
      └───────┬─────────────────────────────┘
              │
     local services (k8s Service, bare process, NAS…)
```

**Data path:** public client connects → router resolves ingress →
(agent, target), edge LB picks a healthy agent → server opens a stream over
that agent's tunnel → agent dials the local target and splices bytes.

**Control path:** operator (CLI/TUI/CRD controller) → control API → routing
table updated live; declarative config reconciles the same state on
boot/SIGHUP.

**Lifecycle (§3.4):** dial → TLS → token/mTLS auth → capability negotiation;
keepalives both ways; per-stream flow control plus global caps — saturation
sheds deterministically instead of ballooning memory; on drop, agent
re-dials with jittered exponential backoff while the server marks targets
unhealthy until re-establishment.

## §4 Transport

Default carrier: **yamux over TCP + TLS** (proven, firewall-friendly;
hashicorp/yamux, not a custom muxer — Decision #3). Optional carrier (v2):
**QUIC** — sidesteps TCP-over-TCP on lossy paths and provides unreliable
datagrams, the semantically correct UDP carrier (§8.2).

**§4.3 Transport interface:** `OpenStream`, `AcceptStream`, `SendDatagram`,
`RecvDatagram`, `Close` — carriers are swappable without touching router or
forwarder. Implemented in `internal/transport`.

## §5 Multiplexing & streams

One tunnel carries many logical streams; a reserved control stream carries
agent↔server control messages. yamux windows give per-stream backpressure;
krtica adds global caps (max concurrent streams, max buffered bytes) — P1.
QUIC transport adds an out-of-band unreliable datagram channel.

## §6 Router

Routing table: ingress key → backend, where the key is an L4 listener
(`:2222`) or an L7 host/SNI (`ssh.example.dev`, share :443). L4 first;
L7 arrives as a mode in Phase 4. The table mutates live via the control
API — no restarts to add a tunnel.

## §7 Edge load balancing & health

When multiple agents advertise the same service, the server distributes
public connections across them: round-robin (v1), least-connections (v2).
Health = tunnel liveness (heartbeat) + optional target probes through the
tunnel; unhealthy backends leave rotation and return on recovery. Edge-only:
krtica never balances pods inside a cluster (Decision #10).

## §8 Data forwarding

- **§8.1 TCP:** accept stream → dial target → bidirectional copy with
  half-close, deadlines, bounded buffers, graceful drain on shutdown.
- **§8.2 UDP:** session tracking keyed by (client addr, target) with idle
  eviction. v1 carries datagrams length-prefixed over a yamux stream —
  functional, but pays TCP's reliability tax (documented limitation).
  v2 carries them as QUIC unreliable datagrams — UDP semantics preserved,
  no head-of-line blocking. That difference is the point: latency-sensitive
  UDP (games, VoIP, WireGuard, DNS) over a reliable stream is the wrong
  tradeoff.
- **§8.3 PROXY protocol:** optionally prepend v1/v2 headers so backends see
  the real client IP. Off by default.

## §9 TLS edge & ACME (L7 mode)

In L7 mode the server terminates public HTTPS with automatic Let's Encrypt
certificates (via an ACME library, not hand-rolled — Decision #12). L4
tunnels pass encrypted bytes through untouched.

## §10 Auth & access control

- Agent↔server auth mandatory: static per-agent token (v1), mTLS bootstrap
  (v2).
- Per-service tokens mandatory — a misconfigured agent cannot open
  arbitrary relays.
- Optional public-endpoint controls: IP allowlists, mTLS-required tunnels,
  basic/forward-auth for L7. Off by default; access control is the ceiling
  (no full auth pipeline — the lane, §1).

## §11 Control API

gRPC (primary) + REST (convenience): `TunnelAdd/Remove/List/Inspect`,
`AgentList`, `Status`, and a `Watch` stream. CLI, TUI, and the CRD
controller are all clients of this one API. Privileged: token/mTLS-gated,
never exposed on public data listeners.

## §12 Observability

Prometheus per tunnel: connected, RTT, throughput, active streams/sessions,
errors/reconnects; server-wide totals and auth rejections. Because the
server sees every tunnel's liveness and RTT, blackbox "is my endpoint up,
from outside" comes free. Structured JSON logs. A Grafana dashboard ships
in-repo.

## §13 Kubernetes integration

A `Tunnel` CRD declares an exposure (target Service/port, ingress, protocol,
access rules); the agent runs as an in-cluster controller watching those
objects and advertising targets over the tunnel. Tunnels live in git and
sync via GitOps. The CRD is a convenience layer — the core stays k8s-free
(P4).

## §14 Client surface

Single binary: `krtica server`, `krtica agent`,
`krtica tunnel add|list|rm|inspect`, `krtica status`. Optional kubectl
plugin. v2: TUI and/or a small server-rendered web dashboard. Declarative
config file (TOML/YAML) with SIGHUP hot-reload for non-k8s use.

## §15 Feature ledger

**Tier 0 (table stakes):** TCP + UDP forwarding, per-service tokens,
always-on encryption, multiplexing, hot-reload config. **Tier 1 (the
differentiators):** stability under load; QUIC transport option; UDP with
datagram semantics; first-class dynamic control API. **Tier 2:** edge LB +
failover, ACME auto-TLS, L7 host/SNI routing, PROXY protocol, blackbox
observability, endpoint access control. **Tier 3:** `Tunnel` CRD /
GitOps-native tunnels, TUI/dashboard. (**§15.2** the competitor matrix
lives in the design vault; README carries the summary.)

## §16 The NEVER list

HTTP-only identity · private mesh/VPN · in-cluster service LB · full API
gateway · P2P/hole-punching · hosted multi-tenant SaaS · writing our own
multiplexer in v1 · hand-rolled ACME.

## §17 Roadmap

Each phase ends demoable: **P0** recon + scaffold + Transport seam + CI
(done) → **P1** core TCP tunnel (TLS, token auth, yamux, one service
end-to-end) → **P2** robustness (reconnect/backoff, backpressure, bounded
resources) + UDP + PROXY protocol → **P3** control API + multi-service +
edge LB/failover → **P4** L7 (SNI routing, ACME, access control) →
**P5** `Tunnel` CRD, QUIC, metrics/dashboard. v2 tier: least-connections
LB, custom muxer (exercise), web dashboard, multi-region exits, OIDC
forward-auth, WireGuard-over-krtica recipe.

## §18 Testing strategy

Load tests to the regime where incumbents degrade, asserting flat memory
and no latency cliff (the P1 proof, with charts); `tc netem` packet-loss
runs validating P3; chaos (kill server/agent, partition) asserting jittered
reconnect and clean failover; UDP session/eviction correctness and the
measured over-TCP tax vs QUIC datagrams; PROXY protocol end-to-end;
security (token rejection, mTLS, allowlists, control-API auth); real-client
sanity: SSH, a UDP DNS resolver, a raw TCP service.

## §19 Decision log (public)

Append-only; cite by number. (Some numbers are internal workflow decisions
and are omitted here.)

1. Naming: krtica = mole; the metaphor is the topology.
2. Server-relayed topology; no P2P hole-punching.
3. hashicorp/yamux in v1; custom muxer at most a v2 exercise.
4. Data-plane-first, k8s-agnostic core; CRD is a layer.
5. Stability under load is the headline differentiator.
6. UDP first-class, done correctly via QUIC datagrams (v2).
7. Dynamic control API (gRPC+REST) is core, from the start.
8. Mandatory per-agent and per-service tokens; encryption always on.
9. PROXY protocol support for client-IP preservation, opt-in.
10. Edge LB across agents only; never in-cluster balancing.
11. The stability claim is proven with load-test benchmarks vs frp.
12. ACME via library (certmagic/autocert), never hand-rolled.
13. Coexist with Cloudflare Tunnel and Tailscale; don't replace them.

## §20 Open questions

QUIC timing (v2 vs pull into Phase 5) · v1 UDP-over-stream framing and idle
eviction policy · agent enrollment: mTLS bootstrap vs Noise NK (one keypair,
no certificates) · CRD shape: single `Tunnel` vs `TunnelServer`+`Tunnel`
split · certmagic vs autocert · control-API binding defaults ·
multi-server/regional exits: v2+ or never.
