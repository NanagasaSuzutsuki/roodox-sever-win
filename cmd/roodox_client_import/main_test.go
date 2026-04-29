package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"roodox_server/internal/accessbundle"
)

func TestDecodeConnectionCodeRaw(t *testing.T) {
	code := buildTestConnectionCode(t, accessbundle.Bundle{
		Overlay: accessbundle.Overlay{
			Provider:   "direct",
			JoinConfig: json.RawMessage(`{}`),
		},
		ServiceDiscovery: accessbundle.ServiceDiscovery{
			Mode:          "static",
			Host:          "server.example.com",
			Port:          50051,
			UseTLS:        true,
			TLSServerName: "server.example.com",
		},
		Roodox: accessbundle.Roodox{
			ServerID:     "srv-main",
			DeviceGroup:  "default",
			SharedSecret: "secret-1",
			DeviceName:   "client-a",
		},
	}.Normalize(), "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n")

	decoded, err := decodeConnectionCode(code)
	if err != nil {
		t.Fatalf("decodeConnectionCode returned error: %v", err)
	}

	if decoded.bundle.ServiceDiscovery.Host != "server.example.com" {
		t.Fatalf("unexpected host: %q", decoded.bundle.ServiceDiscovery.Host)
	}
	if !decoded.bundle.ServiceDiscovery.UseTLS {
		t.Fatal("expected TLS to stay enabled")
	}
	if !strings.Contains(decoded.caPEM, "BEGIN CERTIFICATE") {
		t.Fatalf("expected CA PEM, got %q", decoded.caPEM)
	}
}

func TestDecodeConnectionCodeURI(t *testing.T) {
	raw := buildTestConnectionCode(t, accessbundle.Bundle{
		Overlay: accessbundle.Overlay{
			Provider:   "tailscale",
			JoinConfig: json.RawMessage(`{"tailnet":"example.ts.net"}`),
		},
		ServiceDiscovery: accessbundle.ServiceDiscovery{
			Mode:   "static",
			Host:   "100.64.0.10",
			Port:   50051,
			UseTLS: false,
		},
		Roodox: accessbundle.Roodox{
			ServerID:    "srv-main",
			DeviceGroup: "default",
		},
	}.Normalize(), "")

	uri := "roodox://connect?v=1&payload=" + strings.TrimPrefix(raw, connectionCodePrefix)
	decoded, err := decodeConnectionCode(uri)
	if err != nil {
		t.Fatalf("decodeConnectionCode returned error: %v", err)
	}

	if decoded.bundle.Overlay.Provider != "tailscale" {
		t.Fatalf("unexpected overlay provider: %q", decoded.bundle.Overlay.Provider)
	}
	if decoded.caPEM != "" {
		t.Fatalf("expected no CA PEM, got %q", decoded.caPEM)
	}
}

func TestDecodeConnectionCodeRejectsMissingTLSCA(t *testing.T) {
	code := buildTestConnectionCode(t, accessbundle.Bundle{
		Overlay: accessbundle.Overlay{
			Provider:   "direct",
			JoinConfig: json.RawMessage(`{}`),
		},
		ServiceDiscovery: accessbundle.ServiceDiscovery{
			Mode:          "static",
			Host:          "server.example.com",
			Port:          50051,
			UseTLS:        true,
			TLSServerName: "server.example.com",
		},
		Roodox: accessbundle.Roodox{
			ServerID:    "srv-main",
			DeviceGroup: "default",
		},
	}.Normalize(), "")

	if _, err := decodeConnectionCode(code); err == nil {
		t.Fatal("expected decodeConnectionCode to reject a TLS bundle without CA PEM")
	}
}

func buildTestConnectionCode(t *testing.T, bundle accessbundle.Bundle, caPEM string) string {
	t.Helper()

	bundleJSON, err := bundle.MarshalJSONFile()
	if err != nil {
		t.Fatalf("MarshalJSONFile returned error: %v", err)
	}

	payload, err := json.Marshal(connectionCodeEnvelope{
		Version: 1,
		Bundle:  json.RawMessage(bundleJSON),
		CAPEM:   caPEM,
	})
	if err != nil {
		t.Fatalf("Marshal payload returned error: %v", err)
	}

	return connectionCodePrefix + base64.RawURLEncoding.EncodeToString(payload)
}
