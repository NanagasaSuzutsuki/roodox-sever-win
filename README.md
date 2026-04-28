# Roodox

Roodox is a Windows-first gRPC file service with device control-plane, TLS/auth handoff, build orchestration, and a Tauri-based operator workbench.

This repository contains:

- a Go server with file, sync, lock, version, build, and admin APIs
- a client connection library used by local tools and tests
- PowerShell deployment and lifecycle scripts
- a Tauri + React workbench for runtime operations and client handoff
- a join-bundle format for shipping client-facing connection metadata

## Current Scope

Roodox currently focuses on these areas:

- file and directory operations over gRPC
- optimistic version-aware writes, range writes, and truncate support
- device registration, heartbeat, mount/sync reporting, and diagnostics upload
- SQLite-backed runtime state, history, lock, and observability data
- TLS certificate inspection, rotation, and client CA export
- Windows process/service lifecycle management
- GUI-based operations and client access export

## Repository Layout

- `cmd/roodox_server`: server binary entrypoint
- `cmd/roodox_qa`: QA and regression tool
- `client/`: Go client helpers
- `internal/`: server, runtime, DB, cleanup, and control-plane packages
- `proto/`: protobuf and gRPC definitions
- `scripts/server/`: service lifecycle, TLS, backup, upgrade, rollback
- `scripts/qa/`: reusable QA wrappers
- `scripts/workbench/`: GUI launch and packaging
- `workbench/`: Tauri + React operator workbench

## Security Model

Roodox can run with:

- TLS enabled
- shared-secret authentication enabled
- client trust distributed as a CA root certificate

The intended client handoff baseline is:

- `host:port`
- `tls_enabled`
- `tls_server_name`
- exported client CA root
- shared secret when auth is enabled
- an optional join bundle containing overlay and device bootstrap metadata

## Quick Start

### 1. Prepare config

Create a local config from the example:

```powershell
Copy-Item .\roodox.config.example.json .\roodox.config.json
```

Then edit at least:

- `root_dir`
- `shared_secret`
- `tls_enabled`
- `control_plane.server_id`
- `control_plane.join_bundle.service_discovery.host`
- `control_plane.join_bundle.service_discovery.tls_server_name`

### 2. Start the server

The simplest local path is:

```powershell
.\scripts\server\start-server.ps1 -BuildIfMissing
```

Common lifecycle commands:

```powershell
.\scripts\server\status-server.ps1
.\scripts\server\restart-server.ps1 -Rebuild
.\scripts\server\stop-server.ps1
```

To run in the foreground instead:

```powershell
.\scripts\server\start-server.ps1 -Foreground -BuildIfMissing
```

### 3. Open the workbench

```powershell
.\scripts\workbench\start-gui.cmd
```

The workbench currently covers:

- runtime health and recent devices
- device inventory and overlay labels
- backup, TLS, and observability
- client access baseline, join-bundle preview, and access export
- local logs and config editing

## Join Bundle and Overlay Strategy

Roodox does not implement Tailscale or EasyTier itself.

Instead, Roodox treats the overlay as a separate network layer and ships overlay metadata through the join bundle so clients know:

- which overlay provider is expected
- which overlay bootstrap JSON should be consumed by the client bootstrap layer
- which Roodox service host, port, TLS, and auth values to use after the overlay is up

The join bundle payload includes:

- `overlay_provider`
- `overlay_join_config_json`
- `service_discovery_mode`
- `service_host`
- `service_port`
- `use_tls`
- `tls_server_name`
- `server_id`
- `device_group`
- `shared_secret`
- optional device identity fields

Server-side examples:

- issue bundle as JSON:

```powershell
.\roodox_server.exe -config .\roodox.config.json -issue-join-bundle-json
```

- export client CA:

```powershell
.\scripts\server\export-client-ca.ps1 -DestinationPath .\handoff\roodox-ca-cert.pem
```

### Direct / No Overlay

If clients connect directly to the server address:

```json
{
  "control_plane": {
    "join_bundle": {
      "overlay_provider": "direct",
      "overlay_join_config_json": "{}",
      "service_discovery": {
        "mode": "static",
        "host": "roodox.example.com",
        "port": 50051,
        "use_tls": true,
        "tls_server_name": "roodox.example.com"
      }
    }
  }
}
```

### Tailscale Usage

Recommended when you want private point-to-point reachability without exposing the gRPC endpoint directly to the public internet.

How Roodox uses it:

- set `overlay_provider` to `tailscale`
- put Tailscale-specific bootstrap data into `overlay_join_config_json`
- set `service_discovery.host` to a Tailscale IP or MagicDNS name reachable inside the tailnet
- keep `tls_server_name` aligned with the server certificate SAN, not just the overlay IP

Example:

```json
{
  "control_plane": {
    "join_bundle": {
      "overlay_provider": "tailscale",
      "overlay_join_config_json": "{\"tailnet\":\"example.ts.net\",\"hostname\":\"roodox-client-01\",\"authKey\":\"tskey-auth-kxxxxx\"}",
      "service_discovery": {
        "mode": "static",
        "host": "server-1.tailnet.ts.net",
        "port": 50051,
        "use_tls": true,
        "tls_server_name": "roodox.example.com"
      }
    }
  }
}
```

Practical notes:

- If you use MagicDNS for `service_discovery.host`, clients still need a certificate whose DNS name matches `tls_server_name`.
- If you terminate TLS on the Roodox server itself, distribute the exported CA root to clients.
- The overlay JSON is treated as opaque by Roodox. Your client bootstrap or launcher decides how to consume it.

### EasyTier Usage

Recommended when you want a user-managed overlay with explicit peer bootstrap and custom network topology.

How Roodox uses it:

- set `overlay_provider` to `easytier`
- put EasyTier bootstrap parameters into `overlay_join_config_json`
- set `service_discovery.host` to the address reachable after EasyTier is connected

Example:

```json
{
  "control_plane": {
    "join_bundle": {
      "overlay_provider": "easytier",
      "overlay_join_config_json": "{\"networkName\":\"roodox-prod\",\"peerTargets\":[\"tcp://overlay-gateway.example.com:11010\"],\"token\":\"replace-me\"}",
      "service_discovery": {
        "mode": "static",
        "host": "10.144.0.10",
        "port": 50051,
        "use_tls": true,
        "tls_server_name": "roodox.example.com"
      }
    }
  }
}
```

Practical notes:

- Treat `overlay_join_config_json` as the client bootstrap contract for EasyTier, not as a server-enforced schema.
- If clients connect through an overlay IP, keep `tls_server_name` pinned to the certificate identity you actually issued.
- The workbench access page can preview and export this bundle for handoff.

## Workbench Access Flow

The workbench has a dedicated access page that lets operators maintain:

- client-facing host and port
- TLS on/off and server name
- shared-secret visibility and export
- overlay provider and overlay bootstrap JSON
- device labels written into the exported bundle

Typical operator flow:

1. Set the client-facing host, port, TLS, and overlay fields.
2. Save access settings.
3. Refresh the join-bundle preview.
4. Export the client access bundle and CA root.

## TLS and Certificate Operations

Inspect current TLS material:

```powershell
.\scripts\server\certificate-status.ps1
```

Rotate the server leaf certificate:

```powershell
.\scripts\server\rotate-certificates.ps1 -RestartAfter
```

Rotate both root CA and leaf certificate:

```powershell
.\scripts\server\rotate-certificates.ps1 -RotateRootCA -RestartAfter
```

Important rules:

- leaf-only rotation keeps the same client trust root
- root CA rotation requires redistributing a new client CA
- clients should trust the exported CA root, not the leaf certificate

## GUI Build

Development requirements:

- Go `1.24`
- Node.js with `npm`
- Rust toolchain
- Tauri Windows build prerequisites for MSI packaging

Run the web build:

```powershell
cd .\workbench
npm install
npm run build
```

Build the packaged workbench:

```powershell
.\scripts\workbench\build-gui.cmd
```

## Testing

Run the full Go test suite:

```powershell
go test ./...
```

Run QA wrappers:

```powershell
.\scripts\qa\run-live-regression.ps1
.\scripts\qa\run-full-qa.ps1
```

See:

- [OPERATIONS.md](OPERATIONS.md)
- [QA.md](QA.md)
- [OPEN_SOURCE_SPLIT.md](OPEN_SOURCE_SPLIT.md)

## Status

This repository is currently centered on:

- server operations closure
- GUI productization
- client handoff material generation
- overlay-aware access packaging

License selection has not been finalized in this public export yet.
