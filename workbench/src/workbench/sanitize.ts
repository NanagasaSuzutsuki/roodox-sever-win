import {
  defaultConfig,
  emptyObservability,
  emptySnapshot,
  emptyTLS
} from "./defaults";
import type {
  AccessHostCandidate,
  AccessProviderInfo,
  AccessSetupResult,
  AppConfig,
  BackupTriggerResult,
  ConnectionCodeResult,
  EnvCheck,
  ExportClientAccessResult,
  ExportClientCAResult,
  IssueJoinBundleResult,
  ServerStatus,
  TLSStatus,
  ToolInfo,
  WorkbenchObservabilitySnapshot,
  WorkbenchSnapshot
} from "./types";

type JsonObject = Record<string, unknown>;

function asRecord(value: unknown): JsonObject {
  return value && typeof value === "object" ? value as JsonObject : {};
}

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function asString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function asOptionalString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function asNullableString(value: unknown): string | null | undefined {
  if (value === null) return null;
  return typeof value === "string" ? value : undefined;
}

function asBoolean(value: unknown, fallback = false): boolean {
  return typeof value === "boolean" ? value : fallback;
}

function asNumber(value: unknown, fallback = 0): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

function asStringArray(value: unknown): string[] {
  return asArray(value).filter((entry): entry is string => typeof entry === "string");
}

function sanitizeToolInfo(value: unknown): ToolInfo {
  const raw = asRecord(value);
  return {
    installed: asBoolean(raw.installed),
    path: asOptionalString(raw.path),
    version: asOptionalString(raw.version),
    error: asOptionalString(raw.error)
  };
}

function sanitizeAccessHostCandidate(value: unknown): AccessHostCandidate {
  const raw = asRecord(value);
  return {
    kind: asString(raw.kind),
    label: asString(raw.label),
    host: asString(raw.host),
    interface_alias: asNullableString(raw.interface_alias),
    source: asString(raw.source)
  };
}

function sanitizeAccessProviderInfo(value: unknown): AccessProviderInfo {
  const raw = asRecord(value);
  return {
    id: asString(raw.id),
    installed: asBoolean(raw.installed),
    version: asNullableString(raw.version),
    path: asNullableString(raw.path),
    host: asNullableString(raw.host)
  };
}

export function sanitizeAppConfig(value: unknown): AppConfig {
  const raw = asRecord(value);
  return {
    ...defaultConfig,
    addr: asString(raw.addr, defaultConfig.addr),
    data_root: asString(raw.data_root),
    root_dir: asString(raw.root_dir),
    remote_build_enabled: asBoolean(raw.remote_build_enabled),
    auth_enabled: asBoolean(raw.auth_enabled),
    shared_secret: asString(raw.shared_secret),
    tls_enabled: asBoolean(raw.tls_enabled),
    tls_cert_path: asString(raw.tls_cert_path, defaultConfig.tls_cert_path),
    tls_key_path: asString(raw.tls_key_path, defaultConfig.tls_key_path),
    bundle_default_device_group: asString(raw.bundle_default_device_group, defaultConfig.bundle_default_device_group),
    bundle_overlay_provider: asString(raw.bundle_overlay_provider),
    bundle_overlay_join_config_json: asString(raw.bundle_overlay_join_config_json, defaultConfig.bundle_overlay_join_config_json),
    bundle_service_mode: asString(raw.bundle_service_mode, defaultConfig.bundle_service_mode),
    bundle_service_host: asString(raw.bundle_service_host),
    bundle_service_port: Math.max(1, Math.trunc(asNumber(raw.bundle_service_port, defaultConfig.bundle_service_port))),
    bundle_use_tls: asBoolean(raw.bundle_use_tls),
    bundle_tls_server_name: asString(raw.bundle_tls_server_name)
  };
}

export function sanitizeServerStatus(value: unknown): ServerStatus {
  const raw = asRecord(value);
  return {
    running: asBoolean(raw.running),
    addr: asOptionalString(raw.addr),
    root_dir: asOptionalString(raw.root_dir),
    remote_build: typeof raw.remote_build === "boolean" ? raw.remote_build : undefined,
    last_error: asOptionalString(raw.last_error),
    installing: asBoolean(raw.installing)
  };
}

export function sanitizeEnvCheck(value: unknown): EnvCheck {
  const raw = asRecord(value);
  const tools = Object.fromEntries(
    Object.entries(asRecord(raw.tools)).map(([name, item]) => [name, sanitizeToolInfo(item)])
  );
  return {
    os: asString(raw.os),
    winget_installed: asBoolean(raw.winget_installed),
    tools
  };
}

export function sanitizeWorkbenchSnapshot(value: unknown): WorkbenchSnapshot {
  const raw = asRecord(value);
  const runtime = raw.runtime === null ? null : asRecord(raw.runtime);
  const runtimeRecord = runtime ?? {};
  const dbFile = asRecord(runtimeRecord.db_file);
  const walFile = asRecord(runtimeRecord.wal_file);
  const shmFile = asRecord(runtimeRecord.shm_file);
  const checkpoint = asRecord(runtimeRecord.checkpoint);
  const backup = asRecord(runtimeRecord.backup);
  return {
    ...emptySnapshot,
    runtime: runtime && Object.keys(runtime).length > 0
      ? {
          server_id: asString(runtimeRecord.server_id),
          listen_addr: asString(runtimeRecord.listen_addr),
          root_dir: asString(runtimeRecord.root_dir),
          db_path: asString(runtimeRecord.db_path),
          tls_enabled: asBoolean(runtimeRecord.tls_enabled),
          auth_enabled: asBoolean(runtimeRecord.auth_enabled),
          started_at_unix: asNumber(runtimeRecord.started_at_unix),
          health_state: asString(runtimeRecord.health_state),
          health_message: asString(runtimeRecord.health_message),
          db_file: {
            path: asString(dbFile.path),
            exists: asBoolean(dbFile.exists),
            size_bytes: asNumber(dbFile.size_bytes),
            modified_at_unix: asNumber(dbFile.modified_at_unix)
          },
          wal_file: {
            path: asString(walFile.path),
            exists: asBoolean(walFile.exists),
            size_bytes: asNumber(walFile.size_bytes),
            modified_at_unix: asNumber(walFile.modified_at_unix)
          },
          shm_file: {
            path: asString(shmFile.path),
            exists: asBoolean(shmFile.exists),
            size_bytes: asNumber(shmFile.size_bytes),
            modified_at_unix: asNumber(shmFile.modified_at_unix)
          },
          checkpoint: {
            last_checkpoint_at_unix: asNumber(checkpoint.last_checkpoint_at_unix),
            mode: asString(checkpoint.mode),
            busy_readers: asNumber(checkpoint.busy_readers),
            log_frames: asNumber(checkpoint.log_frames),
            checkpointed_frames: asNumber(checkpoint.checkpointed_frames),
            last_error: asString(checkpoint.last_error)
          },
          backup: {
            dir: asString(backup.dir),
            interval_seconds: asNumber(backup.interval_seconds),
            keep_latest: asNumber(backup.keep_latest),
            last_backup_at_unix: asNumber(backup.last_backup_at_unix),
            last_backup_path: asString(backup.last_backup_path),
            last_error: asString(backup.last_error)
          }
        }
      : null,
    devices: asArray(raw.devices).map((item) => {
      const device = asRecord(item);
      return {
        device_id: asString(device.device_id),
        display_name: asString(device.display_name),
        role: asString(device.role),
        overlay_provider: asString(device.overlay_provider),
        overlay_address: asString(device.overlay_address),
        online_state: asString(device.online_state),
        last_seen_at: asNumber(device.last_seen_at),
        sync_state: asString(device.sync_state),
        mount_state: asString(device.mount_state),
        client_version: asString(device.client_version),
        policy_revision: asNumber(device.policy_revision)
      };
    }),
    collected_at_unix: asNumber(raw.collected_at_unix),
    query_error: asNullableString(raw.query_error)
  };
}

export function sanitizeWorkbenchObservability(value: unknown): WorkbenchObservabilitySnapshot {
  const raw = asRecord(value);
  const build = asRecord(raw.build);
  return {
    ...emptyObservability,
    write_file_range_calls: asNumber(raw.write_file_range_calls),
    write_file_range_bytes: asNumber(raw.write_file_range_bytes),
    write_file_range_conflicts: asNumber(raw.write_file_range_conflicts),
    small_write_bursts: asNumber(raw.small_write_bursts),
    small_write_hot_paths: asArray(raw.small_write_hot_paths).map((item) => {
      const metric = asRecord(item);
      return {
        path: asString(metric.path),
        count: asNumber(metric.count)
      };
    }),
    build: {
      success_count: asNumber(build.success_count),
      failure_count: asNumber(build.failure_count),
      log_bytes: asNumber(build.log_bytes),
      queue_wait_count: asNumber(build.queue_wait_count),
      queue_wait_p50_ms: asNumber(build.queue_wait_p50_ms),
      queue_wait_p95_ms: asNumber(build.queue_wait_p95_ms),
      queue_wait_p99_ms: asNumber(build.queue_wait_p99_ms),
      duration_count: asNumber(build.duration_count),
      duration_p50_ms: asNumber(build.duration_p50_ms),
      duration_p95_ms: asNumber(build.duration_p95_ms),
      duration_p99_ms: asNumber(build.duration_p99_ms)
    },
    rpc_metrics: asArray(raw.rpc_metrics).map((item) => {
      const metric = asRecord(item);
      return {
        method: asString(metric.method),
        count: asNumber(metric.count),
        error_count: asNumber(metric.error_count),
        p50_ms: asNumber(metric.p50_ms),
        p95_ms: asNumber(metric.p95_ms),
        p99_ms: asNumber(metric.p99_ms)
      };
    }),
    collected_at_unix: asNumber(raw.collected_at_unix)
  };
}

export function sanitizeTLSStatus(value: unknown): TLSStatus {
  const raw = asRecord(value);
  return {
    ...emptyTLS,
    cert_path: asString(raw.cert_path),
    key_path: asString(raw.key_path),
    root_cert_path: asString(raw.root_cert_path),
    root_key_path: asString(raw.root_key_path),
    server_cert_exists: asBoolean(raw.server_cert_exists),
    server_key_exists: asBoolean(raw.server_key_exists),
    root_cert_exists: asBoolean(raw.root_cert_exists),
    root_key_exists: asBoolean(raw.root_key_exists),
    server_subject: asString(raw.server_subject),
    root_subject: asString(raw.root_subject),
    server_dns_names: asStringArray(raw.server_dns_names),
    server_not_before_unix: asNumber(raw.server_not_before_unix),
    server_not_after_unix: asNumber(raw.server_not_after_unix),
    root_not_before_unix: asNumber(raw.root_not_before_unix),
    root_not_after_unix: asNumber(raw.root_not_after_unix),
    root_is_ca: asBoolean(raw.root_is_ca),
    server_valid: asBoolean(raw.server_valid),
    root_valid: asBoolean(raw.root_valid),
    overall_valid: asBoolean(raw.overall_valid)
  };
}

export function sanitizeBackupTriggerResult(value: unknown): BackupTriggerResult {
  const raw = asRecord(value);
  return {
    created_at_unix: asNumber(raw.created_at_unix),
    path: asString(raw.path)
  };
}

export function sanitizeExportClientCAResult(value: unknown): ExportClientCAResult {
  const raw = asRecord(value);
  return {
    root_cert_path: asString(raw.root_cert_path),
    exported_path: asString(raw.exported_path)
  };
}

export function sanitizeIssueJoinBundleResult(value: unknown): IssueJoinBundleResult {
  const raw = asRecord(value);
  const bundle = asRecord(raw.bundle);
  return {
    bundle_json: asString(raw.bundle_json),
    bundle: {
      version: asNumber(bundle.version),
      overlay_provider: asString(bundle.overlay_provider),
      overlay_join_config_json: asString(bundle.overlay_join_config_json),
      service_discovery_mode: asString(bundle.service_discovery_mode),
      service_host: asString(bundle.service_host),
      service_port: asNumber(bundle.service_port),
      use_tls: asBoolean(bundle.use_tls),
      tls_server_name: asString(bundle.tls_server_name),
      server_id: asString(bundle.server_id),
      device_group: asString(bundle.device_group),
      shared_secret: asString(bundle.shared_secret),
      device_id: asString(bundle.device_id),
      device_name: asString(bundle.device_name),
      device_role: asString(bundle.device_role)
    }
  };
}

export function sanitizeExportClientAccessResult(value: unknown): ExportClientAccessResult {
  const raw = asRecord(value);
  return {
    export_dir: asString(raw.export_dir),
    bundle_path: asString(raw.bundle_path),
    ca_path: asNullableString(raw.ca_path),
    connection_code_path: asNullableString(raw.connection_code_path),
    importer_path: asNullableString(raw.importer_path),
    readme_path: asNullableString(raw.readme_path)
  };
}

export function sanitizeAccessSetupResult(value: unknown): AccessSetupResult {
  const raw = asRecord(value);
  return {
    computer_name: asString(raw.computer_name),
    lan_candidates: asArray(raw.lan_candidates).map(sanitizeAccessHostCandidate),
    recommended_lan_host: asNullableString(raw.recommended_lan_host),
    providers: asArray(raw.providers).map(sanitizeAccessProviderInfo)
  };
}

export function sanitizeConnectionCode(value: unknown): ConnectionCodeResult | null {
  const raw = asRecord(value);
  const format = asString(raw.format);
  const code = asString(raw.code);
  const uri = asString(raw.uri);
  const payloadSize = asNumber(raw.payload_size, Number.NaN);
  if (!format || !code || !uri || !Number.isFinite(payloadSize)) return null;
  return {
    format,
    code,
    uri,
    payload_size: Math.max(0, Math.trunc(payloadSize))
  };
}
