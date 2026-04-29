package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"roodox_server/client"
	"roodox_server/internal/accessbundle"
)

const (
	connectionCodePrefix = "roodox:1:"
	connectionCodeName   = "roodox-connection-code.txt"
	bundleFileName       = "roodox-client-access.json"
	caFileName           = "roodox-ca-cert.pem"
)

type connectionCodeEnvelope struct {
	Version uint32          `json:"version"`
	Bundle  json.RawMessage `json:"bundle"`
	CAPEM   string          `json:"ca_pem,omitempty"`
}

type bundleFile struct {
	Version           uint32          `json:"version"`
	OverlayProvider   string          `json:"overlayProvider"`
	OverlayJoinConfig json.RawMessage `json:"overlayJoinConfig"`
	ServiceDiscovery  bundleDiscovery `json:"serviceDiscovery"`
	Roodox            bundleRoodox    `json:"roodox"`
}

type bundleDiscovery struct {
	Mode          string `json:"mode"`
	Host          string `json:"host"`
	Port          uint32 `json:"port"`
	UseTLS        bool   `json:"useTLS"`
	TLSServerName string `json:"tlsServerName"`
}

type bundleRoodox struct {
	ServerID     string `json:"serverID"`
	DeviceGroup  string `json:"deviceGroup"`
	SharedSecret string `json:"sharedSecret"`
	DeviceID     string `json:"deviceID,omitempty"`
	DeviceName   string `json:"deviceName,omitempty"`
	DeviceRole   string `json:"deviceRole,omitempty"`
}

type decodedConnectionCode struct {
	bundle     accessbundle.Bundle
	bundleJSON []byte
	caPEM      string
}

type importResult struct {
	Format         string `json:"format"`
	OutputDir      string `json:"output_dir"`
	ConnectionCode string `json:"connection_code_path"`
	BundlePath     string `json:"bundle_path"`
	CAPath         string `json:"ca_path,omitempty"`
	ServiceAddr    string `json:"service_addr"`
	TLSEnabled     bool   `json:"tls_enabled"`
	ProbeAttempted bool   `json:"probe_attempted"`
	ProbeOK        bool   `json:"probe_ok"`
	ProbeError     string `json:"probe_error,omitempty"`
}

func main() {
	connectionCode := flag.String("connection-code", "", "Inline connection code or roodox:// URI")
	connectionCodeFile := flag.String("connection-code-file", "", "Path to a text file containing the connection code")
	outputDir := flag.String("output-dir", ".", "Directory to write the client handoff files into")
	probe := flag.Bool("probe", false, "Probe the target server after writing the files")
	probeTimeoutSeconds := flag.Int("probe-timeout-seconds", 5, "Timeout in seconds used by -probe")
	flag.Parse()

	rawCode, err := loadConnectionCode(*connectionCode, *connectionCodeFile)
	if err != nil {
		exitWithError(err)
	}

	decoded, err := decodeConnectionCode(rawCode)
	if err != nil {
		exitWithError(err)
	}

	result, err := writeImportArtifacts(*outputDir, rawCode, decoded)
	if err != nil {
		exitWithError(err)
	}

	if *probe {
		result.ProbeAttempted = true
		timeout := time.Duration(max(*probeTimeoutSeconds, 1)) * time.Second
		if err := probeServer(decoded.bundle, result.CAPath, timeout); err != nil {
			result.ProbeError = err.Error()
		} else {
			result.ProbeOK = true
		}
	}

	writeJSON(result)
	if result.ProbeAttempted && !result.ProbeOK {
		os.Exit(1)
	}
}

func loadConnectionCode(inlineCode, filePath string) (string, error) {
	inlineCode = strings.TrimSpace(inlineCode)
	filePath = strings.TrimSpace(filePath)
	switch {
	case inlineCode != "" && filePath != "":
		return "", errors.New("specify either -connection-code or -connection-code-file, not both")
	case inlineCode != "":
		return inlineCode, nil
	case filePath != "":
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read connection code file failed: %w", err)
		}
		return string(data), nil
	default:
		return "", errors.New("connection code is required")
	}
}

func decodeConnectionCode(raw string) (decodedConnectionCode, error) {
	encoded, err := extractEncodedPayload(raw)
	if err != nil {
		return decodedConnectionCode{}, err
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return decodedConnectionCode{}, fmt.Errorf("decode connection code failed: %w", err)
	}

	var envelope connectionCodeEnvelope
	if err := json.Unmarshal(payloadBytes, &envelope); err != nil {
		return decodedConnectionCode{}, fmt.Errorf("parse connection code payload failed: %w", err)
	}
	if envelope.Version != 1 {
		return decodedConnectionCode{}, fmt.Errorf("unsupported connection code version: %d", envelope.Version)
	}

	bundle, bundleJSON, err := parseBundleJSON(envelope.Bundle)
	if err != nil {
		return decodedConnectionCode{}, err
	}
	caPEM := normalizePEM(envelope.CAPEM)
	if bundle.ServiceDiscovery.UseTLS && caPEM == "" {
		return decodedConnectionCode{}, errors.New("connection code is missing the TLS root certificate")
	}

	return decodedConnectionCode{
		bundle:     bundle,
		bundleJSON: bundleJSON,
		caPEM:      caPEM,
	}, nil
}

func extractEncodedPayload(raw string) (string, error) {
	code := strings.Join(strings.Fields(strings.TrimSpace(raw)), "")
	switch {
	case strings.HasPrefix(code, connectionCodePrefix):
		payload := strings.TrimPrefix(code, connectionCodePrefix)
		if payload == "" {
			return "", errors.New("connection code payload is empty")
		}
		return payload, nil
	case strings.HasPrefix(strings.ToLower(code), "roodox://connect"):
		parsed, err := url.Parse(code)
		if err != nil {
			return "", fmt.Errorf("parse connection uri failed: %w", err)
		}
		payload := strings.TrimSpace(parsed.Query().Get("payload"))
		if payload == "" {
			return "", errors.New("connection uri is missing payload")
		}
		return payload, nil
	default:
		return "", errors.New("unsupported connection code format")
	}
}

func parseBundleJSON(raw []byte) (accessbundle.Bundle, []byte, error) {
	if len(raw) == 0 {
		return accessbundle.Bundle{}, nil, errors.New("connection code bundle is empty")
	}

	var payload bundleFile
	if err := json.Unmarshal(raw, &payload); err != nil {
		return accessbundle.Bundle{}, nil, fmt.Errorf("parse join bundle failed: %w", err)
	}
	bundle := accessbundle.Bundle{
		Version: payload.Version,
		Overlay: accessbundle.Overlay{
			Provider:   payload.OverlayProvider,
			JoinConfig: append(json.RawMessage(nil), payload.OverlayJoinConfig...),
		},
		ServiceDiscovery: accessbundle.ServiceDiscovery{
			Mode:          payload.ServiceDiscovery.Mode,
			Host:          payload.ServiceDiscovery.Host,
			Port:          payload.ServiceDiscovery.Port,
			UseTLS:        payload.ServiceDiscovery.UseTLS,
			TLSServerName: payload.ServiceDiscovery.TLSServerName,
		},
		Roodox: accessbundle.Roodox{
			ServerID:     payload.Roodox.ServerID,
			DeviceGroup:  payload.Roodox.DeviceGroup,
			SharedSecret: payload.Roodox.SharedSecret,
			DeviceID:     payload.Roodox.DeviceID,
			DeviceName:   payload.Roodox.DeviceName,
			DeviceRole:   payload.Roodox.DeviceRole,
		},
	}.Normalize()

	if err := bundle.Validate(); err != nil {
		return accessbundle.Bundle{}, nil, fmt.Errorf("validate join bundle failed: %w", err)
	}

	prettyBundle, err := bundle.MarshalJSONFile()
	if err != nil {
		return accessbundle.Bundle{}, nil, fmt.Errorf("encode join bundle failed: %w", err)
	}
	return bundle, []byte(prettyBundle), nil
}

func writeImportArtifacts(outputDir, rawCode string, decoded decodedConnectionCode) (importResult, error) {
	resolvedOutputDir, err := filepath.Abs(strings.TrimSpace(outputDir))
	if err != nil {
		return importResult{}, fmt.Errorf("resolve output dir failed: %w", err)
	}
	if err := os.MkdirAll(resolvedOutputDir, 0o755); err != nil {
		return importResult{}, fmt.Errorf("create output dir failed: %w", err)
	}

	connectionCodePath := filepath.Join(resolvedOutputDir, connectionCodeName)
	if err := os.WriteFile(connectionCodePath, []byte(strings.TrimSpace(rawCode)+"\n"), 0o644); err != nil {
		return importResult{}, fmt.Errorf("write connection code file failed: %w", err)
	}

	bundlePath := filepath.Join(resolvedOutputDir, bundleFileName)
	if err := os.WriteFile(bundlePath, decoded.bundleJSON, 0o644); err != nil {
		return importResult{}, fmt.Errorf("write join bundle failed: %w", err)
	}

	caPath := ""
	if decoded.caPEM != "" {
		caPath = filepath.Join(resolvedOutputDir, caFileName)
		if err := os.WriteFile(caPath, []byte(decoded.caPEM), 0o644); err != nil {
			return importResult{}, fmt.Errorf("write tls root certificate failed: %w", err)
		}
	}

	return importResult{
		Format:         "roodox:1",
		OutputDir:      resolvedOutputDir,
		ConnectionCode: connectionCodePath,
		BundlePath:     bundlePath,
		CAPath:         caPath,
		ServiceAddr:    bundleServiceAddr(decoded.bundle),
		TLSEnabled:     decoded.bundle.ServiceDiscovery.UseTLS,
	}, nil
}

func probeServer(bundle accessbundle.Bundle, caPath string, timeout time.Duration) error {
	options := client.ConnectionOptions{
		SharedSecret: strings.TrimSpace(bundle.Roodox.SharedSecret),
		TLSEnabled:   bundle.ServiceDiscovery.UseTLS,
	}
	if bundle.ServiceDiscovery.UseTLS {
		if strings.TrimSpace(caPath) == "" {
			return errors.New("probe requires a TLS root certificate path")
		}
		options.TLSRootCertPath = caPath
		options.TLSServerName = strings.TrimSpace(bundle.ServiceDiscovery.TLSServerName)
	}

	c, err := client.NewRoodoxClientWithOptions(bundleServiceAddr(bundle), options)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if _, err := c.HealthCheck(ctx); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}

func bundleServiceAddr(bundle accessbundle.Bundle) string {
	host := strings.TrimSpace(bundle.ServiceDiscovery.Host)
	port := fmt.Sprintf("%d", bundle.ServiceDiscovery.Port)
	return net.JoinHostPort(strings.Trim(host, "[]"), port)
}

func normalizePEM(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	return trimmed + "\n"
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		exitWithError(err)
	}
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func max(value, floor int) int {
	if value < floor {
		return floor
	}
	return value
}
