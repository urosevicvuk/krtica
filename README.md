# krtica

**krtica** (Serbian: *mole*) is a self-hosted reverse tunnel that exposes
services behind NAT/CGNAT to the public internet, built from scratch in Go.

A mole digs its tunnel *outward*, from underground to the surface — which is
exactly the topology: an agent behind NAT dials an outbound connection to a
public server, and public traffic flows back down that tunnel to your
services. No opened router ports, works behind CGNAT.

> **Status: early development.** Pre-v1; the design is settled
> ([docs/DESIGN.md](docs/DESIGN.md)), the code is being built phase by phase.
> Nothing is usable yet.

## Why another tunnel?

In one sentence: *a modern, Kubernetes-native reverse tunnel that stays up
under real load, does UDP properly, and treats tunnels as declarative
objects.* Every incumbent leaves a gap:

- **frp** — comprehensive, but documented stability trouble under heavy load.
- **rathole** — fast and lean, but semi-abandoned since 2023 with its HTTP
  control API permanently WIP.
- **ngrok** — polished but proprietary, bandwidth-capped, and still no UDP.
- **Cloudflare Tunnel** — excellent, and HTTP/HTTPS-centric by design.
- **Tailscale** — a private mesh, not public exposure.

krtica targets the intersection none of them own: **stable under load** +
**UDP with real datagram semantics** (QUIC datagrams, not UDP-taxed-over-TCP)
+ **a first-class dynamic control API (gRPC/REST)** + **tunnels as
Kubernetes/GitOps objects** — self-hosted and protocol-agnostic.

## How it works

```
        PUBLIC INTERNET
              │
      ┌───────▼──────────────────────┐
      │  krtica-server (the molehill)│  VPS with a public IP:
      │  listeners · router · edge LB│  routes ingress down the
      │  TLS edge · control API      │  right tunnel
      └───────▲──────────────────────┘
              │ ONE persistent, multiplexed,
              │ encrypted tunnel — dug outward
      ┌───────┴──────────────────────┐
      │  krtica-agent (the mole)     │  behind NAT/CGNAT:
      │  dials out · demuxes streams │  splices streams to
      │  → local targets             │  local services
      └──────────────────────────────┘
```

## What krtica will never be

Deliberate non-goals, so the lane stays sharp:

- **HTTP-only** — that's Cloudflare Tunnel. krtica is arbitrary protocols;
  L7 is a mode, never the identity.
- **A private mesh / overlay VPN** — that's Tailscale. krtica is public
  exposure.
- **An in-cluster load balancer** — that's Cilium/Envoy. krtica balances
  across tunnel endpoints at the edge only.
- **A full API gateway** — no auth/rate-limit/transform pipeline; light
  access control is the ceiling.
- **P2P / NAT hole-punching** — server-relayed only.
- **A hosted multi-tenant SaaS** — self-hosted, single-tenant.

## Roadmap

| Phase | Delivers | Milestone |
|---|---|---|
| 0 | Recon, scaffold, `Tunnel` carrier seam, CI | done |
| 1 | Core TCP tunnel: TLS, token auth, yamux | one homelab TCP port exposed publicly |
| 2 | Robustness + UDP + PROXY protocol | survives connection storms; forwards UDP |
| 3 | Dynamic control API, multi-service, edge LB | add/remove routes live, failover across agents |
| 4 | L7: SNI routing, ACME auto-TLS, access control | many HTTPS services sharing :443 |
| 5 | `Tunnel` CRD, QUIC transport, observability | tunnels as GitOps objects, visible in Grafana |

## Development

NixOS-friendly: the flake pins the entire toolchain.

```sh
nix develop          # or `direnv allow` once
go build ./...
go test -race ./...
golangci-lint run
```

Design questions are settled in [docs/DESIGN.md](docs/DESIGN.md) before they
are settled in code.
