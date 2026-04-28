package qasuite

import (
	"context"
	"fmt"
	pathpkg "path"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"roodox_server/client"
	pb "roodox_server/proto"
)

type FaultOptions struct {
	KeepArtifacts bool
}

func RunFaults(ctx context.Context, rt Runtime, opts FaultOptions) error {
	runRoot := BuildRunRelRoot("faults")
	filePath := pathpkg.Join(runRoot, "conflict.txt")
	missingBuildUnit := pathpkg.Join(runRoot, "missing-build-unit")

	if !opts.KeepArtifacts {
		defer func() { _ = RemoveRunRoot(rt.RootDir, runRoot) }()
	}

	fmt.Printf("[faults] dial=%s tls=%t auth=%t\n", rt.DialAddr, rt.TLSEnabled, !IsAuthDisabled(rt))

	if !IsAuthDisabled(rt) {
		if err := expectHealthFailure(client.ConnectionOptions{
			SharedSecret:    "wrong-secret",
			TLSEnabled:      rt.TLSEnabled,
			TLSRootCertPath: rt.TLSRootCertPath,
			TLSServerName:   rt.TLSServerName,
		}, rt.DialAddr, codes.Unauthenticated, "invalid shared secret"); err != nil {
			return err
		}
	}

	if rt.TLSEnabled {
		if err := expectHealthErrorContains(client.ConnectionOptions{
			SharedSecret:    rt.SharedSecret,
			TLSEnabled:      true,
			TLSRootCertPath: rt.TLSRootCertPath,
			TLSServerName:   "definitely-wrong-name",
		}, rt.DialAddr, "failed to verify certificate"); err != nil {
			return err
		}
		if err := expectClientCreateError(client.ConnectionOptions{
			SharedSecret:    rt.SharedSecret,
			TLSEnabled:      true,
			TLSRootCertPath: resolveMaybeRelative(rt.ConfigDir, "certs/roodox-server-key.pem"),
			TLSServerName:   rt.TLSServerName,
		}, rt.DialAddr, "append tls root certificate failed"); err != nil {
			return err
		}
	}

	c, err := rt.Dial()
	if err != nil {
		return err
	}
	defer c.Close()

	opCtx, cancel := OpContext(ctx, 8*time.Second)
	firstWrite, err := c.WriteFile(opCtx, filePath, []byte("v1"), 0)
	cancel()
	if err != nil {
		return fmt.Errorf("initial WriteFile failed: %w", err)
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	secondWrite, err := c.WriteFile(opCtx, filePath, []byte("v2"), firstWrite.GetNewVersion())
	cancel()
	if err != nil {
		return fmt.Errorf("second WriteFile failed: %w", err)
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	staleWrite, err := c.WriteFile(opCtx, filePath, []byte("stale"), firstWrite.GetNewVersion())
	cancel()
	if err != nil {
		return fmt.Errorf("stale WriteFile RPC failed: %w", err)
	}
	if err := ExpectConflict(staleWrite.GetConflicted(), staleWrite.GetConflictPath(), "stale WriteFile"); err != nil {
		return err
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	staleRange, err := c.WriteFileRangeWithVersion(opCtx, filePath, 0, []byte("xx"), firstWrite.GetNewVersion())
	cancel()
	if err != nil {
		return fmt.Errorf("stale WriteFileRange RPC failed: %w", err)
	}
	if err := ExpectConflict(staleRange.GetConflicted(), staleRange.GetConflictPath(), "stale WriteFileRange"); err != nil {
		return err
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	staleResize, err := c.SetFileSize(opCtx, filePath, 1, firstWrite.GetNewVersion())
	cancel()
	if err != nil {
		return fmt.Errorf("stale SetFileSize RPC failed: %w", err)
	}
	if err := ExpectConflict(staleResize.GetConflicted(), staleResize.GetConflictPath(), "stale SetFileSize"); err != nil {
		return err
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	buildResp, err := c.StartBuild(opCtx, missingBuildUnit, "smoke")
	cancel()
	if err != nil {
		if err := ExpectStringContains(err.Error(), "unsupported build unit", "StartBuild immediate error"); err != nil {
			return err
		}
	} else {
		statusResp, _, err := WaitBuildTerminal(ctx, c, buildResp.GetBuildId(), 15*time.Second)
		if err != nil {
			return fmt.Errorf("missing-build-unit terminal wait failed: %w", err)
		}
		if statusResp.GetStatus() != "failed" {
			return fmt.Errorf("missing-build-unit status=%q, want failed", statusResp.GetStatus())
		}
		if err := ExpectStringContains(statusResp.GetError(), "unsupported build unit", "missing-build-unit error"); err != nil {
			return err
		}
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	_, err = c.Heartbeat(opCtx, &pb.HeartbeatRequest{
		DeviceId:      "unknown-device",
		SessionId:     "unknown-session",
		TimestampUnix: time.Now().Unix(),
	})
	cancel()
	if err := ExpectStatusCode(err, codes.NotFound); err != nil {
		return fmt.Errorf("Heartbeat(unknown-device) validation failed: %w", err)
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	_, err = c.GetAssignedConfig(opCtx, "unknown-device")
	cancel()
	if err := ExpectStatusCode(err, codes.NotFound); err != nil {
		return fmt.Errorf("GetAssignedConfig(unknown-device) validation failed: %w", err)
	}

	fmt.Printf("[faults] ok first_version=%d second_version=%d stale_write=%s stale_range=%s stale_resize=%s\n",
		firstWrite.GetNewVersion(),
		secondWrite.GetNewVersion(),
		staleWrite.GetConflictPath(),
		staleRange.GetConflictPath(),
		staleResize.GetConflictPath(),
	)
	return nil
}

func expectHealthFailure(opts client.ConnectionOptions, addr string, code codes.Code, contains string) error {
	return expectHealthError(addr, opts, func(err error) error {
		if err := ExpectStatusCode(err, code); err != nil {
			return err
		}
		if contains != "" && !strings.Contains(err.Error(), contains) {
			return fmt.Errorf("health error should contain %q, got %q", contains, err.Error())
		}
		return nil
	})
}

func expectHealthErrorContains(opts client.ConnectionOptions, addr, contains string) error {
	return expectHealthError(addr, opts, func(err error) error {
		if err == nil {
			return fmt.Errorf("expected health check to fail")
		}
		if !strings.Contains(err.Error(), contains) {
			return fmt.Errorf("health error should contain %q, got %q", contains, err.Error())
		}
		return nil
	})
}

func expectClientCreateError(opts client.ConnectionOptions, addr, contains string) error {
	c, err := client.NewRoodoxClientWithOptions(addr, opts)
	if err == nil {
		_ = c.Close()
		return fmt.Errorf("expected client creation to fail")
	}
	if !strings.Contains(err.Error(), contains) {
		return fmt.Errorf("client creation error should contain %q, got %q", contains, err.Error())
	}
	return nil
}

func expectHealthError(addr string, opts client.ConnectionOptions, validate func(error) error) error {
	c, err := client.NewRoodoxClientWithOptions(addr, opts)
	if err != nil {
		return validate(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = c.HealthCheck(ctx)
	return validate(err)
}

func StatusSummary(err error) string {
	if err == nil {
		return "OK"
	}
	return fmt.Sprintf("%s: %s", status.Code(err), err.Error())
}
