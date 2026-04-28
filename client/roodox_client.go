package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	pb "roodox_server/proto"
)

const sharedSecretHeader = "x-roodox-secret"

type ConnectionOptions struct {
	SharedSecret    string
	TLSEnabled      bool
	TLSRootCertPath string
	TLSServerName   string
}

type RoodoxClient struct {
	cc      *grpc.ClientConn
	core    pb.CoreServiceClient
	sync    pb.SyncServiceClient
	lock    pb.LockServiceClient
	build   pb.BuildServiceClient
	version pb.VersionServiceClient
	control pb.ControlPlaneServiceClient
	admin   pb.AdminConsoleServiceClient
	health  grpc_health_v1.HealthClient
}

func NewRoodoxClient(addr string) (*RoodoxClient, error) {
	return NewRoodoxClientWithOptions(addr, ConnectionOptions{})
}

func NewRoodoxClientWithOptions(addr string, opts ConnectionOptions) (*RoodoxClient, error) {
	dialOptions, err := buildDialOptions(opts)
	if err != nil {
		return nil, err
	}

	cc, err := grpc.Dial(addr, dialOptions...)
	if err != nil {
		return nil, err
	}
	return &RoodoxClient{
		cc:      cc,
		core:    pb.NewCoreServiceClient(cc),
		sync:    pb.NewSyncServiceClient(cc),
		lock:    pb.NewLockServiceClient(cc),
		build:   pb.NewBuildServiceClient(cc),
		version: pb.NewVersionServiceClient(cc),
		control: pb.NewControlPlaneServiceClient(cc),
		admin:   pb.NewAdminConsoleServiceClient(cc),
		health:  grpc_health_v1.NewHealthClient(cc),
	}, nil
}

func buildDialOptions(opts ConnectionOptions) ([]grpc.DialOption, error) {
	dialOptions := make([]grpc.DialOption, 0, 2)

	if opts.TLSEnabled {
		if opts.TLSRootCertPath == "" {
			return nil, errors.New("tls is enabled but tls_root_cert_path is empty")
		}
		creds, err := loadClientTLSCredentials(opts.TLSRootCertPath, opts.TLSServerName)
		if err != nil {
			return nil, err
		}
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))
	} else {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if opts.SharedSecret != "" {
		dialOptions = append(dialOptions, grpc.WithPerRPCCredentials(sharedSecretCredentials{secret: opts.SharedSecret}))
	}

	return dialOptions, nil
}

func loadClientTLSCredentials(rootCertPath, serverName string) (credentials.TransportCredentials, error) {
	pemData, err := os.ReadFile(rootCertPath)
	if err != nil {
		return nil, fmt.Errorf("read tls root certificate failed: %w", err)
	}

	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, errors.New("append tls root certificate failed")
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
	}
	if serverName != "" {
		tlsConfig.ServerName = serverName
	}

	return credentials.NewTLS(tlsConfig), nil
}

type sharedSecretCredentials struct {
	secret string
}

func (c sharedSecretCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{sharedSecretHeader: c.secret}, nil
}

func (c sharedSecretCredentials) RequireTransportSecurity() bool {
	return false
}

func (c *RoodoxClient) Close() error {
	return c.cc.Close()
}

func (c *RoodoxClient) ListDir(ctx context.Context, path string) ([]*pb.FileInfo, error) {
	resp, err := c.core.ListDir(ctx, &pb.ListDirRequest{Path: path})
	if err != nil {
		return nil, err
	}
	return resp.Entries, nil
}

func (c *RoodoxClient) Stat(ctx context.Context, path string) (*pb.FileInfo, error) {
	resp, err := c.core.Stat(ctx, &pb.StatRequest{Path: path})
	if err != nil {
		return nil, err
	}
	return resp.Info, nil
}

func (c *RoodoxClient) ReadFile(ctx context.Context, path string) ([]byte, error) {
	resp, err := c.core.ReadFile(ctx, &pb.ReadFileRequest{Path: path})
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (c *RoodoxClient) ReadFileRange(ctx context.Context, path string, offset, length int64) ([]byte, int64, error) {
	resp, err := c.core.ReadFileRange(ctx, &pb.ReadFileRangeRequest{Path: path, Offset: offset, Length: length})
	if err != nil {
		return nil, 0, err
	}
	return resp.Data, resp.FileSize, nil
}

func (c *RoodoxClient) WriteFileRange(ctx context.Context, path string, offset int64, data []byte) (*pb.WriteFileRangeResponse, error) {
	return c.WriteFileRangeWithVersion(ctx, path, offset, data, 0)
}

func (c *RoodoxClient) WriteFileRangeWithVersion(ctx context.Context, path string, offset int64, data []byte, baseVersion uint64) (*pb.WriteFileRangeResponse, error) {
	return c.core.WriteFileRange(ctx, &pb.WriteFileRangeRequest{
		Path:        path,
		Offset:      offset,
		Data:        data,
		BaseVersion: baseVersion,
	})
}

func (c *RoodoxClient) SetFileSize(ctx context.Context, path string, size int64, baseVersion uint64) (*pb.SetFileSizeResponse, error) {
	return c.core.SetFileSize(ctx, &pb.SetFileSizeRequest{
		Path:        path,
		Size:        size,
		BaseVersion: baseVersion,
	})
}

func (c *RoodoxClient) WriteFile(ctx context.Context, path string, data []byte, baseVersion uint64) (*pb.WriteFileResponse, error) {
	return c.sync.WriteFile(ctx, &pb.WriteFileRequest{Path: path, Data: data, BaseVersion: baseVersion})
}

func (c *RoodoxClient) AcquireLock(ctx context.Context, path, clientID string, ttl time.Duration) (*pb.AcquireLockResponse, error) {
	sec := uint32(ttl.Seconds())
	if sec == 0 {
		sec = 30
	}
	return c.lock.AcquireLock(ctx, &pb.AcquireLockRequest{Path: path, ClientId: clientID, TtlSeconds: sec})
}

func (c *RoodoxClient) RenewLock(ctx context.Context, path, clientID string, ttl time.Duration) (*pb.RenewLockResponse, error) {
	sec := uint32(ttl.Seconds())
	if sec == 0 {
		sec = 30
	}
	return c.lock.RenewLock(ctx, &pb.RenewLockRequest{Path: path, ClientId: clientID, TtlSeconds: sec})
}

func (c *RoodoxClient) ReleaseLock(ctx context.Context, path, clientID string) (*pb.ReleaseLockResponse, error) {
	return c.lock.ReleaseLock(ctx, &pb.ReleaseLockRequest{Path: path, ClientId: clientID})
}

func (c *RoodoxClient) StartBuild(ctx context.Context, unitPath, target string) (*pb.StartBuildResponse, error) {
	return c.build.StartBuild(ctx, &pb.StartBuildRequest{UnitPath: unitPath, Target: target})
}

func (c *RoodoxClient) GetBuildStatus(ctx context.Context, buildID string) (*pb.GetBuildStatusResponse, error) {
	return c.build.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{BuildId: buildID})
}

func (c *RoodoxClient) FetchBuildLog(ctx context.Context, buildID string) (*pb.FetchBuildLogResponse, error) {
	return c.build.FetchBuildLog(ctx, &pb.FetchBuildLogRequest{BuildId: buildID})
}

func (c *RoodoxClient) GetBuildProduct(ctx context.Context, buildID string) (*pb.GetBuildProductResponse, error) {
	return c.build.GetBuildProduct(ctx, &pb.GetBuildProductRequest{BuildId: buildID})
}

func (c *RoodoxClient) GetHistory(ctx context.Context, path string) ([]*pb.VersionRecord, error) {
	resp, err := c.version.GetHistory(ctx, &pb.GetHistoryRequest{Path: path})
	if err != nil {
		return nil, err
	}
	return resp.Records, nil
}

func (c *RoodoxClient) GetVersion(ctx context.Context, path string, version uint64) ([]byte, error) {
	resp, err := c.version.GetVersion(ctx, &pb.GetVersionRequest{Path: path, Version: version})
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (c *RoodoxClient) RegisterDevice(ctx context.Context, req *pb.RegisterDeviceRequest) (*pb.RegisterDeviceResponse, error) {
	return c.control.RegisterDevice(ctx, req)
}

func (c *RoodoxClient) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	return c.control.Heartbeat(ctx, req)
}

func (c *RoodoxClient) GetAssignedConfig(ctx context.Context, deviceID string) (*pb.GetAssignedConfigResponse, error) {
	return c.control.GetAssignedConfig(ctx, &pb.GetAssignedConfigRequest{DeviceId: deviceID})
}

func (c *RoodoxClient) ReportSyncState(ctx context.Context, req *pb.ReportSyncStateRequest) (*pb.ReportSyncStateResponse, error) {
	return c.control.ReportSyncState(ctx, req)
}

func (c *RoodoxClient) ListDevices(ctx context.Context) ([]*pb.DeviceSummary, error) {
	resp, err := c.admin.ListDevices(ctx, &pb.ListDevicesRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Devices, nil
}

func (c *RoodoxClient) GetServerRuntime(ctx context.Context) (*pb.GetServerRuntimeResponse, error) {
	return c.admin.GetServerRuntime(ctx, &pb.GetServerRuntimeRequest{})
}

func (c *RoodoxClient) GetServerObservability(ctx context.Context) (*pb.GetServerObservabilityResponse, error) {
	return c.admin.GetServerObservability(ctx, &pb.GetServerObservabilityRequest{})
}

func (c *RoodoxClient) TriggerServerBackup(ctx context.Context) (*pb.TriggerServerBackupResponse, error) {
	return c.admin.TriggerServerBackup(ctx, &pb.TriggerServerBackupRequest{})
}

func (c *RoodoxClient) ShutdownServer(ctx context.Context, reason string) (*pb.ShutdownServerResponse, error) {
	return c.admin.ShutdownServer(ctx, &pb.ShutdownServerRequest{Reason: reason})
}

func (c *RoodoxClient) HealthCheck(ctx context.Context) (*grpc_health_v1.HealthCheckResponse, error) {
	return c.health.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
}

func AppendSharedSecret(ctx context.Context, secret string) context.Context {
	if secret == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, sharedSecretHeader, secret)
}
