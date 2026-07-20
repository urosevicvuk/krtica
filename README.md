# krtica

> Still in early development, work in progress.

**krtica** (Serbian: _mole_) is a self-hosted reverse tunnel that exposes
services behind NAT/CGNAT to the public internet, built from scratch in Go. It
is a **learning project** (for now?) - a way to understand reverse tunnels, Go,
and network programming by building one end to end, not by reading about it.

A mole digs its tunnel _outward_, from underground to the surface - which is
exactly the topology: an agent behind NAT dials an outbound connection to a
public server, and public traffic flows back down that tunnel to your services.
No opened router ports, works behind CGNAT.

## Why this exists

To learn. I wanted to understand by building it, how a reverse tunnel actually
works: multiplexing many connections over one, carrying UDP sensibly, staying up
across drops, terminating TLS at an edge, getting better at Go and wiring it all
into Kubernetes.

Before you pull this and run it on anything serious, please keep in mind that
the reverse-tunnel space is full of excellent, mature tools, and krtica is a
study of the ideas they pioneered - with gratitude, not as a competitor:

- **[frp](https://github.com/fatedier/frp)** — the one krtica learns from most
  directly. Comprehensive, battle-tested, ~90k stars, and does far more than
  this project ever will (P2P/xtcp, KCP, bandwidth limits, port reuse, server
  plugins, HTTP vhosting, dashboards). If you need a real tunnel, reach for frp
  first.
- **[rathole](https://github.com/rathole-org/rathole)** — a lean, fast Rust take
  on the same idea; a lovely study in doing a lot with a little.
- **[ngrok](https://ngrok.com)** — the polished gold standard for developer
  ergonomics.
- **[Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)**
  — excellent and free for HTTP/HTTPS exposure. I just want to have more control
  over it, hence why I'm building my own.
- **[Tailscale](https://tailscale.com)** — the reference for private, key-based
  mesh networking (WireGuard under the hood).

If you need a tunnel in production, use one of those. krtica exists so I can
experiment and learn, if this project matures, I'd be happy to suggest it when
the time comes.

## How it works

```
  PUBLIC INTERNET
        │
┌───────▼─────────────────────────┐
│  krtica-server (the molehill)   │  
│  listeners, router, edge LB,    │  
│  TLS edge, control API          │ 
└───────▲─────────────────────────┘
        │ ONE persistent, multiplexed,
        │ encrypted tunnel — dug outward
┌───────┴─────────────────────────┐
│  krtica-agent (the mole)        │  
│  dials out, demuxes streams,    │ 
│  splices to local targets       │  
└─────────────────────────────────┘
```

## Scope — what it deliberately leaves out

To stay small and understandable, krtica does one job: arbitrary-protocol public
exposure and skips things other tools do well. These aren't criticisms of those
tools; they're just outside what this project is for:

- **HTTP-specific features** (virtual hosting, path rewriting, request
  pipelines) - frp's HTTP mode and Cloudflare Tunnel do this well.
- **Private mesh / overlay VPN** - that's Tailscale/WireGuard's job.
- **P2P / NAT hole-punching** - krtica is server-relayed only; frp's `xtcp`
  covers the P2P case.
- **In-cluster pod load balancing** - that's Cilium/Envoy; krtica only balances
  across tunnel endpoints at the edge.
- **A hosted multi-tenant SaaS** - self-hosted and single-tenant by design,
  maybe in the future I'll add a managed hosting option when this matures, but
  you will always be able to use this repo and self-host it on your own
  hardware.

As the project grows, we could delete some of the things from here and actually
implement them, but not in near future at least.

## Quick start

On the VPS (the molehill):

```sh
krtica server -c server.yaml
```

```yaml
# server.yaml
agent_listen: "0.0.0.0:7000"
token: "generate-something-long"
routes:
    - name: ssh
      listen: "0.0.0.0:2222"
    - name: dns
      listen: "0.0.0.0:5353"
      protocol: udp
```

In the homelab (the mole):

```sh
krtica agent -c agent.yaml
```

```yaml
# agent.yaml
name: homelab
server: "vps.example.dev:7000"
token: "generate-something-long"
services:
    - name: ssh
      target: "192.168.1.10:22"
    - name: dns
      target: "192.168.1.53:53"
```

Manage it live - no restarts:

```sh
krtica route add --name game --listen :25565 --token ...
krtica route list --token ...
krtica agents --token ...
krtica watch --token ...
```

See [config-examples/](config-examples/) for every knob (QUIC transport, L7/SNI
routes, ACME, allowlists, mTLS, limits, metrics)

See [deploy/k8s/](deploy/k8s/) for the `Tunnel` CRD + in-cluster agent.

## Development

NixOS-friendly: the flake pins the entire toolchain.

```sh
nix develop          # or `direnv allow` once if you use it
task --list          # all dev commands (Taskfile.yml)
task check           # fmt + lint + race tests - the pre-commit gate
task build           # binary → bin/krtica
```
