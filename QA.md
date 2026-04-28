# Roodox QA Scripts

This repository now includes reusable local QA entrypoints for live regression, soak testing, fault injection, and restart recovery. These are intended to replace one-off terminal snippets and provide stable verification commands for future development and GUI integration.

## Go QA Tool

Core entrypoint:

- `go run ./cmd/roodox_qa live`
- `go run ./cmd/roodox_qa soak`
- `go run ./cmd/roodox_qa faults`
- `go run ./cmd/roodox_qa probe`

Common overrides:

- `-config`
- `-addr`
- `-root-dir`
- `-shared-secret`
- `-tls-root-cert`
- `-tls-server-name`
- `-server-id`

The tool loads `roodox.config.json` by default, derives the dial address and TLS root CA path from local server configuration, creates temporary QA artifacts under `root_dir/qa/...`, and cleans them up unless `-keep-artifacts` is specified.

## PowerShell Wrappers

Windows-friendly wrappers live under [`scripts/qa`](scripts/qa):

- [`run-live-regression.ps1`](scripts/qa/run-live-regression.ps1)
- [`run-fault-injection.ps1`](scripts/qa/run-fault-injection.ps1)
- [`run-soak.ps1`](scripts/qa/run-soak.ps1)
- [`run-restart-recovery.ps1`](scripts/qa/run-restart-recovery.ps1)
- [`run-full-qa.ps1`](scripts/qa/run-full-qa.ps1)

Examples:

```powershell
.\scripts\qa\run-live-regression.ps1
.\scripts\qa\run-fault-injection.ps1
.\scripts\qa\run-soak.ps1 -Duration 5m -Workers 6 -BuildInterval 30s
.\scripts\qa\run-restart-recovery.ps1 -PreSeconds 5 -DownSeconds 7 -PostSeconds 14
.\scripts\qa\run-restart-recovery.ps1 -KeepLogs -CaptureRestartServerLogs
.\scripts\qa\run-full-qa.ps1 -SoakDuration 3m
```

`run-restart-recovery.ps1` now deletes probe logs on success by default and restarts the long-running server process without binding it to `%TEMP%\roodox-qa` files. Use `-KeepLogs` to preserve probe output, and add `-CaptureRestartServerLogs` only when redirected startup logs are explicitly needed for debugging.

## Deployment Lifecycle Smoke

Reusable packaging and certificate lifecycle smoke validation lives under [`scripts/server/validate-deployment-lifecycle.ps1`](scripts/server/validate-deployment-lifecycle.ps1).

Default fixture config:

- [`testdata/deployment-smoke/roodox-smoke.config.json`](testdata/deployment-smoke/roodox-smoke.config.json)

Example:

```powershell
.\scripts\server\validate-deployment-lifecycle.ps1 -Rebuild
```

This validates, in an isolated deployment directory:

- install snapshot creation
- TLS root export for client handoff
- leaf-only certificate rotation during upgrade
- rollback restoring the pre-upgrade deployment snapshot

## Coverage Summary

`live` covers:

- TLS and shared-secret connection
- gRPC health
- runtime and observability admin APIs
- device registration and config pull
- heartbeat and sync-state reporting
- file write, range write, read, lock, history, version lookup
- remote build
- device list query
- manual backup trigger

`soak` covers:

- mixed concurrent file IO
- repeated history and lock calls
- periodic heartbeat and sync-state reporting
- admin runtime and observability polling
- optional backup trigger
- periodic remote builds

`faults` covers:

- wrong shared secret
- wrong TLS server name
- invalid TLS root certificate input
- stale version conflicts for `WriteFile`, `WriteFileRange`, `SetFileSize`
- missing build unit failure path
- unknown device control-plane errors

`probe` is used by restart recovery to verify:

- healthy service before restart
- actual connection failures during outage
- healthy recovery after restart
