package qasuite

import (
	"context"
	"errors"
	"fmt"
	"net"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"roodox_server/client"
	"roodox_server/internal/appconfig"
	pb "roodox_server/proto"
)

const defaultRootCAFileName = "roodox-ca-cert.pem"

type Override struct {
	ConfigPath      string
	Addr            string
	RootDir         string
	SharedSecret    string
	TLSRootCertPath string
	TLSServerName   string
	ServerID        string
}

type Runtime struct {
	ConfigPath         string
	ConfigDir          string
	DialAddr           string
	RootDir            string
	SharedSecret       string
	TLSEnabled         bool
	TLSRootCertPath    string
	TLSServerName      string
	ServerID           string
	RemoteBuildEnabled bool
}

func LoadRuntime(override Override) (Runtime, error) {
	configPath := strings.TrimSpace(override.ConfigPath)
	if configPath == "" {
		configPath = appconfig.ConfigPath
	}
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return Runtime{}, err
	}
	configDir := filepath.Dir(absConfigPath)

	cfg, err := appconfig.Load(absConfigPath)
	if err != nil {
		return Runtime{}, err
	}

	rootDir := resolveMaybeRelative(configDir, cfg.RootDir)
	if v := strings.TrimSpace(override.RootDir); v != "" {
		rootDir = resolveMaybeRelative(configDir, v)
	}

	dialAddr := normalizeDialAddr(cfg.Addr)
	if v := strings.TrimSpace(override.Addr); v != "" {
		dialAddr = normalizeDialAddr(v)
	}

	tlsRootCertPath := ""
	if cfg.TLSEnabled {
		tlsRootCertPath = defaultTLSRootCertPath(resolveMaybeRelative(configDir, cfg.TLSCertPath))
	}
	if v := strings.TrimSpace(override.TLSRootCertPath); v != "" {
		tlsRootCertPath = resolveMaybeRelative(configDir, v)
	}

	tlsServerName := defaultTLSServerName(cfg, dialAddr)
	if v := strings.TrimSpace(override.TLSServerName); v != "" {
		tlsServerName = v
	}

	sharedSecret := strings.TrimSpace(cfg.SharedSecret)
	if v := strings.TrimSpace(override.SharedSecret); v != "" {
		sharedSecret = v
	}

	serverID := strings.TrimSpace(cfg.ControlPlane.ServerID)
	if v := strings.TrimSpace(override.ServerID); v != "" {
		serverID = v
	}

	return Runtime{
		ConfigPath:         absConfigPath,
		ConfigDir:          configDir,
		DialAddr:           dialAddr,
		RootDir:            rootDir,
		SharedSecret:       sharedSecret,
		TLSEnabled:         cfg.TLSEnabled,
		TLSRootCertPath:    tlsRootCertPath,
		TLSServerName:      tlsServerName,
		ServerID:           serverID,
		RemoteBuildEnabled: cfg.RemoteBuildEnabled,
	}, nil
}

func (r Runtime) Dial() (*client.RoodoxClient, error) {
	return client.NewRoodoxClientWithOptions(r.DialAddr, client.ConnectionOptions{
		SharedSecret:    r.SharedSecret,
		TLSEnabled:      r.TLSEnabled,
		TLSRootCertPath: r.TLSRootCertPath,
		TLSServerName:   r.TLSServerName,
	})
}

func OpContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

func ResolveRunRoot(rootDir, rel string) (string, error) {
	rootDir = filepath.Clean(rootDir)
	full := filepath.Clean(filepath.Join(rootDir, filepath.FromSlash(rel)))
	relative, err := filepath.Rel(rootDir, full)
	if err != nil {
		return "", err
	}
	if relative == "." || relative == "" || strings.HasPrefix(relative, "..") {
		return "", fmt.Errorf("refusing to resolve path outside root: rel=%q root=%q", rel, rootDir)
	}
	return full, nil
}

func BuildRunID(prefix string) string {
	now := time.Now().UTC()
	return fmt.Sprintf("%s-%s", prefix, now.Format("20060102-150405"))
}

func BuildRunRelRoot(kind string) string {
	return pathpkg.Join("qa", BuildRunID(kind))
}

func defaultTLSRootCertPath(certPath string) string {
	return filepath.Join(filepath.Dir(certPath), defaultRootCAFileName)
}

func defaultTLSServerName(cfg appconfig.Config, dialAddr string) string {
	if v := strings.TrimSpace(cfg.ControlPlane.JoinBundle.ServiceDiscovery.TLSServerName); v != "" {
		return v
	}
	host, _, err := net.SplitHostPort(dialAddr)
	if err == nil {
		host = strings.Trim(host, "[]")
		if host != "" {
			if ip := net.ParseIP(host); ip == nil {
				return host
			}
			if host == "127.0.0.1" || host == "::1" {
				return "localhost"
			}
		}
	}
	return "localhost"
}

func normalizeDialAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "127.0.0.1:50051"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return "127.0.0.1" + addr
		}
		return addr
	}

	switch strings.Trim(host, "[]") {
	case "", "0.0.0.0", "::":
		return net.JoinHostPort("127.0.0.1", port)
	default:
		return net.JoinHostPort(strings.Trim(host, "[]"), port)
	}
}

func resolveMaybeRelative(baseDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(baseDir, value))
}

func WaitBuildTerminal(ctx context.Context, c *client.RoodoxClient, buildID string, timeout time.Duration) (*pb.GetBuildStatusResponse, *pb.FetchBuildLogResponse, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		opCtx, cancel := OpContext(ctx, 8*time.Second)
		statusResp, err := c.GetBuildStatus(opCtx, buildID)
		cancel()
		if err != nil {
			return nil, nil, err
		}
		switch statusResp.GetStatus() {
		case "success", "failed":
			opCtx, cancel = OpContext(ctx, 8*time.Second)
			logResp, logErr := c.FetchBuildLog(opCtx, buildID)
			cancel()
			if logErr != nil {
				return statusResp, nil, logErr
			}
			return statusResp, logResp, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, nil, fmt.Errorf("build %q did not reach terminal state within %s", buildID, timeout)
}

func ExpectStatusCode(err error, code codes.Code) error {
	if err == nil {
		return fmt.Errorf("expected gRPC status %s, got nil", code)
	}
	if got := status.Code(err); got != code {
		return fmt.Errorf("expected gRPC status %s, got %s: %w", code, got, err)
	}
	return nil
}

func ExpectConflict(conflicted bool, conflictPath string, label string) error {
	if !conflicted {
		return fmt.Errorf("%s should report conflicted=true", label)
	}
	if strings.TrimSpace(conflictPath) == "" {
		return fmt.Errorf("%s should return conflict_path", label)
	}
	return nil
}

func IsAuthDisabled(rt Runtime) bool {
	return strings.TrimSpace(rt.SharedSecret) == ""
}

func BuildCapabilitySet(rt Runtime) []string {
	capabilities := []string{"sync", "admin"}
	if rt.RemoteBuildEnabled {
		capabilities = append(capabilities, "build")
	}
	return capabilities
}

func ExpectStringContains(value, needle, label string) error {
	if !strings.Contains(value, needle) {
		return fmt.Errorf("%s should contain %q, got %q", label, needle, value)
	}
	return nil
}

func JoinRunPath(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return pathpkg.Join(filtered...)
}

func EnsureNonEmpty(value, label string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", label)
	}
	return nil
}

func IgnoreNotExist(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
