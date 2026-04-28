package serverapp

import (
	"net"
	"os"
	"strings"
	"unicode"

	"roodox_server/internal/appconfig"
	"roodox_server/internal/server"
)

func BuildControlPlaneConfig(cfg appconfig.Config) server.ControlPlaneConfig {
	serviceHost, servicePort := parseServiceDiscoveryAddress(cfg.Addr)
	sharedSecret := ""
	if cfg.AuthEnabled {
		sharedSecret = strings.TrimSpace(cfg.SharedSecret)
	}

	serverID := strings.TrimSpace(cfg.ControlPlane.ServerID)
	if serverID == "" {
		serverID = resolveServerID()
	}
	defaultDeviceGroup := strings.TrimSpace(cfg.ControlPlane.DefaultDeviceGroup)
	if defaultDeviceGroup == "" {
		defaultDeviceGroup = "default"
	}
	assigned := cfg.ControlPlane.AssignedConfig
	joinBundle := cfg.ControlPlane.JoinBundle
	discovery := joinBundle.ServiceDiscovery
	if strings.TrimSpace(discovery.Host) == "" {
		discovery.Host = serviceHost
	}
	if discovery.Port == 0 {
		discovery.Port = servicePort
	}
	if !discovery.UseTLS {
		discovery.UseTLS = cfg.TLSEnabled
	}

	return server.ControlPlaneConfig{
		ServerID:                 serverID,
		DefaultDeviceGroup:       defaultDeviceGroup,
		HeartbeatIntervalSeconds: uint32(cfg.ControlPlane.HeartbeatIntervalSeconds),
		DefaultPolicyRevision:    cfg.ControlPlane.DefaultPolicyRevision,
		DefaultAssignedConfig: server.AssignedConfig{
			MountPath:          assigned.MountPath,
			SyncRoots:          append([]string(nil), assigned.SyncRoots...),
			ConflictPolicy:     assigned.ConflictPolicy,
			ReadOnly:           assigned.ReadOnly,
			AutoConnect:        assigned.AutoConnect,
			BandwidthLimit:     assigned.BandwidthLimit,
			LogLevel:           assigned.LogLevel,
			LargeFileThreshold: assigned.LargeFileThreshold,
		},
		JoinBundle: server.JoinBundleConfig{
			OverlayProvider:       joinBundle.OverlayProvider,
			OverlayJoinConfigJSON: joinBundle.OverlayJoinConfigJSON,
			ServiceDiscovery: server.ServiceDiscoveryConfig{
				Mode:          discovery.Mode,
				Host:          discovery.Host,
				Port:          discovery.Port,
				UseTLS:        discovery.UseTLS,
				TLSServerName: discovery.TLSServerName,
			},
			SharedSecret: sharedSecret,
		},
		AvailableActions:      append([]string(nil), cfg.ControlPlane.AvailableActions...),
		DiagnosticsKeepLatest: cfg.ControlPlane.DiagnosticsKeepLatest,
	}
}

func buildControlPlaneConfig(cfg appconfig.Config) server.ControlPlaneConfig {
	return BuildControlPlaneConfig(cfg)
}

func resolveServerID() string {
	if value := strings.TrimSpace(os.Getenv("ROODOX_SERVER_ID")); value != "" {
		return value
	}
	host, err := os.Hostname()
	if err != nil {
		return "srv-main"
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "srv-main"
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(host) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}
	value := strings.Trim(builder.String(), "-")
	if value == "" {
		return "srv-main"
	}
	return value
}

func parseServiceDiscoveryAddress(addr string) (string, uint32) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return "", 0
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = ""
	}

	if strings.TrimSpace(portText) == "" {
		return host, 0
	}

	var port uint32
	for _, r := range portText {
		if r < '0' || r > '9' {
			return host, 0
		}
		port = port*10 + uint32(r-'0')
	}
	return host, port
}
