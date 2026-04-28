package accessbundle

import (
	"strings"
	"testing"
)

func TestBundleNormalizeAndMarshal(t *testing.T) {
	bundle := Bundle{
		Overlay: Overlay{
			Provider:   "TailScale",
			JoinConfig: []byte(`{"authKey":"tskey"}`),
		},
	ServiceDiscovery: ServiceDiscovery{
			Mode:          "Static",
			Host:          "cp.roodox.internal",
			Port:          50051,
			UseTLS:        true,
			TLSServerName: "roodox.internal",
		},
		Roodox: Roodox{
			ServerID:     "srv-main",
			DeviceGroup:  "default",
			SharedSecret: "secret-1",
			DeviceID:     "device-1",
		},
	}

	normalized := bundle.Normalize()
	if normalized.Overlay.Provider != "tailscale" {
		t.Fatalf("Overlay.Provider = %q, want %q", normalized.Overlay.Provider, "tailscale")
	}
	if normalized.ServiceDiscovery.Mode != "static" {
		t.Fatalf("ServiceDiscovery.Mode = %q, want %q", normalized.ServiceDiscovery.Mode, "static")
	}

	raw, err := normalized.MarshalJSONFile()
	if err != nil {
		t.Fatalf("MarshalJSONFile returned error: %v", err)
	}
	if !strings.Contains(raw, `"overlayProvider": "tailscale"`) {
		t.Fatalf("MarshalJSONFile output = %q, want tailscale overlay provider", raw)
	}
}

func TestBundleValidateRejectsMissingStaticEndpoint(t *testing.T) {
	bundle := Bundle{
		Overlay: Overlay{
			Provider:   "easytier",
			JoinConfig: []byte(`{"networkName":"roodox"}`),
		},
		ServiceDiscovery: ServiceDiscovery{
			Mode: "static",
		},
		Roodox: Roodox{
			ServerID:    "srv-main",
			DeviceGroup: "default",
		},
	}

	if err := bundle.Validate(); err == nil {
		t.Fatal("Validate unexpectedly succeeded without service host/port")
	}
}
