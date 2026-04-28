package serverapp

import (
	"testing"

	"roodox_server/internal/appconfig"
)

func TestBuildControlPlaneConfigUsesStructuredJoinBundleFallbacks(t *testing.T) {
	cfg := appconfig.Config{
		Addr:         "10.20.30.40:50051",
		AuthEnabled:  true,
		SharedSecret: "shared-secret",
		TLSEnabled:   true,
		ControlPlane: appconfig.ControlPlaneConfig{
			ServerID:                 "srv-main",
			DefaultDeviceGroup:       "team-a",
			HeartbeatIntervalSeconds: 25,
			DefaultPolicyRevision:    7,
			AssignedConfig: appconfig.ClientAssignedConfig{
				MountPath:          "/Volumes/Roodox",
				SyncRoots:          []string{"src", "docs"},
				ConflictPolicy:     "manual",
				AutoConnect:        true,
				LogLevel:           "info",
				LargeFileThreshold: 32 << 20,
			},
			JoinBundle: appconfig.JoinBundleControlConfig{
				OverlayProvider:       "custom-overlay",
				OverlayJoinConfigJSON: `{"token":"abc"}`,
				ServiceDiscovery: appconfig.ServiceDiscoveryConfig{
					Mode:          "static",
					TLSServerName: "roodox.internal",
				},
			},
		},
	}

	got := buildControlPlaneConfig(cfg)

	if got.JoinBundle.OverlayProvider != "custom-overlay" {
		t.Fatalf("JoinBundle.OverlayProvider = %q, want %q", got.JoinBundle.OverlayProvider, "custom-overlay")
	}
	if got.JoinBundle.OverlayJoinConfigJSON != `{"token":"abc"}` {
		t.Fatalf("JoinBundle.OverlayJoinConfigJSON = %q, want %q", got.JoinBundle.OverlayJoinConfigJSON, `{"token":"abc"}`)
	}
	if got.JoinBundle.ServiceDiscovery.Mode != "static" {
		t.Fatalf("JoinBundle.ServiceDiscovery.Mode = %q, want %q", got.JoinBundle.ServiceDiscovery.Mode, "static")
	}
	if got.JoinBundle.ServiceDiscovery.Host != "10.20.30.40" {
		t.Fatalf("JoinBundle.ServiceDiscovery.Host = %q, want %q", got.JoinBundle.ServiceDiscovery.Host, "10.20.30.40")
	}
	if got.JoinBundle.ServiceDiscovery.Port != 50051 {
		t.Fatalf("JoinBundle.ServiceDiscovery.Port = %d, want %d", got.JoinBundle.ServiceDiscovery.Port, 50051)
	}
	if !got.JoinBundle.ServiceDiscovery.UseTLS {
		t.Fatal("JoinBundle.ServiceDiscovery.UseTLS = false, want true")
	}
	if got.JoinBundle.ServiceDiscovery.TLSServerName != "roodox.internal" {
		t.Fatalf("JoinBundle.ServiceDiscovery.TLSServerName = %q, want %q", got.JoinBundle.ServiceDiscovery.TLSServerName, "roodox.internal")
	}
	if got.JoinBundle.SharedSecret != "shared-secret" {
		t.Fatalf("JoinBundle.SharedSecret = %q, want %q", got.JoinBundle.SharedSecret, "shared-secret")
	}
}

func TestBuildControlPlaneConfigPreservesExplicitServiceDiscovery(t *testing.T) {
	cfg := appconfig.Config{
		Addr:        "127.0.0.1:50051",
		AuthEnabled: false,
		TLSEnabled:  false,
		ControlPlane: appconfig.ControlPlaneConfig{
			ServerID:           "srv-main",
			DefaultDeviceGroup: "team-a",
			JoinBundle: appconfig.JoinBundleControlConfig{
				OverlayProvider:       "overlay-x",
				OverlayJoinConfigJSON: `{"network":"prod"}`,
				ServiceDiscovery: appconfig.ServiceDiscoveryConfig{
					Mode:          "dns",
					Host:          "cp.roodox.internal",
					Port:          5443,
					UseTLS:        true,
					TLSServerName: "roodox.internal",
				},
			},
		},
	}

	got := buildControlPlaneConfig(cfg)

	if got.JoinBundle.ServiceDiscovery.Mode != "dns" {
		t.Fatalf("JoinBundle.ServiceDiscovery.Mode = %q, want %q", got.JoinBundle.ServiceDiscovery.Mode, "dns")
	}
	if got.JoinBundle.ServiceDiscovery.Host != "cp.roodox.internal" {
		t.Fatalf("JoinBundle.ServiceDiscovery.Host = %q, want %q", got.JoinBundle.ServiceDiscovery.Host, "cp.roodox.internal")
	}
	if got.JoinBundle.ServiceDiscovery.Port != 5443 {
		t.Fatalf("JoinBundle.ServiceDiscovery.Port = %d, want %d", got.JoinBundle.ServiceDiscovery.Port, 5443)
	}
	if !got.JoinBundle.ServiceDiscovery.UseTLS {
		t.Fatal("JoinBundle.ServiceDiscovery.UseTLS = false, want true")
	}
	if got.JoinBundle.ServiceDiscovery.TLSServerName != "roodox.internal" {
		t.Fatalf("JoinBundle.ServiceDiscovery.TLSServerName = %q, want %q", got.JoinBundle.ServiceDiscovery.TLSServerName, "roodox.internal")
	}
	if got.JoinBundle.SharedSecret != "" {
		t.Fatalf("JoinBundle.SharedSecret = %q, want empty string", got.JoinBundle.SharedSecret)
	}
}
