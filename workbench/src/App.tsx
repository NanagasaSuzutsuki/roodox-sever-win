import { useDeferredValue, useEffect, useMemo, useState } from "react";
import { invoke } from "@tauri-apps/api/core";

type Lang = "zh" | "en";
type ViewKey = "dashboard" | "devices" | "operations" | "access" | "logs" | "settings" | "security";
type DeviceFilter = "all" | "online" | "degraded" | "offline" | "mounted" | `role:${string}` | `overlay:${string}`;
type DeviceSort = "recent" | "name";

type AppConfig = {
  addr: string;
  data_root: string;
  root_dir: string;
  remote_build_enabled: boolean;
  build_tool_dirs: string[];
  required_build_tools: string[];
  auth_enabled: boolean;
  shared_secret: string;
  tls_enabled: boolean;
  tls_cert_path: string;
  tls_key_path: string;
  bundle_default_device_group: string;
  bundle_overlay_provider: string;
  bundle_overlay_join_config_json: string;
  bundle_service_mode: string;
  bundle_service_host: string;
  bundle_service_port: number;
  bundle_use_tls: boolean;
  bundle_tls_server_name: string;
};

type ServerStatus = {
  running: boolean;
  addr?: string;
  root_dir?: string;
  remote_build?: boolean;
  last_error?: string;
  installing?: boolean;
};

type ToolInfo = { installed: boolean; path?: string; version?: string; error?: string };
type EnvCheck = {
  os: string;
  winget_installed: boolean;
  tools: Record<string, ToolInfo>;
  recommended_tool_dirs: string[];
  config_tool_dirs: string[];
};

type FileStatSummary = { path: string; exists: boolean; size_bytes: number; modified_at_unix: number };
type CheckpointStatus = {
  last_checkpoint_at_unix: number;
  mode: string;
  busy_readers: number;
  log_frames: number;
  checkpointed_frames: number;
  last_error: string;
};
type BackupStatus = {
  dir: string;
  interval_seconds: number;
  keep_latest: number;
  last_backup_at_unix: number;
  last_backup_path: string;
  last_error: string;
};

type WorkbenchRuntime = {
  server_id: string;
  listen_addr: string;
  root_dir: string;
  db_path: string;
  tls_enabled: boolean;
  auth_enabled: boolean;
  started_at_unix: number;
  health_state: string;
  health_message: string;
  db_file: FileStatSummary;
  wal_file: FileStatSummary;
  shm_file: FileStatSummary;
  checkpoint: CheckpointStatus;
  backup: BackupStatus;
};

type DeviceSummary = {
  device_id: string;
  display_name: string;
  role: string;
  overlay_provider: string;
  overlay_address: string;
  online_state: string;
  last_seen_at: number;
  sync_state: string;
  mount_state: string;
  client_version: string;
  policy_revision: number;
};

type WorkbenchSnapshot = {
  runtime: WorkbenchRuntime | null;
  devices: DeviceSummary[];
  collected_at_unix: number;
  query_error?: string | null;
};

type HotPathMetric = { path: string; count: number };
type RPCMetric = { method: string; count: number; error_count: number; p50_ms: number; p95_ms: number; p99_ms: number };
type BuildObservability = {
  success_count: number;
  failure_count: number;
  log_bytes: number;
  queue_wait_count: number;
  queue_wait_p50_ms: number;
  queue_wait_p95_ms: number;
  queue_wait_p99_ms: number;
  duration_count: number;
  duration_p50_ms: number;
  duration_p95_ms: number;
  duration_p99_ms: number;
};
type WorkbenchObservabilitySnapshot = {
  write_file_range_calls: number;
  write_file_range_bytes: number;
  write_file_range_conflicts: number;
  small_write_bursts: number;
  small_write_hot_paths: HotPathMetric[];
  build: BuildObservability;
  rpc_metrics: RPCMetric[];
  collected_at_unix: number;
};

type TLSStatus = {
  cert_path: string;
  key_path: string;
  root_cert_path: string;
  root_key_path: string;
  server_cert_exists: boolean;
  server_key_exists: boolean;
  root_cert_exists: boolean;
  root_key_exists: boolean;
  server_subject: string;
  root_subject: string;
  server_dns_names: string[];
  server_not_before_unix: number;
  server_not_after_unix: number;
  root_not_before_unix: number;
  root_not_after_unix: number;
  root_is_ca: boolean;
  server_valid: boolean;
  root_valid: boolean;
  overall_valid: boolean;
};

type BackupTriggerResult = { created_at_unix: number; path: string };
type ExportClientCAResult = { root_cert_path: string; exported_path: string };
type JoinBundleRequest = { device_id: string; device_name: string; device_role: string; device_group: string };
type JoinBundleView = {
  version: number;
  overlay_provider: string;
  overlay_join_config_json: string;
  service_discovery_mode: string;
  service_host: string;
  service_port: number;
  use_tls: boolean;
  tls_server_name: string;
  server_id: string;
  device_group: string;
  shared_secret: string;
  device_id: string;
  device_name: string;
  device_role: string;
};
type IssueJoinBundleResult = { bundle_json: string; bundle: JoinBundleView };
type ExportClientAccessResult = { export_dir: string; bundle_path: string; ca_path?: string | null };

const defaultConfig: AppConfig = {
  addr: ":50051",
  data_root: "",
  root_dir: "",
  remote_build_enabled: true,
  build_tool_dirs: [],
  required_build_tools: ["cmake", "make", "build-essential"],
  auth_enabled: false,
  shared_secret: "",
  tls_enabled: false,
  tls_cert_path: "certs/roodox-server-cert.pem",
  tls_key_path: "certs/roodox-server-key.pem",
  bundle_default_device_group: "default",
  bundle_overlay_provider: "",
  bundle_overlay_join_config_json: "{}",
  bundle_service_mode: "static",
  bundle_service_host: "",
  bundle_service_port: 50051,
  bundle_use_tls: false,
  bundle_tls_server_name: ""
};

const emptySnapshot: WorkbenchSnapshot = { runtime: null, devices: [], collected_at_unix: 0, query_error: null };
const emptyObservability: WorkbenchObservabilitySnapshot = {
  write_file_range_calls: 0,
  write_file_range_bytes: 0,
  write_file_range_conflicts: 0,
  small_write_bursts: 0,
  small_write_hot_paths: [],
  build: { success_count: 0, failure_count: 0, log_bytes: 0, queue_wait_count: 0, queue_wait_p50_ms: 0, queue_wait_p95_ms: 0, queue_wait_p99_ms: 0, duration_count: 0, duration_p50_ms: 0, duration_p95_ms: 0, duration_p99_ms: 0 },
  rpc_metrics: [],
  collected_at_unix: 0
};
const emptyTLS: TLSStatus = {
  cert_path: "",
  key_path: "",
  root_cert_path: "",
  root_key_path: "",
  server_cert_exists: false,
  server_key_exists: false,
  root_cert_exists: false,
  root_key_exists: false,
  server_subject: "",
  root_subject: "",
  server_dns_names: [],
  server_not_before_unix: 0,
  server_not_after_unix: 0,
  root_not_before_unix: 0,
  root_not_after_unix: 0,
  root_is_ca: false,
  server_valid: false,
  root_valid: false,
  overall_valid: false
};

const toMultiline = (value: string[]) => value.join("\n");
const initialLang = (): Lang => {
  const saved = localStorage.getItem("roodox.workbench.lang");
  return saved === "zh" || saved === "en" ? saved : "zh";
};
const errorText = (error: unknown) => (error instanceof Error ? error.message : String(error));
const formatTime = (value: number | undefined, lang: Lang, fallback: string) =>
  !value || value <= 0 ? fallback : new Intl.DateTimeFormat(lang === "zh" ? "zh-CN" : "en-US", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }).format(new Date(value * 1000));
const formatNumber = (value: number | undefined, lang: Lang, fallback: string) =>
  value === undefined || value === null || Number.isNaN(value) ? fallback : new Intl.NumberFormat(lang === "zh" ? "zh-CN" : "en-US").format(value);
const formatBytes = (value: number | undefined, lang: Lang, fallback: string) => {
  if (value === undefined || value === null || Number.isNaN(value)) return fallback;
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = Math.abs(value);
  let idx = 0;
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024;
    idx += 1;
  }
  return `${new Intl.NumberFormat(lang === "zh" ? "zh-CN" : "en-US", { maximumFractionDigits: size >= 10 || idx === 0 ? 0 : 1 }).format(size)} ${units[idx]}`;
};
const formatMillis = (value: number | undefined, lang: Lang, fallback: string) =>
  value === undefined || value === null || Number.isNaN(value) ? fallback : value >= 1000 ? `${new Intl.NumberFormat(lang === "zh" ? "zh-CN" : "en-US", { maximumFractionDigits: value >= 10000 ? 0 : 1 }).format(value / 1000)} s` : `${formatNumber(value, lang, fallback)} ms`;
const formatInterval = (value: number | undefined, lang: Lang, fallback: string) => {
  if (!value || value <= 0) return fallback;
  if (value % 3600 === 0) return `${formatNumber(value / 3600, lang, fallback)} h`;
  if (value % 60 === 0) return `${formatNumber(value / 60, lang, fallback)} min`;
  return `${formatNumber(value, lang, fallback)} s`;
};
const isMountedState = (value: string) => ["mounted", "ready", "active"].includes(value.trim().toLowerCase());

export default function App() {
  const [lang, setLang] = useState<Lang>(initialLang);
  const [view, setView] = useState<ViewKey>("dashboard");
  const [config, setConfig] = useState<AppConfig>(defaultConfig);
  const [toolDirsText, setToolDirsText] = useState("");
  const [requiredToolsText, setRequiredToolsText] = useState("");
  const [status, setStatus] = useState<ServerStatus>({ running: false });
  const [snapshot, setSnapshot] = useState<WorkbenchSnapshot>(emptySnapshot);
  const [observability, setObservability] = useState<WorkbenchObservabilitySnapshot>(emptyObservability);
  const [tlsStatus, setTLSStatus] = useState<TLSStatus>(emptyTLS);
  const [env, setEnv] = useState<EnvCheck | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [message, setMessage] = useState("");
  const [deviceSearch, setDeviceSearch] = useState("");
  const [deviceFilter, setDeviceFilter] = useState<DeviceFilter>("all");
  const [deviceSort, setDeviceSort] = useState<DeviceSort>("recent");
  const [logFilter, setLogFilter] = useState("");
  const [exportPath, setExportPath] = useState("artifacts/handoff/roodox-ca-cert.pem");
  const [accessExportDir, setAccessExportDir] = useState("artifacts/handoff/client-access");
  const [joinRequest, setJoinRequest] = useState<JoinBundleRequest>({ device_id: "", device_name: "", device_role: "", device_group: "" });
  const [accessBundle, setAccessBundle] = useState<IssueJoinBundleResult | null>(null);
  const [showAccessSecret, setShowAccessSecret] = useState(false);
  const deferredSearch = useDeferredValue(deviceSearch);
  const t = (zh: string, en: string) => (lang === "zh" ? zh : en);
  const unknown = t("æœªçŸ¥", "Unknown");
  const none = t("æ— ", "None");
  const never = t("ä»Žæœª", "Never");
  const yes = t("æ˜¯", "Yes");
  const no = t("å¦", "No");
  const runtime = snapshot.runtime;
  const collator = useMemo(() => new Intl.Collator(lang === "zh" ? ["zh-Hans-CN-u-co-pinyin", "zh-CN", "en"] : ["en", "zh-Hans-CN-u-co-pinyin"], { sensitivity: "base", numeric: true }), [lang]);

  const statusText = useMemo(() => {
    const running = t("æœåŠ¡è¿è¡Œä¸­", "Server running");
    const stopped = t("æœåŠ¡æœªè¿è¡Œ", "Server stopped");
    const installing = status.installing ? ` Â· ${t("çŽ¯å¢ƒå®‰è£…ä¸­", "Installer running")}` : "";
    if (status.running) return `${running}${status.addr ? ` (${status.addr})` : ""}${installing}`;
    if (status.last_error) return `${stopped} Â· ${status.last_error}${installing}`;
    return `${stopped}${installing}`;
  }, [lang, status]);

  const deviceStats = useMemo(() => {
    let online = 0;
    let degraded = 0;
    let offline = 0;
    for (const device of snapshot.devices) {
      const state = device.online_state.trim().toLowerCase();
      if (state === "online") online += 1;
      else if (state === "offline") offline += 1;
      else degraded += 1;
    }
    return { total: snapshot.devices.length, online, degraded, offline };
  }, [snapshot.devices]);

  const recentDevices = useMemo(() => [...snapshot.devices].sort((a, b) => b.last_seen_at - a.last_seen_at).slice(0, 6), [snapshot.devices]);
  const roleFilters = useMemo(() => Array.from(new Set(snapshot.devices.map((d) => d.role.trim()).filter(Boolean))).sort(), [snapshot.devices]);
  const overlayFilters = useMemo(() => Array.from(new Set(snapshot.devices.map((d) => d.overlay_provider.trim()).filter(Boolean))).sort(), [snapshot.devices]);
  const filteredDevices = useMemo(() => {
    const keyword = deferredSearch.trim().toLowerCase();
    return snapshot.devices
      .filter((device) => {
        const state = device.online_state.trim().toLowerCase();
        if (deviceFilter === "online" || deviceFilter === "degraded" || deviceFilter === "offline") {
          if (state !== deviceFilter) return false;
        }
        if (deviceFilter === "mounted" && !isMountedState(device.mount_state)) return false;
        if (deviceFilter.startsWith("role:") && device.role.trim() !== deviceFilter.slice(5)) return false;
        if (deviceFilter.startsWith("overlay:") && device.overlay_provider.trim() !== deviceFilter.slice(8)) return false;
        if (!keyword) return true;
        return [device.display_name, device.device_id, device.role, device.overlay_provider, device.overlay_address].join(" ").toLowerCase().includes(keyword);
      })
      .sort((left, right) => {
        if (deviceSort === "recent" && right.last_seen_at !== left.last_seen_at) return right.last_seen_at - left.last_seen_at;
        return collator.compare(left.display_name || left.device_id, right.display_name || right.device_id);
      });
  }, [collator, deferredSearch, deviceFilter, deviceSort, snapshot.devices]);
  const filteredLogs = useMemo(() => {
    const keyword = logFilter.trim().toLowerCase();
    return keyword ? logs.filter((line) => line.toLowerCase().includes(keyword)) : logs;
  }, [logFilter, logs]);
  const files = [
    { label: t("æ•°æ®åº“æ–‡ä»¶", "Database file"), value: runtime?.db_file },
    { label: t("WAL æ–‡ä»¶", "WAL file"), value: runtime?.wal_file },
    { label: t("SHM æ–‡ä»¶", "SHM file"), value: runtime?.shm_file }
  ];
  const recommendedServerName = useMemo(() => tlsStatus.server_dns_names.find((name) => name && name !== "localhost") || tlsStatus.server_dns_names[0] || "", [tlsStatus.server_dns_names]);
  const accessPreview = accessBundle?.bundle;
  const effectiveServiceHost = accessPreview?.service_host || config.bundle_service_host || unknown;
  const effectiveServicePort = accessPreview?.service_port || config.bundle_service_port;
  const effectiveTLS = accessPreview?.use_tls ?? config.bundle_use_tls;
  const effectiveServerName = accessPreview?.tls_server_name || config.bundle_tls_server_name || recommendedServerName || none;
  const effectiveDeviceGroup = accessPreview?.device_group || joinRequest.device_group.trim() || config.bundle_default_device_group || none;
  const effectiveOverlayProvider = accessPreview?.overlay_provider || config.bundle_overlay_provider || none;
  const accessWarnings = useMemo(() => {
    const warnings: string[] = [];
    if (!config.bundle_overlay_provider.trim()) warnings.push(t("客户端接入的 Overlay Provider 还没配置，Join Bundle 现在发不出去。", "Overlay provider is not configured yet, so the join bundle cannot be issued."));
    if (!config.bundle_service_host.trim()) warnings.push(t("客户端接入地址 host 还是空的，需要填一个客户端真实可达的地址。", "The client-facing service host is empty. Set a host that clients can actually reach."));
    if (config.bundle_use_tls && !config.bundle_tls_server_name.trim() && !recommendedServerName) warnings.push(t("TLS 已开启，但还没有 tls_server_name，可先填证书 DNS 名称。", "TLS is enabled but tls_server_name is missing. Fill in a DNS name from the certificate."));
    if (config.auth_enabled && !config.shared_secret.trim()) warnings.push(t("共享密钥认证已开启，但 shared_secret 为空。", "Shared-secret auth is enabled but shared_secret is empty."));
    return warnings;
  }, [config.auth_enabled, config.bundle_overlay_provider, config.bundle_service_host, config.bundle_tls_server_name, config.bundle_use_tls, config.shared_secret, recommendedServerName, t]);

  const loadConfig = async () => {
    const cfg = await invoke<AppConfig>("load_config");
    setConfig(cfg);
    setToolDirsText(toMultiline(cfg.build_tool_dirs ?? []));
    setRequiredToolsText((cfg.required_build_tools ?? []).join(","));
  };
  const buildConfigForSave = (): AppConfig => ({
    ...config,
    build_tool_dirs: toolDirsText.split(/\r?\n/).map((s) => s.trim()).filter(Boolean),
    required_build_tools: requiredToolsText.split(/[\n,]/).map((s) => s.trim()).filter(Boolean),
    data_root: config.data_root.trim(),
    root_dir: config.root_dir.trim(),
    addr: config.addr.trim(),
    tls_cert_path: config.tls_cert_path.trim(),
    tls_key_path: config.tls_key_path.trim(),
    shared_secret: config.shared_secret.trim(),
    bundle_default_device_group: config.bundle_default_device_group.trim(),
    bundle_overlay_provider: config.bundle_overlay_provider.trim(),
    bundle_overlay_join_config_json: config.bundle_overlay_join_config_json.trim() || "{}",
    bundle_service_mode: config.bundle_service_mode.trim() || "static",
    bundle_service_host: config.bundle_service_host.trim(),
    bundle_service_port: Number.isFinite(config.bundle_service_port) && config.bundle_service_port > 0 ? Math.trunc(config.bundle_service_port) : 50051,
    bundle_tls_server_name: config.bundle_tls_server_name.trim()
  });
  const persistConfig = async (next: AppConfig, okText: string) => {
    const saved = await invoke<AppConfig>("save_config", { cfg: next });
    setConfig(saved);
    setToolDirsText(toMultiline(saved.build_tool_dirs ?? []));
    setRequiredToolsText((saved.required_build_tools ?? []).join(","));
    setMessage(okText);
  };
  const refreshStatus = async () => setStatus(await invoke<ServerStatus>("server_status"));
  const refreshSnapshot = async () => setSnapshot(await invoke<WorkbenchSnapshot>("load_workbench_snapshot"));
  const refreshObservability = async () => setObservability(await invoke<WorkbenchObservabilitySnapshot>("load_workbench_observability"));
  const refreshTLS = async () => setTLSStatus(await invoke<TLSStatus>("load_tls_status"));
  const refreshLogs = async () => setLogs(await invoke<string[]>("read_logs"));
  const refreshAccessPreview = async () => setAccessBundle(await invoke<IssueJoinBundleResult>("issue_join_bundle", { request: joinRequest }));
  const refreshOps = async () => {
    await refreshSnapshot();
    await refreshObservability();
    await refreshTLS();
  };
  const safeRun = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
    } catch (error) {
      setMessage(errorText(error));
    }
  };

  useEffect(() => {
    localStorage.setItem("roodox.workbench.lang", lang);
  }, [lang]);

  useEffect(() => {
    void safeRun(async () => {
      await loadConfig();
      const envResult = await invoke<EnvCheck>("check_environment");
      setEnv(envResult);
      if ((envResult.recommended_tool_dirs?.length ?? 0) > 0 && !toolDirsText.trim()) setToolDirsText(envResult.recommended_tool_dirs.join("\n"));
      await refreshStatus();
      await refreshSnapshot();
      await refreshObservability();
      await refreshTLS();
      await refreshLogs();
    });
  }, []);

  useEffect(() => {
    const timer = setInterval(() => {
      void (async () => {
        try {
          await refreshStatus();
          if (view === "dashboard" || view === "devices" || view === "operations") await refreshSnapshot();
          if (view === "operations" || view === "access") {
            await refreshObservability();
            await refreshTLS();
          }
          if (view === "logs") await refreshLogs();
        } catch {
        }
      })();
    }, 4000);
    return () => clearInterval(timer);
  }, [view]);

  useEffect(() => {
    if (view !== "access" || accessWarnings.length > 0 || accessBundle) return;
    void safeRun(refreshAccessPreview);
  }, [accessBundle, accessWarnings.length, view]);

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand-block">
          <p className="brand-kicker">{t("Roodox 工作台", "Roodox Workbench")}</p>
          <strong>{status.running ? t("服务运行中", "Server running") : t("服务未运行", "Server stopped")}</strong>
          <span>{runtime?.health_state || unknown}</span>
        </div>
        <nav className="main-nav">
          {[["dashboard", t("工作台", "Workbench")], ["devices", t("设备", "Devices")], ["operations", t("运维", "Operations")], ["access", t("接入", "Client access")], ["logs", t("日志", "Logs")], ["settings", t("设置", "Settings")], ["security", t("安全", "Security")]].map(([key, label]) => (
            <button key={key} className={`nav-link ${view === key ? "active" : ""}`} onClick={() => setView(key as ViewKey)}>{label}</button>
          ))}
        </nav>
        <div className="sidebar-foot"><span>{deviceStats.total} {t("设备", "devices")}</span><span>{deviceStats.online} {t("在线", "online")}</span></div>
      </aside>

      <main className="workspace">
        {message ? <div className="message-bar">{message}</div> : null}
        {view === "dashboard" ? (
          <>
            <section className="panel hero-panel">
              <div className="panel-head">
                <div>
                  <p className="eyebrow">{t("工作台", "Workbench")}</p>
                  <h1>{t("工作台状态", "Workbench Status")}</h1>
                  <p className="panel-subtitle">{t("本机服务控制、设备概览与运维入口", "Local server control, device overview, and operator entrypoints")}</p>
                </div>
                <div className="lang-box">
                  <label>{t("语言", "Language")}</label>
                  <select value={lang} onChange={(e) => setLang(e.target.value as Lang)}>
                    <option value="zh">中文</option>
                    <option value="en">English</option>
                  </select>
                </div>
              </div>
              <div className={`status-banner ${status.running ? "running" : "stopped"}`}>{statusText}</div>
              <div className="action-row">
                <button className="primary" onClick={() => void safeRun(async () => { await invoke("start_server"); await refreshStatus(); await refreshSnapshot(); setMessage(t("服务已启动", "Server started")); })}>{t("启动服务", "Start server")}</button>
                <button className="ghost" onClick={() => void safeRun(async () => { await invoke("stop_server"); await refreshStatus(); await refreshSnapshot(); setMessage(t("已请求停止服务", "Stop requested")); })}>{t("停止服务", "Stop server")}</button>
                <button className="secondary" onClick={() => void safeRun(async () => { await refreshStatus(); await refreshSnapshot(); })}>{t("刷新", "Refresh")}</button>
                <button className="secondary" onClick={() => setView("operations")}>{t("打开运维页", "Open operations")}</button>
              </div>
            </section>
            <section className="metric-grid">
              <article className="metric-card"><span>{t("设备总数", "Total devices")}</span><strong>{deviceStats.total}</strong></article>
              <article className="metric-card ok"><span>{t("在线设备", "Online")}</span><strong>{deviceStats.online}</strong></article>
              <article className="metric-card warn"><span>{t("异常设备", "Attention")}</span><strong>{deviceStats.degraded}</strong></article>
              <article className="metric-card muted"><span>{t("离线设备", "Offline")}</span><strong>{deviceStats.offline}</strong></article>
            </section>
            <section className="content-grid">
              <article className="panel">
                <div className="panel-head compact"><div><p className="eyebrow">{t("服务概览", "Runtime")}</p><h2>{t("服务概览", "Runtime Overview")}</h2></div></div>
                {snapshot.query_error ? <div className="inline-warning">{t("当前无法读取服务端管理快照", "Unable to read the admin snapshot")} · {snapshot.query_error}</div> : null}
                <dl className="detail-grid">
                  <div><dt>{t("监听地址", "Listen address")}</dt><dd>{runtime?.listen_addr || status.addr || unknown}</dd></div>
                  <div><dt>{t("共享根目录", "Share root")}</dt><dd>{runtime?.root_dir || config.root_dir || unknown}</dd></div>
                  <div><dt>{t("数据目录", "Data root")}</dt><dd>{config.data_root || none}</dd></div>
                  <div><dt>{t("数据库", "Database")}</dt><dd>{runtime?.db_path || unknown}</dd></div>
                  <div><dt>TLS</dt><dd>{(runtime?.tls_enabled ?? config.tls_enabled) ? yes : no}</dd></div>
                  <div><dt>{t("认证", "Auth")}</dt><dd>{(runtime?.auth_enabled ?? config.auth_enabled) ? yes : no}</dd></div>
                  <div><dt>{t("健康状态", "Health")}</dt><dd>{runtime?.health_state || unknown}</dd></div>
                  <div><dt>{t("健康说明", "Health message")}</dt><dd>{runtime?.health_message || none}</dd></div>
                  <div><dt>{t("启动时间", "Started at")}</dt><dd>{formatTime(runtime?.started_at_unix, lang, never)}</dd></div>
                  <div><dt>{t("采样时间", "Collected at")}</dt><dd>{formatTime(snapshot.collected_at_unix, lang, never)}</dd></div>
                </dl>
              </article>
              <article className="panel">
                <div className="panel-head compact"><div><p className="eyebrow">{t("设备管理", "Devices")}</p><h2>{t("最近活动设备", "Recently active devices")}</h2></div><button className="secondary" onClick={() => setView("devices")}>{t("打开设备页", "Open devices")}</button></div>
                {recentDevices.length === 0 ? <div className="empty-state">{t("当前没有已注册设备", "No registered devices yet")}</div> : (
                  <div className="device-list compact-list">
                    {recentDevices.map((device) => (
                      <button key={device.device_id} className="device-card compact-card" onClick={() => { setView("devices"); setDeviceSearch(device.display_name || device.device_id); }}>
                        <div className="device-card-head"><strong>{device.display_name || device.device_id}</strong><span className={`state-pill ${device.online_state.toLowerCase()}`}>{device.online_state || unknown}</span></div>
                        <div className="device-card-meta"><span>{device.role || none}</span><span>{formatTime(device.last_seen_at, lang, never)}</span></div>
                      </button>
                    ))}
                  </div>
                )}
              </article>
            </section>
          </>
        ) : null}
        {view === "devices" ? (
          <section className="panel device-page">
            <div className="panel-head"><div><p className="eyebrow">{t("设备", "Devices")}</p><h1>{t("设备管理", "Devices")}</h1><p className="panel-subtitle">{t("按最近活动或名称查看客户端，并用左侧标签快速分类。", "Inspect clients by recent activity or name, then narrow them with left-side tags.")}</p></div></div>
            <div className="device-shell">
              <aside className="filter-rail">
                <button className={`rail-tag ${deviceFilter === "all" ? "active" : ""}`} onClick={() => setDeviceFilter("all")}>{t("全部设备", "All devices")}</button>
                <button className={`rail-tag ${deviceFilter === "online" ? "active" : ""}`} onClick={() => setDeviceFilter("online")}>{t("在线", "Online")}</button>
                <button className={`rail-tag ${deviceFilter === "degraded" ? "active" : ""}`} onClick={() => setDeviceFilter("degraded")}>{t("异常", "Attention")}</button>
                <button className={`rail-tag ${deviceFilter === "offline" ? "active" : ""}`} onClick={() => setDeviceFilter("offline")}>{t("离线", "Offline")}</button>
                <button className={`rail-tag ${deviceFilter === "mounted" ? "active" : ""}`} onClick={() => setDeviceFilter("mounted")}>{t("已挂载", "Mounted")}</button>
                {roleFilters.length > 0 ? <p className="rail-group">{t("按角色", "By role")}</p> : null}
                {roleFilters.map((role) => <button key={role} className={`rail-tag ${deviceFilter === `role:${role}` ? "active" : ""}`} onClick={() => setDeviceFilter(`role:${role}`)}>{role}</button>)}
                {overlayFilters.length > 0 ? <p className="rail-group">{t("按网络", "By overlay")}</p> : null}
                {overlayFilters.map((overlay) => <button key={overlay} className={`rail-tag ${deviceFilter === `overlay:${overlay}` ? "active" : ""}`} onClick={() => setDeviceFilter(`overlay:${overlay}`)}>{overlay}</button>)}
              </aside>
              <div className="device-main">
                <div className="toolbar">
                  <input className="search-input" placeholder={t("搜索设备名、角色、地址", "Search by name, role, or address")} value={deviceSearch} onChange={(e) => setDeviceSearch(e.target.value)} />
                  <div className="segmented">
                    <button className={deviceSort === "recent" ? "active" : ""} onClick={() => setDeviceSort("recent")}>{t("最近活动", "Recent")}</button>
                    <button className={deviceSort === "name" ? "active" : ""} onClick={() => setDeviceSort("name")}>{t("名称", "Name")}</button>
                  </div>
                </div>
                {snapshot.query_error ? <div className="inline-warning">{t("当前无法读取服务端管理快照", "Unable to read the admin snapshot")} · {snapshot.query_error}</div> : null}
                {filteredDevices.length === 0 ? <div className="empty-state">{t("没有匹配的设备", "No matching devices")}</div> : (
                  <div className="device-list">
                    {filteredDevices.map((device) => (
                      <article key={device.device_id} className="device-card">
                        <div className="device-card-head"><div><strong>{device.display_name || device.device_id}</strong><p>{device.role || none}</p></div><span className={`state-pill ${device.online_state.toLowerCase()}`}>{device.online_state || unknown}</span></div>
                        <dl className="device-meta-grid">
                          <div><dt>{t("设备 ID", "Device ID")}</dt><dd>{device.device_id}</dd></div>
                          <div><dt>{t("客户端版本", "Client version")}</dt><dd>{device.client_version || none}</dd></div>
                          <div><dt>{t("网络", "Overlay")}</dt><dd>{device.overlay_provider || none}</dd></div>
                          <div><dt>{t("网络地址", "Overlay address")}</dt><dd>{device.overlay_address || none}</dd></div>
                          <div><dt>{t("最后活动", "Last activity")}</dt><dd>{formatTime(device.last_seen_at, lang, never)}</dd></div>
                          <div><dt>{t("策略版本", "Policy rev")}</dt><dd>{device.policy_revision}</dd></div>
                          <div><dt>{t("同步", "Sync")}</dt><dd>{device.sync_state || unknown}</dd></div>
                          <div><dt>{t("挂载", "Mount")}</dt><dd>{device.mount_state || unknown}</dd></div>
                        </dl>
                      </article>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </section>
        ) : null}
        {view === "operations" ? (
          <section className="panel">
            <div className="panel-head">
              <div><p className="eyebrow">{t("运维", "Operations")}</p><h1>{t("运维收口", "Operations")}</h1><p className="panel-subtitle">{t("把运行态、备份、TLS 与可观测性收进同一个工作台。", "Keep runtime, backup, TLS, and observability in one operator surface.")}</p></div>
              <div className="action-row compact-actions">
                <button className="secondary" onClick={() => void safeRun(refreshOps)}>{t("刷新运维状态", "Refresh operations")}</button>
                <button className="primary" onClick={() => void safeRun(async () => { const result = await invoke<BackupTriggerResult>("trigger_server_backup"); await refreshOps(); setMessage(`${t("手动备份已触发", "Manual backup triggered")}: ${formatTime(result.created_at_unix, lang, never)} · ${result.path}`); })}>{t("立即备份", "Trigger backup")}</button>
              </div>
            </div>
            <div className="ops-grid">
              <article className="subpanel">
                <div className="panel-head compact"><div><h2>{t("运行与备份", "Runtime and backup")}</h2></div></div>
                {snapshot.query_error ? <div className="inline-warning">{t("当前无法读取运行态", "Unable to read the runtime snapshot")} · {snapshot.query_error}</div> : null}
                <dl className="detail-grid">
                  <div><dt>{t("Checkpoint 模式", "Checkpoint mode")}</dt><dd>{runtime?.checkpoint.mode || none}</dd></div>
                  <div><dt>{t("最近 Checkpoint", "Last checkpoint")}</dt><dd>{formatTime(runtime?.checkpoint.last_checkpoint_at_unix, lang, never)}</dd></div>
                  <div><dt>{t("忙读者", "Busy readers")}</dt><dd>{formatNumber(runtime?.checkpoint.busy_readers, lang, none)}</dd></div>
                  <div><dt>{t("日志帧", "Log frames")}</dt><dd>{formatNumber(runtime?.checkpoint.log_frames, lang, none)}</dd></div>
                  <div><dt>{t("已落盘帧", "Checkpointed frames")}</dt><dd>{formatNumber(runtime?.checkpoint.checkpointed_frames, lang, none)}</dd></div>
                  <div><dt>{t("最近备份", "Last backup")}</dt><dd>{formatTime(runtime?.backup.last_backup_at_unix, lang, never)}</dd></div>
                  <div><dt>{t("备份目录", "Backup dir")}</dt><dd>{runtime?.backup.dir || none}</dd></div>
                  <div><dt>{t("备份间隔", "Backup interval")}</dt><dd>{formatInterval(runtime?.backup.interval_seconds, lang, none)}</dd></div>
                  <div><dt>{t("保留份数", "Keep latest")}</dt><dd>{formatNumber(runtime?.backup.keep_latest, lang, none)}</dd></div>
                  <div className="wide"><dt>{t("备份文件", "Backup file")}</dt><dd>{runtime?.backup.last_backup_path || none}</dd></div>
                  <div className="wide"><dt>{t("最近错误", "Last error")}</dt><dd>{runtime?.checkpoint.last_error || runtime?.backup.last_error || none}</dd></div>
                </dl>
                <div className="stack-list">
                  {files.map((file) => (
                    <article key={file.label} className="info-card">
                      <div className="info-card-head"><strong>{file.label}</strong><span className={`state-pill ${file.value?.exists ? "online" : "offline"}`}>{file.value?.exists ? yes : no}</span></div>
                      <dl className="detail-grid">
                        <div className="wide"><dt>{t("路径", "Path")}</dt><dd className="mono">{file.value?.path || none}</dd></div>
                        <div><dt>{t("大小", "Size")}</dt><dd>{formatBytes(file.value?.size_bytes, lang, none)}</dd></div>
                        <div><dt>{t("修改时间", "Modified at")}</dt><dd>{formatTime(file.value?.modified_at_unix, lang, never)}</dd></div>
                      </dl>
                    </article>
                  ))}
                </div>
              </article>
              <article className="subpanel">
                <div className="panel-head compact"><div><h2>TLS</h2><p className="form-note">{config.tls_enabled ? t("当前配置启用了 TLS。", "TLS is enabled in config.") : t("当前配置未启用 TLS，但仍可检查证书文件状态。", "TLS is disabled in config, but certificate files can still be inspected.")}</p></div></div>
                <div className="toolbar">
                  <input value={exportPath} onChange={(e) => setExportPath(e.target.value)} placeholder={t("导出路径", "Export path")} />
                  <button className="primary" onClick={() => void safeRun(async () => { const result = await invoke<ExportClientCAResult>("export_client_ca", { destinationPath: exportPath }); setExportPath(result.exported_path); await refreshTLS(); setMessage(`${t("客户端 CA 已导出", "Client CA exported")}: ${result.exported_path}`); })}>{t("导出客户端 CA", "Export client CA")}</button>
                </div>
                <div className="pill-strip">
                  <span className={`state-pill ${tlsStatus.overall_valid ? "online" : "offline"}`}>{t("整体有效", "Overall valid")}: {tlsStatus.overall_valid ? yes : no}</span>
                  <span className={`state-pill ${tlsStatus.server_valid ? "online" : "degraded"}`}>{t("服务端证书有效", "Server cert valid")}: {tlsStatus.server_valid ? yes : no}</span>
                  <span className={`state-pill ${tlsStatus.root_valid ? "online" : "degraded"}`}>{t("根证书有效", "Root cert valid")}: {tlsStatus.root_valid ? yes : no}</span>
                  <span className={`state-pill ${tlsStatus.root_is_ca ? "online" : "offline"}`}>{t("根证书为 CA", "Root cert is CA")}: {tlsStatus.root_is_ca ? yes : no}</span>
                </div>
                <dl className="detail-grid">
                  <div className="wide"><dt>{t("服务端证书", "Server cert")}</dt><dd className="mono">{tlsStatus.cert_path || config.tls_cert_path || none}</dd></div>
                  <div className="wide"><dt>{t("服务端私钥", "Server key")}</dt><dd className="mono">{tlsStatus.key_path || config.tls_key_path || none}</dd></div>
                  <div className="wide"><dt>{t("根证书", "Root cert")}</dt><dd className="mono">{tlsStatus.root_cert_path || none}</dd></div>
                  <div className="wide"><dt>{t("根私钥", "Root key")}</dt><dd className="mono">{tlsStatus.root_key_path || none}</dd></div>
                  <div><dt>{t("服务端主题", "Server subject")}</dt><dd>{tlsStatus.server_subject || none}</dd></div>
                  <div><dt>{t("根主题", "Root subject")}</dt><dd>{tlsStatus.root_subject || none}</dd></div>
                  <div><dt>{t("服务端有效期至", "Server expires")}</dt><dd>{formatTime(tlsStatus.server_not_after_unix, lang, never)}</dd></div>
                  <div><dt>{t("根证书有效期至", "Root expires")}</dt><dd>{formatTime(tlsStatus.root_not_after_unix, lang, never)}</dd></div>
                  <div className="wide"><dt>{t("服务端 DNS 名称", "Server DNS names")}</dt><dd>{tlsStatus.server_dns_names.length > 0 ? tlsStatus.server_dns_names.join(", ") : none}</dd></div>
                </dl>
              </article>
              <article className="subpanel ops-span-full">
                <div className="panel-head compact"><div><h2>{t("可观测性", "Observability")}</h2><p className="form-note">{t("采样时间", "Collected at")}: {formatTime(observability.collected_at_unix, lang, never)}</p></div></div>
                <section className="metric-grid ops-metric-grid">
                  <article className="metric-card"><span>{t("区间写次数", "Range writes")}</span><strong>{formatNumber(observability.write_file_range_calls, lang, none)}</strong></article>
                  <article className="metric-card"><span>{t("区间写字节", "Range write bytes")}</span><strong>{formatBytes(observability.write_file_range_bytes, lang, none)}</strong></article>
                  <article className="metric-card warn"><span>{t("区间写冲突", "Range write conflicts")}</span><strong>{formatNumber(observability.write_file_range_conflicts, lang, none)}</strong></article>
                  <article className="metric-card muted"><span>{t("小写突发", "Small write bursts")}</span><strong>{formatNumber(observability.small_write_bursts, lang, none)}</strong></article>
                  <article className="metric-card ok"><span>{t("构建成功", "Build success")}</span><strong>{formatNumber(observability.build.success_count, lang, none)}</strong></article>
                  <article className="metric-card warn"><span>{t("构建失败", "Build failure")}</span><strong>{formatNumber(observability.build.failure_count, lang, none)}</strong></article>
                </section>
                <dl className="detail-grid">
                  <div><dt>{t("构建日志字节", "Build log bytes")}</dt><dd>{formatBytes(observability.build.log_bytes, lang, none)}</dd></div>
                  <div><dt>{t("排队 P95", "Queue wait P95")}</dt><dd>{formatMillis(observability.build.queue_wait_p95_ms, lang, none)}</dd></div>
                  <div><dt>{t("构建耗时 P95", "Build duration P95")}</dt><dd>{formatMillis(observability.build.duration_p95_ms, lang, none)}</dd></div>
                  <div><dt>{t("样本数", "Sample count")}</dt><dd>{formatNumber(observability.build.duration_count, lang, none)}</dd></div>
                </dl>
                <div className="ops-details-grid">
                  <article className="info-card">
                    <div className="info-card-head"><strong>{t("热点路径", "Hot paths")}</strong></div>
                    {observability.small_write_hot_paths.length === 0 ? <div className="empty-state compact-empty">{t("当前没有热点路径数据", "No hot-path data yet")}</div> : <div className="table-list">{observability.small_write_hot_paths.map((item) => <div key={`${item.path}-${item.count}`} className="table-row"><span className="table-main mono">{item.path}</span><span>{formatNumber(item.count, lang, none)}</span></div>)}</div>}
                  </article>
                  <article className="info-card">
                    <div className="info-card-head"><strong>{t("RPC 延迟", "RPC latency")}</strong></div>
                    {observability.rpc_metrics.length === 0 ? <div className="empty-state compact-empty">{t("当前没有 RPC 指标", "No RPC metrics yet")}</div> : <div className="table-list">{observability.rpc_metrics.map((item) => <div key={item.method} className="table-row table-row-multi"><div className="table-main"><strong>{item.method}</strong><span>{t("次数", "Count")}: {formatNumber(item.count, lang, none)} · {t("错误", "Errors")}: {formatNumber(item.error_count, lang, none)}</span></div><div className="table-metrics"><span>P50: {formatMillis(item.p50_ms, lang, none)}</span><span>P95: {formatMillis(item.p95_ms, lang, none)}</span><span>P99: {formatMillis(item.p99_ms, lang, none)}</span></div></div>)}</div>}
                  </article>
                </div>
              </article>
            </div>
          </section>
        ) : null}
        {view === "access" ? (
          <section className="panel">
            <div className="panel-head">
              <div><p className="eyebrow">{t("接入", "Client access")}</p><h1>{t("客户端接入", "Client access")}</h1><p className="panel-subtitle">{t("把客户端连接地址、TLS、共享密钥、Join Bundle 和导出交付收口到一个可见入口。", "Keep client connection inputs, TLS, shared secret, join-bundle preview, and handoff export in one visible surface.")}</p></div>
              <div className="action-row compact-actions">
                <button className="secondary" onClick={() => void safeRun(async () => { await refreshTLS(); if (accessWarnings.length === 0) await refreshAccessPreview(); })}>{t("刷新接入信息", "Refresh access")}</button>
                <button className="primary" onClick={() => void safeRun(async () => { await persistConfig(buildConfigForSave(), t("接入设置已保存", "Access settings saved")); const result = await invoke<ExportClientAccessResult>("export_client_access_bundle", { request: joinRequest, destinationDir: accessExportDir }); setAccessExportDir(result.export_dir); setMessage(result.ca_path ? `${t("客户端接入包已导出", "Client access bundle exported")}: ${result.bundle_path} · CA: ${result.ca_path}` : `${t("客户端接入包已导出", "Client access bundle exported")}: ${result.bundle_path}`); await refreshTLS(); setAccessBundle(await invoke<IssueJoinBundleResult>("issue_join_bundle", { request: joinRequest })); })}>{t("导出客户端接入包", "Export access bundle")}</button>
              </div>
            </div>
            {accessWarnings.length > 0 ? <div className="stack-list">{accessWarnings.map((warning) => <div key={warning} className="inline-warning">{warning}</div>)}</div> : null}
            <div className="access-grid">
              <article className="subpanel">
                <div className="panel-head compact"><div><h2>{t("连接基线", "Connection baseline")}</h2><p className="form-note">{t("这是当前准备发给客户端的连接材料。", "These are the connection materials prepared for the client right now.")}</p></div></div>
                <div className="pill-strip">
                  <span className={`state-pill ${effectiveTLS ? "online" : "offline"}`}>TLS: {effectiveTLS ? yes : no}</span>
                  <span className={`state-pill ${config.auth_enabled ? "online" : "offline"}`}>{t("共享密钥认证", "Shared secret auth")}: {config.auth_enabled ? yes : no}</span>
                  <span className={`state-pill ${tlsStatus.root_valid ? "online" : "degraded"}`}>{t("客户端 CA", "Client CA")}: {tlsStatus.root_valid ? yes : no}</span>
                </div>
                <dl className="detail-grid">
                  <div><dt>{t("服务 host", "Service host")}</dt><dd>{effectiveServiceHost}</dd></div>
                  <div><dt>{t("服务端口", "Service port")}</dt><dd>{effectiveServicePort ? formatNumber(effectiveServicePort, lang, none) : none}</dd></div>
                  <div><dt>{t("TLS server name", "TLS server name")}</dt><dd>{effectiveServerName}</dd></div>
                  <div><dt>{t("默认设备组", "Default device group")}</dt><dd>{effectiveDeviceGroup}</dd></div>
                  <div><dt>{t("Overlay Provider", "Overlay provider")}</dt><dd>{effectiveOverlayProvider}</dd></div>
                  <div><dt>{t("服务发现模式", "Service discovery mode")}</dt><dd>{accessPreview?.service_discovery_mode || config.bundle_service_mode || none}</dd></div>
                  <div className="wide"><dt>{t("客户端 CA 路径", "Client CA path")}</dt><dd className="mono">{tlsStatus.root_cert_path || none}</dd></div>
                  <div className="wide"><dt>{t("共享密钥", "Shared secret")}</dt><dd className="mono">{config.auth_enabled ? (showAccessSecret ? (accessPreview?.shared_secret || config.shared_secret || none) : "••••••••") : none}</dd></div>
                </dl>
                <div className="action-row">
                  <button className="secondary" onClick={() => setShowAccessSecret((value) => !value)}>{showAccessSecret ? t("隐藏共享密钥", "Hide shared secret") : t("显示共享密钥", "Show shared secret")}</button>
                  {recommendedServerName ? <button className="secondary" onClick={() => setConfig((value) => ({ ...value, bundle_tls_server_name: recommendedServerName }))}>{t("使用证书 DNS 名称", "Use cert DNS name")}</button> : null}
                </div>
                <div className="toolbar">
                  <input value={accessExportDir} onChange={(e) => setAccessExportDir(e.target.value)} placeholder={t("导出目录", "Export directory")} />
                </div>
              </article>
              <article className="subpanel">
                <div className="panel-head compact"><div><h2>{t("接入设置", "Access settings")}</h2><p className="form-note">{t("这里维护客户端真正要连的外部地址，不是本机 GUI 的管理地址。", "Maintain the client-facing address here, not the local GUI admin address.")}</p></div></div>
                <div className="form-grid two">
                  <div><label>{t("默认设备组", "Default device group")}</label><input value={config.bundle_default_device_group} onChange={(e) => setConfig((value) => ({ ...value, bundle_default_device_group: e.target.value }))} /></div>
                  <div><label>{t("Overlay Provider", "Overlay provider")}</label><input value={config.bundle_overlay_provider} onChange={(e) => setConfig((value) => ({ ...value, bundle_overlay_provider: e.target.value }))} placeholder={t("例如 tailscale / easytier", "For example tailscale / easytier")} /></div>
                  <div><label>{t("服务发现模式", "Service discovery mode")}</label><select value={config.bundle_service_mode} onChange={(e) => setConfig((value) => ({ ...value, bundle_service_mode: e.target.value }))}><option value="static">static</option><option value="dns">dns</option></select></div>
                  <div><label>{t("客户端可达 host", "Client-facing host")}</label><input value={config.bundle_service_host} onChange={(e) => setConfig((value) => ({ ...value, bundle_service_host: e.target.value }))} placeholder={t("例如 roodox.example.com", "For example roodox.example.com")} /></div>
                  <div><label>{t("客户端可达端口", "Client-facing port")}</label><input type="number" value={config.bundle_service_port} onChange={(e) => setConfig((value) => ({ ...value, bundle_service_port: Number(e.target.value) || 0 }))} /></div>
                  <div><label>TLS server name</label><input value={config.bundle_tls_server_name} onChange={(e) => setConfig((value) => ({ ...value, bundle_tls_server_name: e.target.value }))} placeholder={recommendedServerName || t("证书中的 DNS 名称", "DNS name from the certificate")} /></div>
                  <div className="wide"><label>{t("Overlay Join JSON", "Overlay join JSON")}</label><textarea value={config.bundle_overlay_join_config_json} onChange={(e) => setConfig((value) => ({ ...value, bundle_overlay_join_config_json: e.target.value }))} /></div>
                </div>
                <label className="check-row"><input type="checkbox" checked={config.bundle_use_tls} onChange={(e) => setConfig((value) => ({ ...value, bundle_use_tls: e.target.checked }))} /><span>{t("接入包声明使用 TLS", "Declare TLS in the access bundle")}</span></label>
                <div className="action-row">
                  <button className="primary" onClick={() => void safeRun(async () => await persistConfig(buildConfigForSave(), t("接入设置已保存", "Access settings saved")))}>{t("保存接入设置", "Save access settings")}</button>
                  <button className="secondary" onClick={() => void safeRun(async () => { await persistConfig(buildConfigForSave(), t("接入设置已保存", "Access settings saved")); await refreshAccessPreview(); setMessage(t("Join Bundle 预览已刷新", "Join bundle preview refreshed")); })}>{t("生成接入预览", "Generate preview")}</button>
                </div>
              </article>
              <article className="subpanel">
                <div className="panel-head compact"><div><h2>{t("设备标签", "Device identity")}</h2><p className="form-note">{t("这些字段会写进这次导出的 Join Bundle，用来给客户端预置身份。", "These fields are embedded into the exported join bundle to prefill client identity.")}</p></div></div>
                <div className="form-grid two">
                  <div><label>{t("设备 ID", "Device ID")}</label><input value={joinRequest.device_id} onChange={(e) => setJoinRequest((value) => ({ ...value, device_id: e.target.value }))} /></div>
                  <div><label>{t("设备名称", "Device name")}</label><input value={joinRequest.device_name} onChange={(e) => setJoinRequest((value) => ({ ...value, device_name: e.target.value }))} /></div>
                  <div><label>{t("设备角色", "Device role")}</label><input value={joinRequest.device_role} onChange={(e) => setJoinRequest((value) => ({ ...value, device_role: e.target.value }))} /></div>
                  <div><label>{t("覆盖设备组", "Override device group")}</label><input value={joinRequest.device_group} onChange={(e) => setJoinRequest((value) => ({ ...value, device_group: e.target.value }))} placeholder={config.bundle_default_device_group || t("留空则用默认设备组", "Leave empty to use the default group")} /></div>
                </div>
              </article>
              <article className="subpanel access-preview">
                <div className="panel-head compact"><div><h2>{t("Join Bundle 预览", "Join bundle preview")}</h2><p className="form-note">{t("这是最终会发给客户端的 JSON 接入包。", "This is the JSON access bundle that will be handed to the client.")}</p></div></div>
                <pre className="json-view">{accessBundle?.bundle_json || t("保存接入设置后，点击“生成接入预览”或“导出客户端接入包”。", "Save the access settings, then generate the preview or export the client access bundle.")}</pre>
              </article>
            </div>
          </section>
        ) : null}
        {view === "logs" ? (
          <section className="panel">
            <div className="panel-head"><div><p className="eyebrow">{t("日志", "Logs")}</p><h1>{t("运行日志", "Logs")}</h1><p className="panel-subtitle">{t("查看 GUI 当前会话收集到的服务输出。", "Inspect service output captured in the current GUI session.")}</p></div><button className="secondary" onClick={() => void safeRun(refreshLogs)}>{t("刷新日志", "Refresh logs")}</button></div>
            <div className="toolbar"><input className="search-input" placeholder={t("筛选日志", "Filter logs")} value={logFilter} onChange={(e) => setLogFilter(e.target.value)} /></div>
            {filteredLogs.length === 0 ? <div className="empty-state">{t("当前没有日志", "No logs available")}</div> : <pre className="log-view">{filteredLogs.join("\n")}</pre>}
          </section>
        ) : null}
        {view === "settings" ? (
          <section className="panel">
            <div className="panel-head"><div><p className="eyebrow">{t("设置", "Settings")}</p><h1>{t("设置", "Settings")}</h1><p className="panel-subtitle">{t("环境检查与非敏感配置集中在这里维护。", "Environment checks and non-secret configuration live here.")}</p></div></div>
            <div className="settings-grid">
              <article className="subpanel">
                <h2>{t("运行设置", "Runtime")}</h2>
                <div className="form-grid two">
                  <div><label>{t("服务地址", "Service address")}</label><input value={config.addr} onChange={(e) => setConfig((v) => ({ ...v, addr: e.target.value }))} /></div>
                  <div><label>{t("数据目录", "Data root")}</label><input value={config.data_root} onChange={(e) => setConfig((v) => ({ ...v, data_root: e.target.value }))} /></div>
                  <div className="wide"><label>{t("共享根目录", "Share root")}</label><input value={config.root_dir} onChange={(e) => setConfig((v) => ({ ...v, root_dir: e.target.value }))} /></div>
                </div>
                <label className="check-row"><input type="checkbox" checked={config.remote_build_enabled} onChange={(e) => setConfig((v) => ({ ...v, remote_build_enabled: e.target.checked }))} /><span>{t("启用远程构建", "Enable remote build")}</span></label>
              </article>
              <article className="subpanel"><h2>{t("构建设置", "Build")}</h2><div className="form-grid"><div><label>{t("工具目录（每行一个）", "Tool dirs (one per line)")}</label><textarea value={toolDirsText} onChange={(e) => setToolDirsText(e.target.value)} /></div><div><label>{t("必需工具（逗号分隔）", "Required tools (comma separated)")}</label><input value={requiredToolsText} onChange={(e) => setRequiredToolsText(e.target.value)} /></div></div></article>
              <article className="subpanel"><h2>{t("传输设置", "Transport")}</h2><label className="check-row"><input type="checkbox" checked={config.tls_enabled} onChange={(e) => setConfig((v) => ({ ...v, tls_enabled: e.target.checked }))} /><span>{t("启用 TLS", "Enable TLS")}</span></label><div className="form-grid"><div><label>{t("TLS 证书路径", "TLS cert path")}</label><input value={config.tls_cert_path} onChange={(e) => setConfig((v) => ({ ...v, tls_cert_path: e.target.value }))} /></div><div><label>{t("TLS 私钥路径", "TLS key path")}</label><input value={config.tls_key_path} onChange={(e) => setConfig((v) => ({ ...v, tls_key_path: e.target.value }))} /></div></div></article>
              <article className="subpanel">
                <div className="panel-head compact"><div><h2>{t("环境检测", "Environment")}</h2></div><div className="action-row compact-actions"><button className="secondary" onClick={() => void safeRun(async () => setEnv(await invoke<EnvCheck>("check_environment")))}>{t("检测环境", "Check environment")}</button><button className="primary" onClick={() => void safeRun(async () => { await invoke("install_missing_tools"); setMessage(t("安装器已启动", "Installer started")); await refreshStatus(); })}>{t("安装缺失工具", "Install missing tools")}</button></div></div>
                {env ? <dl className="detail-grid"><div><dt>OS</dt><dd>{env.os}</dd></div><div><dt>winget</dt><dd>{env.winget_installed ? t("可用", "available") : t("缺失", "missing")}</dd></div><div><dt>cmake</dt><dd>{env.tools?.cmake?.installed ? `${t("可用", "available")} · ${env.tools.cmake.version ?? "ok"}` : `${t("缺失", "missing")} · ${env.tools?.cmake?.error ?? "n/a"}`}</dd></div><div><dt>make</dt><dd>{env.tools?.make?.installed ? `${t("可用", "available")} · ${env.tools.make.version ?? "ok"}` : `${t("缺失", "missing")} · ${env.tools?.make?.error ?? "n/a"}`}</dd></div><div className="wide"><dt>{t("推荐目录", "Recommended dirs")}</dt><dd>{(env.recommended_tool_dirs ?? []).join("; ") || none}</dd></div></dl> : <div className="empty-state">{t("暂无环境数据", "No environment data")}</div>}
              </article>
            </div>
            <div className="action-row"><button className="primary" onClick={() => void safeRun(async () => { await persistConfig(buildConfigForSave(), t("配置已保存", "Config saved")); })}>{t("保存设置", "Save settings")}</button><button className="secondary" onClick={() => void safeRun(loadConfig)}>{t("重新加载", "Reload")}</button></div>
          </section>
        ) : null}
        {view === "security" ? (
          <section className="panel">
            <div className="panel-head"><div><p className="eyebrow">{t("安全", "Security")}</p><h1>{t("安全", "Security")}</h1><p className="panel-subtitle">{t("将共享密钥与连接认证单独收口，避免和普通设置混在一起。", "Keep shared secret handling isolated from ordinary settings.")}</p></div></div>
            <article className="subpanel"><label className="check-row"><input type="checkbox" checked={config.auth_enabled} onChange={(e) => setConfig((v) => ({ ...v, auth_enabled: e.target.checked }))} /><span>{t("启用共享密钥认证", "Enable shared-secret auth")}</span></label><div className="form-grid"><div><label>{t("共享密钥", "Shared secret")}</label><input type="password" value={config.shared_secret} onChange={(e) => setConfig((v) => ({ ...v, shared_secret: e.target.value }))} /></div></div><p className="form-note">{t("客户端接入包和 Join Bundle 已转到“接入”页统一导出。", "Client access bundles and join bundles are now exported from the Access page.")}</p></article>
            <div className="action-row"><button className="primary" onClick={() => void safeRun(async () => await persistConfig(buildConfigForSave(), t("配置已保存", "Config saved")))}>{t("保存安全设置", "Save security")}</button></div>
          </section>
        ) : null}
      </main>
    </div>
  );
}
