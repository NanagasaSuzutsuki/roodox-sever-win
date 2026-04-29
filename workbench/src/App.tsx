import { type MouseEvent as ReactMouseEvent, useDeferredValue, useEffect, useMemo, useState } from "react";
import { invoke } from "@tauri-apps/api/core";

import {
  detectOverlayPreset,
  parseOverlayJoinConfig,
  setOverlayStringField,
  setOverlayStringListField
} from "./workbench/access";
import {
  defaultClientAccessExportDir,
  defaultClientCAExportPath,
  defaultConfig,
  defaultJoinRequest,
  emptyObservability,
  emptySnapshot,
  emptyTLS
} from "./workbench/defaults";
import {
  errorText,
  formatBytes,
  formatInterval,
  formatMillis,
  formatNumber,
  formatTime,
  initialLang,
  isMountedState
} from "./workbench/format";
import {
  accessOnboardingStorageKey,
  connectionCodeStorageKey,
  joinRequestStorageKey,
  langStorageKey,
  safeGetItem,
  safeRemoveItem,
  safeSetItem
} from "./workbench/persistence";
import {
  sanitizeAccessSetupResult,
  sanitizeAppConfig,
  sanitizeBackupTriggerResult,
  sanitizeConnectionCode,
  sanitizeEnvCheck,
  sanitizeExportClientAccessResult,
  sanitizeExportClientCAResult,
  sanitizeIssueJoinBundleResult,
  sanitizeServerStatus,
  sanitizeTLSStatus,
  sanitizeWorkbenchObservability,
  sanitizeWorkbenchSnapshot
} from "./workbench/sanitize";
import type {
  AccessSetupResult,
  AppConfig,
  BackupTriggerResult,
  ConnectionCodeResult,
  DeviceFilter,
  DeviceSort,
  EnvCheck,
  ExportClientAccessResult,
  ExportClientCAResult,
  IssueJoinBundleResult,
  JoinBundleRequest,
  Lang,
  ServerStatus,
  TLSStatus,
  ViewKey,
  WorkbenchObservabilitySnapshot,
  WorkbenchSnapshot
} from "./workbench/types";

type DetectedAccessChoice = {
  key: string;
  preset: "direct" | "tailscale" | "easytier";
  host: string;
  title: string;
  subtitle: string;
  recommended: boolean;
};

function isPlaceholderSecret(value: string): boolean {
  const normalized = value.trim().toLowerCase();
  return !normalized || normalized.startsWith("replace-with");
}

function isPlaceholderHost(value: string): boolean {
  const normalized = value.trim().toLowerCase();
  return !normalized || normalized.includes("example.com");
}

function sanitizeJoinRequest(value?: Partial<JoinBundleRequest> | null): JoinBundleRequest {
  return {
    device_id: typeof value?.device_id === "string" ? value.device_id : "",
    device_name: typeof value?.device_name === "string" ? value.device_name : "",
    device_role: typeof value?.device_role === "string" ? value.device_role : "",
    device_group: typeof value?.device_group === "string" ? value.device_group : ""
  };
}

function readStoredJoinRequest(): JoinBundleRequest {
  try {
    const raw = safeGetItem(joinRequestStorageKey);
    if (!raw) return defaultJoinRequest;
    return sanitizeJoinRequest(JSON.parse(raw) as Partial<JoinBundleRequest>);
  } catch {
    return defaultJoinRequest;
  }
}

function readStoredConnectionCode(): { signature: string; result: ConnectionCodeResult | null } {
  try {
    const raw = safeGetItem(connectionCodeStorageKey);
    if (!raw) {
      return { signature: "", result: null };
    }
    const parsed = JSON.parse(raw) as { signature?: string; result?: Partial<ConnectionCodeResult> | null };
    return {
      signature: typeof parsed.signature === "string" ? parsed.signature : "",
      result: sanitizeConnectionCode(parsed.result)
    };
  } catch {
    return { signature: "", result: null };
  }
}

export default function App() {
  const [lang, setLang] = useState<Lang>(initialLang);
  const [view, setView] = useState<ViewKey>("dashboard");
  const [config, setConfig] = useState<AppConfig>(defaultConfig);
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
  const [exportPath, setExportPath] = useState(defaultClientCAExportPath);
  const [accessExportDir, setAccessExportDir] = useState(defaultClientAccessExportDir);
  const [joinRequest, setJoinRequest] = useState<JoinBundleRequest>(() => readStoredJoinRequest());
  const [accessBundle, setAccessBundle] = useState<IssueJoinBundleResult | null>(null);
  const [accessSetup, setAccessSetup] = useState<AccessSetupResult | null>(null);
  const [connectionCode, setConnectionCode] = useState<ConnectionCodeResult | null>(() => readStoredConnectionCode().result);
  const [connectionCodeSignature, setConnectionCodeSignature] = useState(() => readStoredConnectionCode().signature);
  const [connectionCodeLoading, setConnectionCodeLoading] = useState(false);
  const [providerAction, setProviderAction] = useState("");
  const [initialLoadComplete, setInitialLoadComplete] = useState(false);
  const [onboardingGateChecked, setOnboardingGateChecked] = useState(false);
  const [onboardingOpen, setOnboardingOpen] = useState(false);
  const [onboardingDismissed, setOnboardingDismissed] = useState(false);
  const [onboardingAction, setOnboardingAction] = useState<"" | "save" | "code">("");
  const [onboardingError, setOnboardingError] = useState("");
  const [showAccessSecret, setShowAccessSecret] = useState(false);
  const [showAdvancedOverlay, setShowAdvancedOverlay] = useState(false);
  const [windowMaximized, setWindowMaximized] = useState(false);
  const deferredSearch = useDeferredValue(deviceSearch);
  const t = (zh: string, en: string) => (lang === "zh" ? zh : en);
  const unknown = t("未知", "Unknown");
  const none = t("无", "None");
  const never = t("从未", "Never");
  const yes = t("是", "Yes");
  const no = t("否", "No");
  const runtime = snapshot.runtime;
  const collator = useMemo(() => new Intl.Collator(lang === "zh" ? ["zh-Hans-CN-u-co-pinyin", "zh-CN", "en"] : ["en", "zh-Hans-CN-u-co-pinyin"], { sensitivity: "base", numeric: true }), [lang]);
  const currentViewLabel = useMemo(() => {
    switch (view) {
      case "dashboard":
        return t("工作台", "Workbench");
      case "devices":
        return t("设备", "Devices");
      case "operations":
        return t("运维", "Operations");
      case "access":
        return t("接入", "Client access");
      case "logs":
        return t("日志", "Logs");
      case "settings":
        return t("设置", "Settings");
      default:
        return "Roodox";
    }
  }, [view, t]);

  const statusText = useMemo(() => {
    const running = t("服务运行中", "Server running");
    const stopped = t("服务未运行", "Server stopped");
    const installing = status.installing ? ` ? ${t("环境安装中", "Installer running")}` : "";
    if (status.running) return `${running}${status.addr ? ` (${status.addr})` : ""}${installing}`;
    if (status.last_error) return `${stopped} ? ${status.last_error}${installing}`;
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
  const recommendedServerName = useMemo(() => tlsStatus.server_dns_names.find((name) => name && name !== "localhost") || tlsStatus.server_dns_names[0] || "", [tlsStatus.server_dns_names]);
  const accessPreview = accessBundle?.bundle;
  const overlayPreset = useMemo(() => detectOverlayPreset(config.bundle_overlay_provider), [config.bundle_overlay_provider]);
  const overlayConfigState = useMemo(() => parseOverlayJoinConfig(config.bundle_overlay_join_config_json), [config.bundle_overlay_join_config_json]);
  const overlayConfig = overlayConfigState.value;
  const overlayConfigError = overlayConfigState.error;
  const normalizedJoinRequest = useMemo(() => sanitizeJoinRequest(joinRequest), [joinRequest]);
  const readOverlayString = (key: string) => typeof overlayConfig[key] === "string" ? String(overlayConfig[key]) : "";
  const readOverlayList = (key: string) => Array.isArray(overlayConfig[key]) ? overlayConfig[key].filter((entry): entry is string => typeof entry === "string" && entry.trim().length > 0) : [];
  const tailscaleAuthKey = readOverlayString("authKey");
  const tailscaleTailnet = readOverlayString("tailnet");
  const tailscaleHostname = readOverlayString("hostname");
  const tailscaleControlUrl = readOverlayString("controlUrl");
  const easytierNetworkName = readOverlayString("networkName");
  const easytierNetworkSecret = readOverlayString("networkSecret");
  const easytierPeerTargetsText = readOverlayList("peerTargets").join("\n");
  const effectiveServiceHost = accessPreview?.service_host || config.bundle_service_host || unknown;
  const effectiveServicePort = accessPreview?.service_port || config.bundle_service_port;
  const effectiveTLS = accessPreview?.use_tls ?? config.bundle_use_tls;
  const effectiveServerName = accessPreview?.tls_server_name || config.bundle_tls_server_name || recommendedServerName || none;
  const effectiveDeviceGroup = accessPreview?.device_group || joinRequest.device_group.trim() || config.bundle_default_device_group || none;
  const effectiveOverlayProvider = accessPreview?.overlay_provider || config.bundle_overlay_provider || none;
  const accessHostHint = useMemo(() => {
    switch (overlayPreset) {
      case "tailscale":
        return t("这里填 Tailscale IP 或 MagicDNS 名。", "Use the Tailscale IP or MagicDNS name here.");
      case "easytier":
        return t("这里填 EasyTier overlay 地址或名称。", "Use the EasyTier overlay address or name here.");
      case "direct":
        return t("这里填客户端直接访问的域名、IP 或反向代理地址。", "Use the public, LAN, or reverse-proxy address that clients can reach directly.");
      case "custom":
        return t("这里填客户端最终要连接的地址。", "Use the final address that the client should connect to.");
      default:
        return t("先选接入方式，再填写客户端可达地址。", "Choose an access mode first, then fill the client-facing address.");
    }
  }, [overlayPreset, t]);
  const accessWarnings = useMemo(() => {
    const warnings: string[] = [];
    if (!config.bundle_overlay_provider.trim()) warnings.push(t("客户端接入的 Overlay Provider 还没配置，Join Bundle 现在发不出去。", "Overlay provider is not configured yet, so the join bundle cannot be issued."));
    if (!config.bundle_service_host.trim()) warnings.push(t("客户端接入地址 host 还是空的，需要填一个客户端真实可达的地址。", "The client-facing service host is empty. Set a host that clients can actually reach."));
    if (config.bundle_use_tls && !config.bundle_tls_server_name.trim() && !recommendedServerName) warnings.push(t("TLS 已开启，但还没有 tls_server_name，可先填证书 DNS 名称。", "TLS is enabled but tls_server_name is missing. Fill in a DNS name from the certificate."));
    if (config.auth_enabled && !config.shared_secret.trim()) warnings.push(t("共享密钥认证已开启，但 shared_secret 为空。", "Shared-secret auth is enabled but shared_secret is empty."));
    if (overlayConfigError) warnings.push(`${t("Overlay JSON 无法解析", "Overlay JSON cannot be parsed")}: ${overlayConfigError}`);
    return warnings;
  }, [config.auth_enabled, config.bundle_overlay_provider, config.bundle_service_host, config.bundle_tls_server_name, config.bundle_use_tls, config.shared_secret, overlayConfigError, recommendedServerName, t]);
  const buildToolsRequired = config.remote_build_enabled;
  const missingTools = useMemo(() => Object.entries(env?.tools ?? {}).filter(([, tool]) => !tool.installed), [env]);
  const hasMissingTools = missingTools.length > 0;
  const toolIssuesActive = buildToolsRequired && hasMissingTools;
  const installToolsDisabled = !env?.winget_installed || !hasMissingTools || !!status.installing;
  const installToolsLabel = status.installing
    ? t("安装中...", "Installing...")
    : hasMissingTools
      ? t("一键安装工具", "Install tools")
      : t("工具已就绪", "Tools ready");
  const envWarningText = useMemo(() => {
    if (!buildToolsRequired || missingTools.length === 0) return "";
    const names = missingTools.map(([name]) => name).join(", ");
    return t(`缺少构建工具：${names}。直接点“一键安装工具”即可。`, `Missing build tools: ${names}. Use "Install tools" directly.`);
  }, [buildToolsRequired, missingTools, t]);
  const providerMap = useMemo(() => new Map((accessSetup?.providers ?? []).map((provider) => [provider.id, provider])), [accessSetup]);
  const tailscaleProvider = providerMap.get("tailscale");
  const easytierProvider = providerMap.get("easytier");
  const lanCandidates = accessSetup?.lan_candidates ?? [];
  const recommendedLanHost = accessSetup?.recommended_lan_host?.trim() ?? "";
  const selectedProviderId = overlayPreset === "tailscale" || overlayPreset === "easytier" ? overlayPreset : "";
  const selectedProviderInfo = selectedProviderId ? providerMap.get(selectedProviderId) : null;
  const selectedProviderDescription = useMemo(() => {
    switch (overlayPreset) {
      case "direct":
        return t("客户端直接连接你提供的局域网、内网或公网地址。", "Clients connect directly to the LAN, private-network, or public address you provide.");
      case "tailscale":
        return t("使用 Tailscale 提供的私网地址或 MagicDNS 名称。", "Use a Tailscale private address or MagicDNS hostname.");
      case "easytier":
        return t("使用 EasyTier overlay 地址，适合自管网络。", "Use an EasyTier overlay address for a self-managed network.");
      case "custom":
        return t("保留自定义 provider 和原始 overlay JSON。", "Keep a custom provider and raw overlay JSON.");
      default:
        return t("先选择一种客户端接入方式。", "Choose a client access mode first.");
    }
  }, [overlayPreset, t]);
  const selectedProviderStatus = selectedProviderId
    ? (selectedProviderInfo?.installed
      ? `${t("已安装", "Installed")}${selectedProviderInfo.version ? ` · ${selectedProviderInfo.version}` : ""}${selectedProviderInfo.host ? ` · ${selectedProviderInfo.host}` : ""}`
      : t("未安装", "Not installed"))
    : "";
  const onboardingTLSName = config.bundle_tls_server_name.trim() || recommendedServerName.trim();
  const accessBaselineReady = useMemo(() => {
    if (!config.bundle_overlay_provider.trim()) return false;
    if (isPlaceholderHost(config.bundle_service_host)) return false;
    if (config.bundle_use_tls && !onboardingTLSName) return false;
    if (config.auth_enabled && isPlaceholderSecret(config.shared_secret)) return false;
    return true;
  }, [config.auth_enabled, config.bundle_overlay_provider, config.bundle_service_host, config.bundle_use_tls, config.shared_secret, onboardingTLSName]);
  const onboardingChecklist = useMemo(() => ([
    {
      key: "provider",
      done: !!config.bundle_overlay_provider.trim(),
      label: t("接入方式已选", "Provider selected")
    },
    {
      key: "host",
      done: !isPlaceholderHost(config.bundle_service_host),
      label: t("客户端地址已确定", "Client host ready")
    },
    {
      key: "tls",
      done: !config.bundle_use_tls || !!onboardingTLSName,
      label: t("TLS 名称已就绪", "TLS name ready")
    },
    {
      key: "secret",
      done: !config.auth_enabled || !isPlaceholderSecret(config.shared_secret),
      label: t("认证密钥已就绪", "Auth secret ready")
    }
  ]), [config.auth_enabled, config.bundle_overlay_provider, config.bundle_service_host, config.bundle_use_tls, config.shared_secret, onboardingTLSName, t]);
  const onboardingDoneCount = onboardingChecklist.filter((item) => item.done).length;
  const currentAccessModeLabel = useMemo(() => {
    switch (overlayPreset) {
      case "direct":
        return t("局域网直连", "LAN direct");
      case "tailscale":
        return "Tailscale";
      case "easytier":
        return "EasyTier";
      case "custom":
        return t("自定义", "Custom");
      default:
        return t("未配置", "Not configured");
    }
  }, [overlayPreset, t]);
  const recommendedDetectedChoiceKey = useMemo(() => {
    if (overlayPreset === "tailscale" && tailscaleProvider?.host) return `tailscale:${tailscaleProvider.host}`;
    if (overlayPreset === "easytier" && easytierProvider?.host) return `easytier:${easytierProvider.host}`;
    if (recommendedLanHost) return `direct:${recommendedLanHost}`;
    if (tailscaleProvider?.host) return `tailscale:${tailscaleProvider.host}`;
    if (easytierProvider?.host) return `easytier:${easytierProvider.host}`;
    if (lanCandidates[0]?.host) return `direct:${lanCandidates[0].host}`;
    return "";
  }, [overlayPreset, tailscaleProvider?.host, easytierProvider?.host, recommendedLanHost, lanCandidates]);
  const detectedAccessChoices = useMemo(() => {
    const seen = new Set<string>();
    const choices: DetectedAccessChoice[] = [];
    const pushChoice = (choice: Omit<DetectedAccessChoice, "recommended">) => {
      if (!choice.host.trim()) return;
      if (seen.has(choice.key)) return;
      seen.add(choice.key);
      choices.push({
        ...choice,
        recommended: choice.key === recommendedDetectedChoiceKey
      });
    };

    if (tailscaleProvider?.host) {
      pushChoice({
        key: `tailscale:${tailscaleProvider.host}`,
        preset: "tailscale",
        host: tailscaleProvider.host,
        title: tailscaleProvider.host,
        subtitle: t("检测到的 Tailscale 地址，可直接发给同 Tailnet 客户端。", "Detected Tailscale address for clients in the same tailnet.")
      });
    }
    if (easytierProvider?.host) {
      pushChoice({
        key: `easytier:${easytierProvider.host}`,
        preset: "easytier",
        host: easytierProvider.host,
        title: easytierProvider.host,
        subtitle: t("检测到的 EasyTier 地址，适合自管 overlay 网络。", "Detected EasyTier address for a self-managed overlay.")
      });
    }
    for (const candidate of lanCandidates) {
      pushChoice({
        key: `direct:${candidate.host}`,
        preset: "direct",
        host: candidate.host,
        title: candidate.host,
        subtitle: candidate.host === recommendedLanHost
          ? (candidate.interface_alias
            ? t(`推荐局域网地址 · ${candidate.interface_alias}`, `Recommended LAN address · ${candidate.interface_alias}`)
            : t("推荐局域网地址", "Recommended LAN address"))
          : (candidate.interface_alias || t("自动识别的局域网地址", "Auto-detected LAN address"))
      });
    }

    if (!choices.some((choice) => choice.recommended) && choices[0]) {
      choices[0] = { ...choices[0], recommended: true };
    }

    return choices;
  }, [t, tailscaleProvider?.host, easytierProvider?.host, lanCandidates, recommendedLanHost, recommendedDetectedChoiceKey]);
  const recommendedDetectedChoice = detectedAccessChoices.find((choice) => choice.recommended) ?? detectedAccessChoices[0] ?? null;
  const secondaryDetectedChoices = recommendedDetectedChoice
    ? detectedAccessChoices.filter((choice) => choice.key !== recommendedDetectedChoice.key)
    : [];
  const onboardingBusyText = onboardingAction === "code"
    ? t("正在保存接入设置并生成连接码，请稍候。", "Saving access settings and generating the connection code. Please wait.")
    : onboardingAction === "save"
      ? t("正在保存接入设置，请稍候。", "Saving access settings. Please wait.")
      : "";
  const setStorageWarning = (detail: string | null, zh: string, en: string) => {
    if (!detail) return;
    setMessage(`${t(zh, en)} (${detail})`);
  };

  const loadConfig = async (): Promise<AppConfig> => {
    const cfg = sanitizeAppConfig(await invoke("load_config"));
    setConfig(cfg);
    return cfg;
  };
  const buildConfigForSave = (): AppConfig => ({
    ...config,
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
    bundle_tls_server_name: config.bundle_tls_server_name.trim() || (config.bundle_use_tls ? recommendedServerName.trim() : "")
  });
  const connectionCodeInputSignature = useMemo(() => {
    const next = buildConfigForSave();
    return JSON.stringify({
      joinRequest: normalizedJoinRequest,
      access: {
        auth_enabled: next.auth_enabled,
        shared_secret: next.shared_secret,
        bundle_default_device_group: next.bundle_default_device_group,
        bundle_overlay_provider: next.bundle_overlay_provider,
        bundle_service_mode: next.bundle_service_mode,
        bundle_service_host: next.bundle_service_host,
        bundle_service_port: next.bundle_service_port,
        bundle_tls_server_name: next.bundle_tls_server_name,
        bundle_use_tls: next.bundle_use_tls,
        tls_enabled: next.tls_enabled,
        tls_cert_path: next.tls_cert_path
      }
    });
  }, [config, normalizedJoinRequest, recommendedServerName]);
  const connectionCodeDirty = !!connectionCode && connectionCodeSignature !== connectionCodeInputSignature;
  const persistConfig = async (next: AppConfig, okText: string) => {
    const saved = sanitizeAppConfig(await invoke("save_config", { cfg: next }));
    setConfig(saved);
    setMessage(okText);
  };
  const refreshStatus = async () => setStatus(sanitizeServerStatus(await invoke("server_status")));
  const refreshSnapshot = async () => setSnapshot(sanitizeWorkbenchSnapshot(await invoke("load_workbench_snapshot")));
  const refreshObservability = async () => setObservability(sanitizeWorkbenchObservability(await invoke("load_workbench_observability")));
  const refreshTLS = async () => setTLSStatus(sanitizeTLSStatus(await invoke("load_tls_status")));
  const refreshLogs = async () => {
    const next = await invoke("read_logs");
    setLogs(Array.isArray(next) ? next.filter((line): line is string => typeof line === "string") : []);
  };
  const refreshAccessPreview = async () => setAccessBundle(sanitizeIssueJoinBundleResult(await invoke("issue_join_bundle", { request: normalizedJoinRequest })));
  const refreshAccessSetup = async () => setAccessSetup(sanitizeAccessSetupResult(await invoke("discover_access_setup")));
  const refreshOps = async () => {
    await refreshSnapshot();
    await refreshObservability();
    await refreshTLS();
  };
  const saveAccessSettings = async () => {
    await persistConfig(buildConfigForSave(), t("接入设置已保存", "Access settings saved"));
  };
  const generateConnectionCodeFlow = async () => {
    setConnectionCodeLoading(true);
    try {
      setMessage(t("正在保存接入设置并生成连接码，请稍候。", "Saving access settings and generating the connection code. Please wait."));
      await saveAccessSettings();
      setMessage(t("正在生成连接码...", "Generating the connection code..."));
      const result = sanitizeConnectionCode(await invoke("generate_connection_code", { request: normalizedJoinRequest }));
      if (!result) throw new Error(t("连接码格式无效", "Invalid connection code payload"));
      presentConnectionCode(result);
      setMessage(t("连接码已生成，可直接复制给客户端。", "Connection code generated and ready to copy."));
    } finally {
      setConnectionCodeLoading(false);
    }
  };
  const exportClientAccessBundleFlow = async () => {
    await saveAccessSettings();
    const result = sanitizeExportClientAccessResult(await invoke("export_client_access_bundle", { request: normalizedJoinRequest, destinationDir: accessExportDir }));
    setAccessExportDir(result.export_dir);
    const messageParts = [`${t("客户端接入包已导出", "Client access bundle exported")}: ${result.bundle_path}`];
    if (result.connection_code_path) messageParts.push(`Code: ${result.connection_code_path}`);
    if (result.ca_path) messageParts.push(`CA: ${result.ca_path}`);
    if (result.importer_path) messageParts.push(`Importer: ${result.importer_path}`);
    setMessage(messageParts.join(" · "));
    await refreshTLS();
    setAccessBundle(sanitizeIssueJoinBundleResult(await invoke("issue_join_bundle", { request: normalizedJoinRequest })));
  };
  const openAccessHomeDialog = () => {
    setOnboardingDismissed(false);
    setOnboardingError("");
    setOnboardingOpen(true);
    setView("dashboard");
  };
  const chooseDirectory = async (initialPath: string, apply: (path: string) => void) => {
    const selected = await invoke<string | null>("pick_directory", { initialPath });
    if (selected) apply(selected);
  };
  const chooseFile = async (initialPath: string, apply: (path: string) => void) => {
    const selected = await invoke<string | null>("pick_file", { initialPath });
    if (selected) apply(selected);
  };
  const chooseSaveFilePath = async (initialPath: string, apply: (path: string) => void) => {
    const selected = await invoke<string | null>("save_file_path", { initialPath });
    if (selected) apply(selected);
  };
  const applyOverlayPreset = (preset: "direct" | "tailscale" | "easytier" | "custom") => {
    setConfig((value) => ({
      ...value,
      bundle_overlay_provider: preset === "custom"
        ? (detectOverlayPreset(value.bundle_overlay_provider) === "custom" ? value.bundle_overlay_provider : "custom")
        : preset
    }));
  };
  const updateOverlayField = (key: string, value: string) => {
    setConfig((current) => ({
      ...current,
      bundle_overlay_join_config_json: setOverlayStringField(current.bundle_overlay_join_config_json, key, value)
    }));
  };
  const updateOverlayListField = (key: string, value: string) => {
    setConfig((current) => ({
      ...current,
      bundle_overlay_join_config_json: setOverlayStringListField(current.bundle_overlay_join_config_json, key, value)
    }));
  };
  const copyText = async (value: string, okText: string) => {
    await navigator.clipboard.writeText(value);
    setMessage(okText);
  };
  const applyDetectedHost = (preset: "direct" | "tailscale" | "easytier", host: string) => {
    applyOverlayPreset(preset);
    setConfig((current) => ({
      ...current,
      bundle_overlay_provider: preset,
      bundle_service_mode: "static",
      bundle_service_host: host,
      bundle_tls_server_name: current.bundle_tls_server_name.trim() || recommendedServerName
    }));
    setMessage(t(`已套用 ${host}，保存后即可导出。`, `${host} applied. Save to export.`));
  };
  const applyProviderPreset = (preset: "tailscale" | "easytier") => {
    applyOverlayPreset(preset);
    setConfig((current) => ({
      ...current,
      bundle_overlay_provider: preset,
      bundle_service_mode: "static",
      bundle_tls_server_name: current.bundle_tls_server_name.trim() || recommendedServerName
    }));
    setMessage(t(`已切换到 ${preset} 接入。`, `${preset} access mode selected.`));
  };
  const applyAccessModeSelection = (value: "" | "direct" | "tailscale" | "easytier" | "custom") => {
    if (value === "tailscale" || value === "easytier") {
      applyProviderPreset(value);
      return;
    }
    applyOverlayPreset(value === "" ? "direct" : value);
    setConfig((current) => ({
      ...current,
      bundle_overlay_provider: value === "" ? "" : (value === "custom"
        ? (detectOverlayPreset(current.bundle_overlay_provider) === "custom" ? current.bundle_overlay_provider : "custom")
        : value),
      bundle_service_mode: "static",
      bundle_tls_server_name: current.bundle_tls_server_name.trim() || recommendedServerName
    }));
  };
  const installAccessProviderById = async (id: "tailscale" | "easytier") => {
    setProviderAction(id);
    try {
      const result = await invoke<string>("install_access_provider", { provider: id });
      setMessage(result);
      await refreshAccessSetup();
    } finally {
      setProviderAction("");
    }
  };
  const cacheConnectionCode = (result: ConnectionCodeResult) => {
    setConnectionCode(result);
    setConnectionCodeSignature(connectionCodeInputSignature);
  };
  const presentConnectionCode = (result: ConnectionCodeResult) => {
    cacheConnectionCode(result);
    setOnboardingOpen(false);
    setView("dashboard");
    requestAnimationFrame(() => {
      const codePanel = document.querySelector("#home-connection-code");
      if (codePanel instanceof HTMLElement) {
        codePanel.scrollIntoView({ block: "start", behavior: "smooth" });
        return;
      }
      const accessHub = document.querySelector("#home-access-hub");
      if (accessHub instanceof HTMLElement) {
        accessHub.scrollIntoView({ block: "start", behavior: "smooth" });
        return;
      }
      const workspace = document.querySelector(".workspace");
      if (workspace instanceof HTMLElement) {
        workspace.scrollTo({ top: 0, behavior: "smooth" });
      }
    });
  };
  const markOnboardingComplete = () => {
    const storageError = safeSetItem(accessOnboardingStorageKey, "done");
    if (storageError) {
      console.warn("persist onboarding flag failed", storageError);
    }
    setOnboardingGateChecked(true);
    setOnboardingDismissed(false);
    setOnboardingError("");
    setOnboardingOpen(false);
  };
  const reopenAccessOnboarding = () => {
    openAccessHomeDialog();
  };
  const dismissAccessOnboarding = () => {
    setOnboardingDismissed(true);
    setOnboardingError("");
    setOnboardingOpen(false);
    setMessage(t("接入弹窗已暂时关闭，可随时从首页重新打开。", "Access dialog hidden for now. Reopen it any time from the home page."));
  };
  const completeAccessOnboarding = async (mode: "save" | "code") => {
    setOnboardingAction(mode);
    setOnboardingError("");
    try {
      setMessage(mode === "code"
        ? t("正在保存接入设置并生成连接码，请稍候。", "Saving access settings and generating the connection code. Please wait.")
        : t("正在保存接入设置，请稍候。", "Saving access settings. Please wait."));
      await persistConfig(buildConfigForSave(), t("接入设置已保存", "Access settings saved"));
      await refreshAccessSetup();
      await refreshAccessPreview();
      if (mode === "code") {
        setConnectionCodeLoading(true);
        try {
          setMessage(t("正在生成连接码...", "Generating the connection code..."));
          const result = sanitizeConnectionCode(await invoke("generate_connection_code", { request: normalizedJoinRequest }));
          if (!result) throw new Error(t("连接码格式无效", "Invalid connection code payload"));
          markOnboardingComplete();
          presentConnectionCode(result);
          setMessage(t("接入向导已完成，连接码已生成。", "Access onboarding completed and the connection code is ready."));
        } finally {
          setConnectionCodeLoading(false);
        }
      } else {
        markOnboardingComplete();
        setView("dashboard");
        setMessage(t("接入向导已完成，后续可继续从首页弹窗修改。", "Access onboarding completed. You can keep editing it from the home dialog."));
      }
    } catch (error) {
      const text = errorText(error);
      setOnboardingError(text);
      setMessage(text);
    } finally {
      setOnboardingAction("");
    }
  };
  const safeRun = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
    } catch (error) {
      setMessage(errorText(error));
    }
  };
  const refreshWindowState = async () => {
    setWindowMaximized(await invoke<boolean>("window_is_maximized"));
  };
  const requestWindowDrag = async () => await invoke("window_start_drag");
  const requestWindowMinimize = async () => await invoke("window_minimize");
  const requestWindowToggleMaximize = async () => {
    await invoke("window_toggle_maximize");
    await refreshWindowState();
  };
  const requestWindowClose = async () => await invoke("window_close");
  const startWindowDrag = (event: ReactMouseEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    if (event.detail > 1) return;
    event.preventDefault();
    event.stopPropagation();
    const target = event.target as HTMLElement | null;
    if (target?.closest(".window-actions, .window-action")) return;
    void safeRun(requestWindowDrag);
  };

  useEffect(() => {
    const storageError = safeSetItem(langStorageKey, lang);
    if (storageError) {
      console.warn("persist language failed", storageError);
    }
  }, [lang]);

  useEffect(() => {
    const storageError = safeSetItem(joinRequestStorageKey, JSON.stringify(normalizedJoinRequest));
    if (storageError) {
      console.warn("persist join request failed", storageError);
    }
  }, [normalizedJoinRequest]);

  useEffect(() => {
    if (!connectionCode) {
      const storageError = safeRemoveItem(connectionCodeStorageKey);
      if (storageError) {
        console.warn("remove connection code failed", storageError);
      }
      return;
    }
    const storageError = safeSetItem(connectionCodeStorageKey, JSON.stringify({
      signature: connectionCodeSignature,
      result: connectionCode
    }));
    setStorageWarning(storageError, "连接码已生成，但本地缓存失败", "The connection code was generated, but local caching failed");
  }, [connectionCode, connectionCodeSignature]);

  useEffect(() => {
    void (async () => {
      try {
      const cfg = await loadConfig();
      const envResult = sanitizeEnvCheck(await invoke("check_environment"));
      setEnv(envResult);
      const missing = Object.entries(envResult.tools ?? {}).filter(([, tool]) => !tool.installed).map(([name]) => name);
      if (cfg.remote_build_enabled && missing.length > 0) {
        setMessage(t(`检测到缺少工具：${missing.join(", ")}。可在设置页直接一键安装。`, `Missing tools detected: ${missing.join(", ")}. Open Settings to install them.`));
      }
      await refreshAccessSetup();
      await refreshStatus();
      await refreshSnapshot();
      await refreshObservability();
      await refreshTLS();
      await refreshLogs();
      await refreshWindowState();
      } catch (error) {
        setMessage(errorText(error));
      } finally {
        setInitialLoadComplete(true);
      }
    })();
  }, []);

  useEffect(() => {
    const timer = setInterval(() => {
      void (async () => {
        try {
          await refreshStatus();
          if (view === "dashboard" || view === "devices" || view === "operations") await refreshSnapshot();
          if (view === "operations") {
            await refreshObservability();
          }
          if (view === "dashboard" || view === "operations" || view === "access") {
            await refreshTLS();
          }
          if (view === "dashboard" || view === "access") await refreshAccessSetup();
          if (view === "logs") await refreshLogs();
        } catch {
        }
      })();
    }, 4000);
    return () => clearInterval(timer);
  }, [view]);

  useEffect(() => {
    if ((view !== "access" && view !== "dashboard") || accessWarnings.length > 0 || accessBundle) return;
    void safeRun(refreshAccessPreview);
  }, [accessBundle, accessWarnings.length, normalizedJoinRequest, view]);

  useEffect(() => {
    if (view !== "access" && view !== "dashboard") return;
    void safeRun(refreshAccessSetup);
  }, [view]);

  useEffect(() => {
    if (!initialLoadComplete || onboardingGateChecked) return;
    const onboardingDone = safeGetItem(accessOnboardingStorageKey) === "done";
    if (onboardingDone) {
      setOnboardingGateChecked(true);
      return;
    }
    if (accessBaselineReady) {
      const storageError = safeSetItem(accessOnboardingStorageKey, "done");
      if (storageError) {
        console.warn("persist onboarding ready flag failed", storageError);
      }
      setOnboardingGateChecked(true);
      return;
    }
    if (!onboardingDismissed) {
      setOnboardingOpen(true);
    }
    setOnboardingGateChecked(true);
  }, [accessBaselineReady, initialLoadComplete, onboardingDismissed, onboardingGateChecked]);

  return (
    <div className="app-shell">
      <header className="window-chrome">
        <div className="window-titlebar">
          <div
            className="window-drag-surface"
            onDragStart={(event) => event.preventDefault()}
            draggable={false}
            onMouseDown={startWindowDrag}
            onDoubleClick={() => void safeRun(requestWindowToggleMaximize)}
          />
          <div className="window-titlebar-visual">
            <div className="window-badge" aria-hidden="true" draggable={false}>
              <span className="window-badge-core" />
            </div>
            <div className="window-copy" draggable={false}>
              <strong draggable={false}>{t("Roodox 工作台", "Roodox Workbench")}</strong>
              <span draggable={false}>{currentViewLabel} · {status.running ? t("服务运行中", "Server running") : t("服务未运行", "Server stopped")}</span>
            </div>
          </div>
        </div>
        <div className="window-actions">
          <button className="window-action minimize" aria-label={t("最小化", "Minimize")} onClick={() => void safeRun(requestWindowMinimize)}><span className="window-glyph minimize" /></button>
          <button className={`window-action maximize ${windowMaximized ? "active" : ""}`} aria-label={t("最大化或还原", "Toggle maximize")} onClick={() => void safeRun(requestWindowToggleMaximize)}><span className={`window-glyph ${windowMaximized ? "restore" : "maximize"}`} /></button>
          <button className="window-action close" aria-label={t("关闭", "Close")} onClick={() => void safeRun(requestWindowClose)}><span className="window-glyph close" /></button>
        </div>
      </header>
      <div className="shell">
      <aside className="sidebar">
        <div className="brand-block">
          <p className="brand-kicker">{t("Roodox 工作台", "Roodox Workbench")}</p>
          <strong>{status.running ? t("服务运行中", "Server running") : t("服务未运行", "Server stopped")}</strong>
          <span>{runtime?.health_state || unknown}</span>
        </div>
        <nav className="main-nav">
          {[["dashboard", t("工作台", "Workbench")], ["devices", t("设备", "Devices")], ["operations", t("运维", "Operations")], ["logs", t("日志", "Logs")], ["settings", t("设置", "Settings")]].map(([key, label]) => (
            <button key={key} className={`nav-link ${view === key ? "active" : ""}`} onClick={() => setView(key as ViewKey)}>{label}</button>
          ))}
        </nav>
        <div className="sidebar-foot"><span>{deviceStats.total} {t("设备", "devices")}</span><span>{deviceStats.online} {t("在线", "online")}</span></div>
      </aside>

      <main className="workspace">
        <div className="workspace-canvas">
        {message ? <div className="message-bar">{message}</div> : null}
        {onboardingOpen ? (
          <section className="onboarding-shell">
            <div className="onboarding-panel panel">
              <div className="panel-head">
                <div>
                  <p className="eyebrow">{t("首次接入", "First-time access")}</p>
                  <h1>{t("客户端接入向导", "Client access onboarding")}</h1>
                  <p className="panel-subtitle">{t("安装器只负责把程序装好。首次打开时，在这里选接入方式、自动识别地址，并生成客户端可用的连接材料。", "The installer only lays down the app. On first launch, choose an access mode here, detect a usable address, and generate client-ready connection materials.")}</p>
                </div>
                <div className="action-row compact-actions">
                  <button className="ghost" onClick={dismissAccessOnboarding}>{t("稍后处理", "Later")}</button>
                </div>
              </div>
              <div className="onboarding-grid">
                <article className="info-card">
                  <div className="info-card-head">
                    <strong>{t("1. 选择接入方式", "1. Choose access mode")}</strong>
                    <span className={`state-pill ${config.bundle_overlay_provider.trim() ? "online" : "offline"}`}>{config.bundle_overlay_provider.trim() || t("未选择", "Unset")}</span>
                  </div>
                  <div className="selector-row">
                    <div className="selector-field">
                      <label>{t("接入方式", "Access mode")}</label>
                      <select value={overlayPreset || ""} onChange={(e) => applyAccessModeSelection(e.target.value as "" | "direct" | "tailscale" | "easytier")}>
                        <option value="">{t("请选择", "Select mode")}</option>
                        <option value="direct">{t("局域网直连", "LAN direct")}</option>
                        <option value="tailscale">Tailscale</option>
                        <option value="easytier">EasyTier</option>
                      </select>
                    </div>
                    {selectedProviderId ? (
                      <div className="selector-actions">
                        {!selectedProviderInfo?.installed ? (
                          <button className="secondary" disabled={providerAction === selectedProviderId} onClick={() => void safeRun(async () => await installAccessProviderById(selectedProviderId))}>{providerAction === selectedProviderId ? t("处理中...", "Working...") : t("安装", "Install")}</button>
                        ) : selectedProviderInfo.host ? (
                          <button className="secondary" onClick={() => applyDetectedHost(selectedProviderId, selectedProviderInfo.host ?? "")}>{t("套用已识别地址", "Apply detected host")}</button>
                        ) : null}
                      </div>
                    ) : null}
                  </div>
                  <p className="form-note compact-note">{selectedProviderDescription}</p>
                  {selectedProviderId ? <div className="inline-note compact-note">{selectedProviderStatus}</div> : null}
                </article>
                <article className="info-card">
                  <div className="info-card-head">
                    <strong>{t("2. 套用可用地址", "2. Apply a usable host")}</strong>
                    <span className={`state-pill ${!isPlaceholderHost(config.bundle_service_host) ? "online" : "offline"}`}>{!isPlaceholderHost(config.bundle_service_host) ? config.bundle_service_host : t("未确定", "Unset")}</span>
                  </div>
                  {lanCandidates.length === 0 && !tailscaleProvider?.host && !easytierProvider?.host ? (
                    <div className="inline-note">{t("暂时还没有识别到可直接分发给客户端的地址。你仍然可以先选模式，再手动填写地址。", "No handoff-ready address was detected yet. You can still choose a mode first and fill the host manually.")}</div>
                  ) : (
                    <div className="stack-list">
                      {lanCandidates.map((candidate) => (
                        <div key={`${candidate.host}-${candidate.interface_alias ?? ""}-wizard`} className="candidate-card">
                          <div>
                            <strong>{candidate.host}</strong>
                            <p>{candidate.interface_alias || t("局域网自动识别", "Auto-detected LAN")}</p>
                          </div>
                          <button className="secondary" onClick={() => applyDetectedHost("direct", candidate.host)}>{t("使用这个地址", "Use this host")}</button>
                        </div>
                      ))}
                      {tailscaleProvider?.host ? (
                        <div className="candidate-card">
                          <div>
                            <strong>{tailscaleProvider.host}</strong>
                            <p>Tailscale</p>
                          </div>
                          <button className="secondary" onClick={() => applyDetectedHost("tailscale", tailscaleProvider.host ?? "")}>{t("使用这个地址", "Use this host")}</button>
                        </div>
                      ) : null}
                      {easytierProvider?.host ? (
                        <div className="candidate-card">
                          <div>
                            <strong>{easytierProvider.host}</strong>
                            <p>EasyTier</p>
                          </div>
                          <button className="secondary" onClick={() => applyDetectedHost("easytier", easytierProvider.host ?? "")}>{t("使用这个地址", "Use this host")}</button>
                        </div>
                      ) : null}
                    </div>
                  )}
                  <div className="form-grid two compact-note">
                    <div>
                      <label>{t("客户端地址", "Client host")}</label>
                      <input value={config.bundle_service_host} onChange={(e) => setConfig((value) => ({ ...value, bundle_service_host: e.target.value }))} placeholder={t("自动识别不到时手动填写", "Fill manually if detection is not enough")} />
                    </div>
                    <div>
                      <label>{t("服务端口", "Service port")}</label>
                      <input type="number" min={1} value={config.bundle_service_port} onChange={(e) => setConfig((value) => ({ ...value, bundle_service_port: Number(e.target.value) || 50051 }))} />
                    </div>
                  </div>
                </article>
                <article className="info-card">
                  <div className="info-card-head">
                    <strong>{t("3. 最小安全基线", "3. Minimum security baseline")}</strong>
                    <span className={`state-pill ${onboardingChecklist.every((item) => item.done) ? "online" : "degraded"}`}>{onboardingChecklist.filter((item) => item.done).length}/4</span>
                  </div>
                  <div className="form-grid two">
                    <label className="check-row">
                      <input type="checkbox" checked={config.bundle_use_tls} onChange={(e) => setConfig((value) => ({ ...value, bundle_use_tls: e.target.checked }))} />
                      <span>{t("客户端连接使用 TLS", "Clients use TLS")}</span>
                    </label>
                    <label className="check-row">
                      <input type="checkbox" checked={config.auth_enabled} onChange={(e) => setConfig((value) => ({ ...value, auth_enabled: e.target.checked }))} />
                      <span>{t("启用共享密钥认证", "Enable shared-secret auth")}</span>
                    </label>
                    <div>
                      <label>{t("TLS server name", "TLS server name")}</label>
                      <input value={config.bundle_tls_server_name} onChange={(e) => setConfig((value) => ({ ...value, bundle_tls_server_name: e.target.value }))} placeholder={recommendedServerName || t("证书里的 DNS 名称", "DNS name from the certificate")} />
                    </div>
                    <div>
                      <label>{t("默认设备组", "Default device group")}</label>
                      <input value={config.bundle_default_device_group} onChange={(e) => setConfig((value) => ({ ...value, bundle_default_device_group: e.target.value }))} />
                    </div>
                    <div className="wide">
                      <label>{t("共享密钥", "Shared secret")}</label>
                      <input type={showAccessSecret ? "text" : "password"} value={config.shared_secret} onChange={(e) => setConfig((value) => ({ ...value, shared_secret: e.target.value }))} placeholder={t("认证开启时必须填写", "Required when auth is enabled")} />
                    </div>
                  </div>
                  <div className="action-row compact-actions">
                    <button className="secondary" onClick={() => setShowAccessSecret((value) => !value)}>{showAccessSecret ? t("隐藏密钥", "Hide secret") : t("显示密钥", "Show secret")}</button>
                    {recommendedServerName ? <button className="secondary" onClick={() => setConfig((value) => ({ ...value, bundle_tls_server_name: recommendedServerName }))}>{t("使用证书 DNS 名称", "Use cert DNS name")}</button> : null}
                  </div>
                </article>
                <article className="info-card onboarding-summary">
                  <div className="info-card-head">
                    <strong>{t("4. 完成并交付", "4. Finish and hand off")}</strong>
                    <span className={`state-pill ${accessBaselineReady ? "online" : "degraded"}`}>{accessBaselineReady ? t("可完成", "Ready") : t("待补全", "Pending")}</span>
                  </div>
                  <div className="checklist">
                    {onboardingChecklist.map((item) => <div key={item.key} className={`checklist-item ${item.done ? "done" : ""}`}>{item.label}</div>)}
                  </div>
                  <p className="form-note">{t("完成后仍可继续从首页弹窗修改。这个向导只负责把第一次交付链路收口。", "You can keep editing everything later from the home dialog. This wizard only closes the first handoff loop.")}</p>
                  <div className="action-row">
                    <button className="primary" disabled={!accessBaselineReady || !!onboardingAction || connectionCodeLoading} onClick={() => void safeRun(async () => await completeAccessOnboarding("code"))}>{onboardingAction === "code" || connectionCodeLoading ? t("生成中...", "Generating...") : t("保存并生成连接码", "Save and generate code")}</button>
                    <button className="secondary" disabled={!accessBaselineReady || !!onboardingAction} onClick={() => void safeRun(async () => await completeAccessOnboarding("save"))}>{onboardingAction === "save" ? t("保存中...", "Saving...") : t("仅保存接入方式", "Save access only")}</button>
                  </div>
                  {onboardingError ? <div className="inline-warning compact-note">{onboardingError}</div> : null}
                  {onboardingBusyText ? <div className="inline-note compact-note">{onboardingBusyText}</div> : null}
                  {!accessBaselineReady ? <div className="inline-note compact-note">{t("至少要选好接入方式和客户端地址；若启用了 TLS 或共享密钥，也要把对应字段补齐。", "At minimum, choose an access mode and client host. If TLS or shared-secret auth is enabled, fill those fields too.")}</div> : null}
                </article>
              </div>
            </div>
          </section>
        ) : null}
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
              {toolIssuesActive && envWarningText ? <div className="inline-warning">{envWarningText}</div> : null}
              <div className="action-row">
                <button className="primary" onClick={() => void safeRun(async () => { await invoke("start_server"); await refreshStatus(); await refreshSnapshot(); setMessage(t("服务已启动", "Server started")); })}>{t("启动服务", "Start server")}</button>
                <button className="ghost" onClick={() => void safeRun(async () => { await invoke("stop_server"); await refreshStatus(); await refreshSnapshot(); setMessage(t("已请求停止服务", "Stop requested")); })}>{t("停止服务", "Stop server")}</button>
                <button className="secondary" onClick={() => void safeRun(async () => { await refreshStatus(); await refreshSnapshot(); })}>{t("刷新", "Refresh")}</button>
                <button className="secondary" onClick={() => setView("operations")}>{t("打开运维页", "Open operations")}</button>
                {toolIssuesActive ? <button className="secondary" onClick={() => setView("settings")}>{t("处理缺失工具", "Fix missing tools")}</button> : null}
              </div>
              <section className="quick-security">
                <div className="panel-head compact">
                  <div>
                    <p className="eyebrow">{t("安全", "Security")}</p>
                    <h2>{t("连接密钥", "Connection secret")}</h2>
                    <p className="form-note">{t("客户端接入包会使用这里的共享密钥。安全设置不再单独放一页。", "The client access bundle uses this shared secret. Security settings now live here on the home page.")}</p>
                  </div>
                </div>
                <div className="quick-security-grid">
                  <label className="check-row">
                    <input type="checkbox" checked={config.auth_enabled} onChange={(e) => setConfig((value) => ({ ...value, auth_enabled: e.target.checked }))} />
                    <span>{t("启用共享密钥认证", "Enable shared-secret auth")}</span>
                  </label>
                  <div>
                    <label>{t("共享密钥", "Shared secret")}</label>
                    <input type={showAccessSecret ? "text" : "password"} value={config.shared_secret} onChange={(e) => setConfig((value) => ({ ...value, shared_secret: e.target.value }))} />
                  </div>
                  <div className="action-row compact-actions">
                    <button className="secondary" onClick={() => setShowAccessSecret((value) => !value)}>{showAccessSecret ? t("隐藏密钥", "Hide secret") : t("显示密钥", "Show secret")}</button>
                    <button className="primary" onClick={() => void safeRun(async () => await persistConfig(buildConfigForSave(), t("安全设置已保存", "Security settings saved")))}>{t("保存密钥", "Save secret")}</button>
                  </div>
                </div>
              </section>
            </section>
            <section id="home-access-hub" className="quick-access-hub">
              <div className="panel-head compact">
                <div>
                  <p className="eyebrow">{t("客户端接入", "Client access")}</p>
                  <h2>{t("首页接入中枢", "Home access hub")}</h2>
                  <p className="panel-subtitle">{t("接入主流程现在固定在首页：先识别可用地址，再保存并生成连接码，最后导出给客户端。详细字段放进首页弹窗，不再要求用户跳副页。", "The primary access flow now stays on the home page: detect a usable route, save and generate the connection code, then export the client bundle. Detailed fields stay in a home-page dialog instead of a separate subpage.")}</p>
                </div>
                <div className="action-row compact-actions">
                  <button className="secondary" onClick={() => void safeRun(async () => { await refreshTLS(); await refreshAccessSetup(); if (accessWarnings.length === 0) await refreshAccessPreview(); })}>{t("重新识别", "Re-detect")}</button>
                  <button className="secondary" onClick={openAccessHomeDialog}>{t("配置接入", "Configure access")}</button>
                  <button className="primary" disabled={!accessBaselineReady || connectionCodeLoading} onClick={() => void safeRun(generateConnectionCodeFlow)}>{connectionCodeLoading ? t("生成中...", "Generating...") : t("保存并生成连接码", "Save and generate code")}</button>
                </div>
              </div>
              {!connectionCode && accessBaselineReady ? <div className="inline-note">{t("当前还没有保存过连接码，请先生成一次。生成后的结果会保留在本机。", "No saved connection code is available yet. Generate one once and it will stay on this machine.")}</div> : null}
              {connectionCode ? (
                <article id="home-connection-code" className="subpanel connection-result-panel access-home-code">
                  <div className="panel-head compact">
                    <div>
                      <h2>{t("当前连接码", "Current connection code")}</h2>
                      <p className="form-note">{t("生成后的连接码会固定显示在首页接入中枢顶部，成功后会自动滚到这里。", "The generated connection code stays at the top of the home access hub and the page scrolls here automatically after success.")}</p>
                    </div>
                    <span className="state-pill online">{connectionCode.format}</span>
                  </div>
                  <textarea className="code-box mono-wrap" readOnly value={connectionCode.code} />
                  <div className="action-row">
                    <button className="secondary" onClick={() => void safeRun(async () => await copyText(connectionCode.code, t("连接码已复制", "Connection code copied")))}>{t("复制连接码", "Copy code")}</button>
                    <button className="secondary" onClick={() => void safeRun(async () => await copyText(connectionCode.uri, t("连接 URI 已复制", "Connection URI copied")))}>{t("复制连接 URI", "Copy URI")}</button>
                    <span className="inline-note small-note">{t(`原始载荷 ${connectionCode.payload_size} 字节`, `${connectionCode.payload_size} bytes payload`)}</span>
                  </div>
                  {connectionCodeDirty ? <div className="inline-warning compact-note">{t("接入参数或设备标签已变更，当前连接码可能已过期，请重新生成。", "Access settings or device labels changed. Regenerate the connection code before handing it off.")}</div> : null}
                </article>
              ) : null}
              {accessWarnings.length > 0 ? <div className="stack-list">{accessWarnings.map((warning) => <div key={`dashboard-${warning}`} className="inline-warning">{warning}</div>)}</div> : null}
              <div className="quick-access-grid">
                <article className="info-card access-overview-card">
                  <div className="info-card-head">
                    <strong>{t("当前交付配置", "Current handoff setup")}</strong>
                    <span className={`state-pill ${accessBaselineReady ? "online" : "degraded"}`}>{accessBaselineReady ? t("可交付", "Ready") : t("待补全", "Needs setup")}</span>
                  </div>
                  <dl className="detail-grid">
                    <div><dt>{t("接入方式", "Access mode")}</dt><dd>{currentAccessModeLabel}</dd></div>
                    <div><dt>{t("客户端地址", "Client host")}</dt><dd>{effectiveServiceHost}</dd></div>
                    <div><dt>{t("客户端端口", "Client port")}</dt><dd>{effectiveServicePort ? formatNumber(effectiveServicePort, lang, none) : none}</dd></div>
                    <div><dt>{t("TLS server name", "TLS server name")}</dt><dd>{effectiveServerName}</dd></div>
                    <div><dt>TLS</dt><dd>{effectiveTLS ? yes : no}</dd></div>
                    <div><dt>{t("认证", "Auth")}</dt><dd>{config.auth_enabled ? yes : no}</dd></div>
                    <div><dt>{t("默认设备组", "Default device group")}</dt><dd>{effectiveDeviceGroup}</dd></div>
                    <div><dt>{t("连接基线", "Baseline")}</dt><dd>{onboardingDoneCount}/4</dd></div>
                  </dl>
                  <div className="checklist compact-checklist">
                    {onboardingChecklist.map((item) => <div key={`home-${item.key}`} className={`checklist-item ${item.done ? "done" : ""}`}>{item.label}</div>)}
                  </div>
                  <div className="action-row">
                    <button className="primary" onClick={openAccessHomeDialog}>{t("打开接入弹窗", "Open access dialog")}</button>
                    <button className="secondary" disabled={!accessBaselineReady} onClick={() => void safeRun(exportClientAccessBundleFlow)}>{t("导出客户端接入包", "Export client bundle")}</button>
                    <button className="secondary" onClick={() => void safeRun(saveAccessSettings)}>{t("仅保存设置", "Save only")}</button>
                  </div>
                  {!connectionCode && accessBaselineReady ? <div className="inline-note compact-note">{t("当前配置已经可交付，但还没有保存过连接码。先生成一次，之后会保留在本机。", "This setup is ready, but no saved connection code exists yet. Generate it once and it will stay on this machine.")}</div> : null}
                  {connectionCodeDirty ? <div className="inline-warning compact-note">{t("你刚修改过接入参数，首页显示的连接码需要重新生成。", "You changed the access settings. Regenerate the home-page connection code.")}</div> : null}
                </article>
                <article className="info-card access-detection-card">
                  <div className="info-card-head">
                    <strong>{t("智能识别结果", "Detection results")}</strong>
                    <span className={`state-pill ${detectedAccessChoices.length > 0 ? "online" : "offline"}`}>{detectedAccessChoices.length > 0 ? t("已识别", "Detected") : t("未识别", "Not found")}</span>
                  </div>
                  {recommendedDetectedChoice ? (
                    <div className="access-choice-list">
                      <div className="candidate-card access-choice-card recommended">
                        <div>
                          <strong>{recommendedDetectedChoice.title}</strong>
                          <p>{recommendedDetectedChoice.subtitle}</p>
                        </div>
                        <div className="selector-actions">
                          <span className="state-pill online">{t("推荐", "Recommended")}</span>
                          <button className="primary" onClick={() => applyDetectedHost(recommendedDetectedChoice.preset, recommendedDetectedChoice.host)}>{t("使用这个地址", "Use this address")}</button>
                        </div>
                      </div>
                      {secondaryDetectedChoices.length > 0 ? (
                        <div className="stack-list access-choice-secondary">
                          {secondaryDetectedChoices.map((choice) => (
                            <div key={choice.key} className="candidate-card access-choice-card">
                              <div>
                                <strong>{choice.title}</strong>
                                <p>{choice.subtitle}</p>
                              </div>
                              <button className="secondary" onClick={() => applyDetectedHost(choice.preset, choice.host)}>{t("改用这个", "Use instead")}</button>
                            </div>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  ) : (
                    <div className="inline-note">{t("当前还没有识别到可以直接发给客户端的地址。你可以先打开接入弹窗选模式，或安装 Tailscale / EasyTier 后再重新识别。", "No client-ready address is detected yet. Open the access dialog first, or install Tailscale / EasyTier and re-detect.")}</div>
                  )}
                  <div className="stack-list access-provider-strip">
                    <div className="candidate-card provider-card">
                      <div>
                        <strong>Tailscale</strong>
                        <p>{tailscaleProvider?.installed ? (tailscaleProvider.host ? t(`已识别 ${tailscaleProvider.host}`, `Detected ${tailscaleProvider.host}`) : t("已安装，但还没识别到可交付地址。", "Installed, but no client-ready address is detected yet.")) : t("未安装。适合需要私网穿透、又不想自己维护服务器的场景。", "Not installed. Good when you want private networking without self-hosting control servers.")}</p>
                      </div>
                      {!tailscaleProvider?.installed ? (
                        <button className="secondary" disabled={providerAction === "tailscale"} onClick={() => void safeRun(async () => await installAccessProviderById("tailscale"))}>{providerAction === "tailscale" ? t("处理中...", "Working...") : t("去安装", "Install")}</button>
                      ) : tailscaleProvider.host ? (
                        <button className="secondary" onClick={() => applyDetectedHost("tailscale", tailscaleProvider.host ?? "")}>{t("套用地址", "Apply host")}</button>
                      ) : null}
                    </div>
                    <div className="candidate-card provider-card">
                      <div>
                        <strong>EasyTier</strong>
                        <p>{easytierProvider?.installed
                          ? (easytierProvider.host
                            ? t(`已识别 ${easytierProvider.host}`, `Detected ${easytierProvider.host}`)
                            : t("已安装，但还没识别到 overlay 地址。", "Installed, but no overlay address is detected yet."))
                          : t("未安装。适合自管 overlay 网络，局域网和公网混合部署也更灵活。", "Not installed. Good for self-managed overlays and mixed LAN/public deployments.")}</p>
                      </div>
                      {!easytierProvider?.installed ? (
                        <button className="secondary" disabled={providerAction === "easytier"} onClick={() => void safeRun(async () => await installAccessProviderById("easytier"))}>{providerAction === "easytier" ? t("处理中...", "Working...") : t("去安装", "Install")}</button>
                      ) : easytierProvider.host ? (
                        <button className="secondary" onClick={() => applyDetectedHost("easytier", easytierProvider.host ?? "")}>{t("套用地址", "Apply host")}</button>
                      ) : null}
                    </div>
                  </div>
                </article>
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
                  <div><dt>{t("服务 ID", "Server ID")}</dt><dd>{runtime?.server_id || unknown}</dd></div>
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
              <div><p className="eyebrow">{t("运维", "Operations")}</p><h1>{t("运维", "Operations")}</h1><p className="panel-subtitle">{t("这里只保留状态、备份、TLS 和导出入口，不再暴露过多底层文件细节。", "This page now focuses on status, backup, TLS, and export actions without exposing low-level file details.")}</p></div>
              <div className="action-row compact-actions">
                <button className="secondary" onClick={() => void safeRun(refreshOps)}>{t("刷新运维状态", "Refresh operations")}</button>
                <button className="primary" onClick={() => void safeRun(async () => { const result = sanitizeBackupTriggerResult(await invoke("trigger_server_backup")); await refreshOps(); setMessage(`${t("手动备份已触发", "Manual backup triggered")}: ${formatTime(result.created_at_unix, lang, never)}`); })}>{t("立即备份", "Trigger backup")}</button>
              </div>
            </div>
            <div className="ops-grid">
              <article className="subpanel">
                <div className="panel-head compact"><div><h2>{t("运行与备份", "Runtime and backup")}</h2><p className="form-note">{t("给管理员看结果，不给普通用户看数据库与 WAL 路径。", "Show operator-facing results without exposing database and WAL paths.")}</p></div></div>
                {snapshot.query_error ? <div className="inline-warning">{t("当前无法读取运行态", "Unable to read the runtime snapshot")} · {snapshot.query_error}</div> : null}
                <dl className="detail-grid">
                  <div><dt>{t("服务健康", "Service health")}</dt><dd>{runtime?.health_state || unknown}</dd></div>
                  <div><dt>{t("健康说明", "Health message")}</dt><dd>{runtime?.health_message || none}</dd></div>
                  <div><dt>{t("最近备份", "Last backup")}</dt><dd>{formatTime(runtime?.backup.last_backup_at_unix, lang, never)}</dd></div>
                  <div><dt>{t("备份间隔", "Backup interval")}</dt><dd>{formatInterval(runtime?.backup.interval_seconds, lang, none)}</dd></div>
                  <div><dt>{t("保留份数", "Keep latest")}</dt><dd>{formatNumber(runtime?.backup.keep_latest, lang, none)}</dd></div>
                  <div><dt>{t("Checkpoint 模式", "Checkpoint mode")}</dt><dd>{runtime?.checkpoint.mode || none}</dd></div>
                  <div className="wide"><dt>{t("最近错误", "Last error")}</dt><dd>{runtime?.checkpoint.last_error || runtime?.backup.last_error || none}</dd></div>
                </dl>
              </article>
              <article className="subpanel">
                <div className="panel-head compact"><div><h2>TLS</h2><p className="form-note">{config.tls_enabled ? t("当前配置启用了 TLS。", "TLS is enabled in config.") : t("当前配置未启用 TLS，但仍可检查证书文件状态。", "TLS is disabled in config, but certificate files can still be inspected.")}</p></div></div>
                <div className="toolbar">
                  <div className="path-field">
                    <input value={exportPath} onChange={(e) => setExportPath(e.target.value)} placeholder={t("导出路径", "Export path")} />
                    <button className="secondary" onClick={() => void safeRun(async () => await chooseSaveFilePath(exportPath, setExportPath))}>{t("浏览", "Browse")}</button>
                  </div>
                  <button className="primary" onClick={() => void safeRun(async () => { const result = sanitizeExportClientCAResult(await invoke("export_client_ca", { destinationPath: exportPath })); setExportPath(result.exported_path); await refreshTLS(); setMessage(`${t("客户端 CA 已导出", "Client CA exported")}: ${result.exported_path}`); })}>{t("导出客户端 CA", "Export client CA")}</button>
                </div>
                <div className="pill-strip">
                  <span className={`state-pill ${tlsStatus.overall_valid ? "online" : "offline"}`}>{t("整体有效", "Overall valid")}: {tlsStatus.overall_valid ? yes : no}</span>
                  <span className={`state-pill ${tlsStatus.server_valid ? "online" : "degraded"}`}>{t("服务端证书有效", "Server cert valid")}: {tlsStatus.server_valid ? yes : no}</span>
                  <span className={`state-pill ${tlsStatus.root_valid ? "online" : "degraded"}`}>{t("根证书有效", "Root cert valid")}: {tlsStatus.root_valid ? yes : no}</span>
                  <span className={`state-pill ${tlsStatus.root_is_ca ? "online" : "offline"}`}>{t("根证书为 CA", "Root cert is CA")}: {tlsStatus.root_is_ca ? yes : no}</span>
                </div>
                <dl className="detail-grid">
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
          <section className="panel access-page">
            <div className="panel-head">
              <div><p className="eyebrow">{t("高级接入", "Advanced access")}</p><h1>{t("高级接入", "Advanced access")}</h1><p className="panel-subtitle">{t("这里保留高级字段和原始预览，但主流程已经迁回首页接入中枢。", "This area keeps advanced fields and raw previews, while the primary flow has moved back to the home access hub.")}</p></div>
              <div className="action-row compact-actions">
                <button className="secondary" onClick={() => setView("dashboard")}>{t("回到首页", "Back to home")}</button>
                <button className="secondary" onClick={reopenAccessOnboarding}>{t("打开首页弹窗", "Open home dialog")}</button>
                <button className="secondary" onClick={() => void safeRun(async () => { await refreshTLS(); await refreshAccessSetup(); if (accessWarnings.length === 0) await refreshAccessPreview(); })}>{t("刷新接入信息", "Refresh access")}</button>
                <button className="primary" onClick={() => void safeRun(exportClientAccessBundleFlow)}>{t("导出客户端接入包", "Export access bundle")}</button>
              </div>
            </div>
            {accessWarnings.length > 0 ? <div className="stack-list">{accessWarnings.map((warning) => <div key={warning} className="inline-warning">{warning}</div>)}</div> : null}
            {connectionCode ? (
              <article className="subpanel connection-result-panel">
                <div className="panel-head compact">
                  <div>
                    <h2>{t("当前连接码", "Current connection code")}</h2>
                    <p className="form-note">{t("刚生成的连接码会固定显示在这里，避免生成后还要往下翻。", "The latest connection code stays pinned here so it remains visible after generation.")}</p>
                  </div>
                  <span className="state-pill online">{connectionCode.format}</span>
                </div>
                <textarea className="code-box mono-wrap" readOnly value={connectionCode.code} />
                <div className="action-row">
                  <button className="secondary" onClick={() => void safeRun(async () => await copyText(connectionCode.code, t("连接码已复制", "Connection code copied")))}>{t("复制连接码", "Copy code")}</button>
                  <button className="secondary" onClick={() => void safeRun(async () => await copyText(connectionCode.uri, t("连接 URI 已复制", "Connection URI copied")))}>{t("复制连接 URI", "Copy URI")}</button>
                  <span className="inline-note small-note">{t(`原始载荷 ${connectionCode.payload_size} 字节`, `${connectionCode.payload_size} bytes payload`)}</span>
                </div>
                {connectionCodeDirty ? <div className="inline-warning compact-note">{t("当前连接码不是最新参数生成的，请重新生成后再交付。", "This connection code was not generated from the latest settings. Regenerate it before handoff.")}</div> : null}
              </article>
            ) : null}
            <div className="access-grid">
              <article className="subpanel access-setup">
                <div className="panel-head compact">
                  <div>
                    <h2>{t("智能接入", "Smart onboarding")}</h2>
                    <p className="form-note">{t("先选可接受的网络服务商，再让工作台自动识别可用地址并生成连接码。当前连接码是自包含长码，不依赖中心兑换服务。", "Choose a network provider first, then let the workbench detect usable addresses and generate a self-contained connection code.")}</p>
                  </div>
                  <div className="action-row compact-actions">
                    <button className="secondary" onClick={() => void safeRun(refreshAccessSetup)}>{t("重新识别", "Re-detect")}</button>
                    <button className="primary" disabled={connectionCodeLoading} onClick={() => void safeRun(generateConnectionCodeFlow)}>{connectionCodeLoading ? t("生成中...", "Generating...") : t("生成连接码", "Generate connection code")}</button>
                  </div>
                </div>
                <div className="setup-grid">
                  <article className="info-card">
                    <div className="info-card-head">
                      <strong>{t("接入方式", "Access mode")}</strong>
                      <span className={`state-pill ${config.bundle_overlay_provider.trim() ? "online" : "offline"}`}>{config.bundle_overlay_provider.trim() || t("未选择", "Unset")}</span>
                    </div>
                    <div className="selector-row">
                      <div className="selector-field">
                        <label>{t("接入方式", "Access mode")}</label>
                        <select value={overlayPreset || ""} onChange={(e) => applyAccessModeSelection(e.target.value as "" | "direct" | "tailscale" | "easytier" | "custom")}>
                          <option value="">{t("请选择", "Select mode")}</option>
                          <option value="direct">{t("局域网直连", "LAN direct")}</option>
                          <option value="tailscale">Tailscale</option>
                          <option value="easytier">EasyTier</option>
                          <option value="custom">{t("自定义", "Custom")}</option>
                        </select>
                      </div>
                      {selectedProviderId ? (
                        <div className="selector-actions">
                          {!selectedProviderInfo?.installed ? (
                            <button className="secondary" disabled={providerAction === selectedProviderId} onClick={() => void safeRun(async () => await installAccessProviderById(selectedProviderId))}>{providerAction === selectedProviderId ? t("处理中...", "Working...") : t("安装", "Install")}</button>
                          ) : selectedProviderInfo.host ? (
                            <button className="secondary" onClick={() => applyDetectedHost(selectedProviderId, selectedProviderInfo.host ?? "")}>{t("套用已识别地址", "Apply detected host")}</button>
                          ) : null}
                        </div>
                      ) : null}
                    </div>
                    <p className="form-note compact-note">{selectedProviderDescription}</p>
                    {selectedProviderId ? <div className="inline-note compact-note">{selectedProviderStatus}</div> : null}
                  </article>
                  <article className="info-card">
                    <div className="info-card-head">
                      <strong>{t("局域网自动识别", "Auto-detected LAN")}</strong>
                      <span className={`state-pill ${lanCandidates.length > 0 ? "online" : "offline"}`}>{lanCandidates.length > 0 ? t("已识别", "Detected") : t("未识别", "Not found")}</span>
                    </div>
                    {lanCandidates.length === 0 ? <div className="inline-note">{t("当前没有识别到适合直接分发给客户端的局域网地址。", "No LAN address suitable for client handoff was detected yet.")}</div> : (
                      <div className="stack-list">
                        {lanCandidates.map((candidate) => (
                          <div key={`${candidate.host}-${candidate.interface_alias ?? ""}`} className="candidate-card">
                            <div>
                              <strong>{candidate.host}</strong>
                              <p>{candidate.interface_alias || t("自动识别", "Auto detected")}</p>
                            </div>
                            <button className="secondary" onClick={() => applyDetectedHost("direct", candidate.host)}>{t("使用这个地址", "Use this address")}</button>
                          </div>
                        ))}
                      </div>
                    )}
                  </article>
                </div>
              </article>
              <article className="subpanel access-baseline-card">
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
                  <div className="path-field">
                    <input value={accessExportDir} onChange={(e) => setAccessExportDir(e.target.value)} placeholder={t("导出目录", "Export directory")} />
                    <button className="secondary" onClick={() => void safeRun(async () => await chooseDirectory(accessExportDir, setAccessExportDir))}>{t("浏览", "Browse")}</button>
                  </div>
                </div>
              </article>
              <article className="subpanel access-settings-card">
                <div className="panel-head compact"><div><h2>{t("接入设置", "Access settings")}</h2><p className="form-note">{t("这里维护客户端真正要连的外部地址，不是本机 GUI 的管理地址。", "Maintain the client-facing address here, not the local GUI admin address.")}</p></div></div>
                <div className="form-grid two">
                  <div className="wide">
                    <label>{t("接入方式", "Access mode")}</label>
                    <div className="selector-row">
                      <div className="selector-field">
                        <select value={overlayPreset || ""} onChange={(e) => applyAccessModeSelection(e.target.value as "" | "direct" | "tailscale" | "easytier" | "custom")}>
                          <option value="">{t("请选择", "Select mode")}</option>
                          <option value="direct">{t("直接地址", "Direct")}</option>
                          <option value="tailscale">Tailscale</option>
                          <option value="easytier">EasyTier</option>
                          <option value="custom">{t("自定义", "Custom")}</option>
                        </select>
                      </div>
                      {selectedProviderId ? (
                        <div className="selector-actions">
                          {!selectedProviderInfo?.installed ? (
                            <button className="secondary" disabled={providerAction === selectedProviderId} onClick={() => void safeRun(async () => await installAccessProviderById(selectedProviderId))}>{providerAction === selectedProviderId ? t("处理中...", "Working...") : t("安装", "Install")}</button>
                          ) : selectedProviderInfo.host ? (
                            <button className="secondary" onClick={() => applyDetectedHost(selectedProviderId, selectedProviderInfo.host ?? "")}>{t("套用已识别地址", "Apply detected host")}</button>
                          ) : null}
                        </div>
                      ) : null}
                    </div>
                    <p className="form-note compact-note">{selectedProviderDescription}</p>
                    {selectedProviderId ? <div className="inline-note compact-note">{selectedProviderStatus}</div> : null}
                  </div>
                  <div><label>{t("默认设备组", "Default device group")}</label><input value={config.bundle_default_device_group} onChange={(e) => setConfig((value) => ({ ...value, bundle_default_device_group: e.target.value }))} /></div>
                  <div>
                    <label>{t("Overlay Provider", "Overlay provider")}</label>
                    {overlayPreset === "custom" ? (
                      <input value={config.bundle_overlay_provider} onChange={(e) => setConfig((value) => ({ ...value, bundle_overlay_provider: e.target.value }))} placeholder={t("例如 my-overlay", "For example my-overlay")} />
                    ) : (
                      <input value={config.bundle_overlay_provider} readOnly />
                    )}
                  </div>
                  <div><label>{t("服务发现模式", "Service discovery mode")}</label><select value={config.bundle_service_mode} onChange={(e) => setConfig((value) => ({ ...value, bundle_service_mode: e.target.value }))}><option value="static">static</option><option value="dns">dns</option></select></div>
                  <div>
                    <label>{overlayPreset === "tailscale" ? "Tailscale Host" : overlayPreset === "easytier" ? "EasyTier Host" : t("客户端可达 host", "Client-facing host")}</label>
                    <input value={config.bundle_service_host} onChange={(e) => setConfig((value) => ({ ...value, bundle_service_host: e.target.value }))} placeholder={overlayPreset === "tailscale" ? t("例如 100.x.x.x 或 host.tailnet.ts.net", "For example 100.x.x.x or host.tailnet.ts.net") : overlayPreset === "easytier" ? t("例如 10.x.x.x 或 overlay 名称", "For example 10.x.x.x or an overlay hostname") : t("例如 roodox.example.com", "For example roodox.example.com")} />
                    <p className="form-note compact-note">{accessHostHint}</p>
                  </div>
                  <div><label>{t("客户端可达端口", "Client-facing port")}</label><input type="number" value={config.bundle_service_port} onChange={(e) => setConfig((value) => ({ ...value, bundle_service_port: Number(e.target.value) || 0 }))} /></div>
                  <div><label>TLS server name</label><input value={config.bundle_tls_server_name} onChange={(e) => setConfig((value) => ({ ...value, bundle_tls_server_name: e.target.value }))} placeholder={recommendedServerName || t("证书中的 DNS 名称", "DNS name from the certificate")} /></div>
                  {overlayPreset === "tailscale" ? (
                    <>
                      <div><label>{t("Tailscale Auth Key", "Tailscale Auth Key")}</label><input value={tailscaleAuthKey} onChange={(e) => updateOverlayField("authKey", e.target.value)} placeholder="tskey-..." /></div>
                      <div><label>{t("Tailnet", "Tailnet")}</label><input value={tailscaleTailnet} onChange={(e) => updateOverlayField("tailnet", e.target.value)} placeholder={t("例如 example.ts.net", "For example example.ts.net")} /></div>
                      <div><label>{t("客户端主机名", "Client hostname")}</label><input value={tailscaleHostname} onChange={(e) => updateOverlayField("hostname", e.target.value)} placeholder={t("可选，发给客户端 bootstrap", "Optional, passed to the client bootstrap")} /></div>
                      <div><label>{t("控制地址", "Control URL")}</label><input value={tailscaleControlUrl} onChange={(e) => updateOverlayField("controlUrl", e.target.value)} placeholder={t("留空表示官方控制面", "Leave empty for the default control plane")} /></div>
                    </>
                  ) : null}
                  {overlayPreset === "easytier" ? (
                    <>
                      <div><label>{t("网络名", "Network name")}</label><input value={easytierNetworkName} onChange={(e) => updateOverlayField("networkName", e.target.value)} placeholder={t("例如 roodox-prod", "For example roodox-prod")} /></div>
                      <div><label>{t("网络密钥", "Network secret")}</label><input value={easytierNetworkSecret} onChange={(e) => updateOverlayField("networkSecret", e.target.value)} placeholder={t("可选", "Optional")} /></div>
                      <div className="wide"><label>{t("Peer Targets", "Peer targets")}</label><textarea value={easytierPeerTargetsText} onChange={(e) => updateOverlayListField("peerTargets", e.target.value)} placeholder={t("每行一个，例如 tcp://cp.roodox.internal:11010", "One per line, for example tcp://cp.roodox.internal:11010")} /></div>
                    </>
                  ) : null}
                  {overlayPreset === "direct" ? <div className="wide inline-note">{t("Direct 模式不需要额外 overlay bootstrap JSON。", "Direct mode does not require extra overlay bootstrap JSON.")}</div> : null}
                  <div className="wide">
                    <div className="toolbar">
                      <label>{t("高级 Overlay JSON", "Advanced overlay JSON")}</label>
                      <button className="ghost" onClick={() => setShowAdvancedOverlay((value) => !value)}>{showAdvancedOverlay ? t("收起高级区", "Hide advanced") : t("展开高级区", "Show advanced")}</button>
                    </div>
                    {(showAdvancedOverlay || overlayPreset === "custom" || !!overlayConfigError) ? (
                      <textarea value={config.bundle_overlay_join_config_json} onChange={(e) => setConfig((value) => ({ ...value, bundle_overlay_join_config_json: e.target.value }))} placeholder={t("这里只放 overlay bootstrap JSON；普通用户优先用上面的结构化字段。", "Keep only overlay bootstrap JSON here; regular users should prefer the structured fields above.")} />
                    ) : null}
                  </div>
                </div>
                <label className="check-row"><input type="checkbox" checked={config.bundle_use_tls} onChange={(e) => setConfig((value) => ({ ...value, bundle_use_tls: e.target.checked }))} /><span>{t("接入包声明使用 TLS", "Declare TLS in the access bundle")}</span></label>
                <div className="action-row">
                  <button className="primary" onClick={() => void safeRun(async () => await persistConfig(buildConfigForSave(), t("接入设置已保存", "Access settings saved")))}>{t("保存接入设置", "Save access settings")}</button>
                  <button className="secondary" onClick={() => void safeRun(async () => { await persistConfig(buildConfigForSave(), t("接入设置已保存", "Access settings saved")); await refreshAccessPreview(); setMessage(t("Join Bundle 预览已刷新", "Join bundle preview refreshed")); })}>{t("生成接入预览", "Generate preview")}</button>
                </div>
              </article>
              <article className="subpanel access-device-card">
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
                  <div>
                    <label>{t("数据目录", "Data root")}</label>
                    <div className="path-field">
                      <input value={config.data_root} readOnly />
                      <button className="secondary" onClick={() => void safeRun(async () => await chooseDirectory(config.data_root, (path) => setConfig((v) => ({ ...v, data_root: path }))))}>{t("浏览", "Browse")}</button>
                    </div>
                    <p className="form-note compact-note">{t("数据目录只能通过浏览按钮更改，避免手动覆写到错误位置。", "Change the data root only through the browse button to avoid accidentally overwriting it with a bad path.")}</p>
                  </div>
                  <div className="wide"><label>{t("共享根目录", "Share root")}</label><div className="path-field"><input value={config.root_dir} onChange={(e) => setConfig((v) => ({ ...v, root_dir: e.target.value }))} /><button className="secondary" onClick={() => void safeRun(async () => await chooseDirectory(config.root_dir, (path) => setConfig((v) => ({ ...v, root_dir: path }))))}>{t("浏览", "Browse")}</button></div></div>
                </div>
                <label className="check-row"><input type="checkbox" checked={config.remote_build_enabled} onChange={(e) => {
                  const enabled = e.target.checked;
                  setConfig((v) => ({ ...v, remote_build_enabled: enabled }));
                  if (enabled && hasMissingTools) {
                    setMessage(t("远程构建依赖额外工具。可在下方直接一键安装。", "Remote build needs extra tools. Install them below."));
                  }
                }} /><span>{t("启用远程构建", "Enable remote build")}</span></label>
              </article>
              <article className="subpanel"><h2>{t("传输设置", "Transport")}</h2><label className="check-row"><input type="checkbox" checked={config.tls_enabled} onChange={(e) => setConfig((v) => ({ ...v, tls_enabled: e.target.checked }))} /><span>{t("启用 TLS", "Enable TLS")}</span></label><div className="form-grid"><div><label>{t("TLS 证书路径", "TLS cert path")}</label><div className="path-field"><input value={config.tls_cert_path} onChange={(e) => setConfig((v) => ({ ...v, tls_cert_path: e.target.value }))} /><button className="secondary" onClick={() => void safeRun(async () => await chooseFile(config.tls_cert_path, (path) => setConfig((v) => ({ ...v, tls_cert_path: path }))))}>{t("浏览", "Browse")}</button></div></div><div><label>{t("TLS 私钥路径", "TLS key path")}</label><div className="path-field"><input value={config.tls_key_path} onChange={(e) => setConfig((v) => ({ ...v, tls_key_path: e.target.value }))} /><button className="secondary" onClick={() => void safeRun(async () => await chooseFile(config.tls_key_path, (path) => setConfig((v) => ({ ...v, tls_key_path: path }))))}>{t("浏览", "Browse")}</button></div></div></div></article>
              <article className="subpanel">
                <div className="panel-head compact">
                  <div>
                    <h2>{t("工具检测", "Tool detection")}</h2>
                    <p className="form-note">{t("这些工具只在启用远程构建时才需要。安装后会自动检测状态；这里只保留检测和一键安装，不再暴露目录与原始字段。", "These tools are only needed when remote build is enabled. Status is detected automatically after install, and raw directories/fields are no longer exposed here.")}</p>
                  </div>
                  <div className="action-row compact-actions">
                    <button className="secondary" onClick={() => void safeRun(async () => setEnv(sanitizeEnvCheck(await invoke("check_environment"))))}>{t("重新检测", "Re-check")}</button>
                    <button className="primary" disabled={installToolsDisabled} onClick={() => void safeRun(async () => { await invoke("install_missing_tools"); setMessage(t("工具安装器已启动，完成后可重新检测。", "Tool installer started. Re-check after it finishes.")); await refreshStatus(); })}>{installToolsLabel}</button>
                  </div>
                </div>
                {env ? (
                  <div className="stack-list">
                    <div className="info-card">
                      <div className="info-card-head">
                        <strong>{t("系统环境", "Environment")}</strong>
                        <span className={`state-pill ${toolIssuesActive ? "degraded" : "online"}`}>{buildToolsRequired ? (hasMissingTools ? t("缺少工具", "Missing tools") : t("已就绪", "Ready")) : t("按需启用", "Optional")}</span>
                      </div>
                      <dl className="detail-grid">
                        <div><dt>OS</dt><dd>{env.os}</dd></div>
                        <div><dt>winget</dt><dd>{env.winget_installed ? t("可用", "available") : t("缺失", "missing")}</dd></div>
                        <div className="wide"><dt>{t("工具状态", "Tool status")}</dt><dd>{Object.entries(env.tools ?? {}).map(([name, tool]) => `${name}: ${tool.installed ? t("可用", "available") : t("缺失", "missing")}`).join(" · ") || none}</dd></div>
                      </dl>
                    </div>
                    {!buildToolsRequired ? <div className="inline-note">{t("当前未启用远程构建，因此缺少这些工具不会影响服务启动和客户端接入。", "Remote build is currently off, so missing tools do not block server startup or client access.")}</div> : null}
                    {!env.winget_installed && hasMissingTools ? <div className="inline-warning">{t("没有检测到 winget，自动安装工具会失败。", "winget is not available, so automatic tool installation will fail.")}</div> : null}
                    {envWarningText ? <div className="inline-warning">{envWarningText}</div> : null}
                  </div>
                ) : <div className="empty-state">{t("暂无环境数据", "No environment data")}</div>}
              </article>
            </div>
            <div className="action-row"><button className="primary" onClick={() => void safeRun(async () => { await persistConfig(buildConfigForSave(), t("配置已保存", "Config saved")); })}>{t("保存设置", "Save settings")}</button><button className="secondary" onClick={() => void safeRun(loadConfig)}>{t("重新加载", "Reload")}</button></div>
          </section>
        ) : null}
        </div>
      </main>
      </div>
    </div>
  );
}
