package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"roodox_server/internal/db"
	"roodox_server/internal/fs"
	pb "roodox_server/proto"
)

func TestBuildGRPCServerOptionsRejectsEmptySharedSecret(t *testing.T) {
	_, err := BuildGRPCServerOptions(SecurityConfig{AuthEnabled: true})
	if err == nil {
		t.Fatal("BuildGRPCServerOptions unexpectedly succeeded without shared secret")
	}
}

func TestValidateSharedSecret(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(sharedSecretHeader, "secret-123"))
	if err := validateSharedSecret(ctx, "secret-123"); err != nil {
		t.Fatalf("validateSharedSecret returned error: %v", err)
	}
}

func TestValidateSharedSecretRejectsMissingSecret(t *testing.T) {
	err := validateSharedSecret(context.Background(), "secret-123")
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("validateSharedSecret code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestValidateSharedSecretRejectsInvalidSecret(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(sharedSecretHeader, "bad-secret"))
	err := validateSharedSecret(ctx, "secret-123")
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("validateSharedSecret code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestBuildGRPCServerOptionsGeneratesTLSFiles(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server-cert.pem")
	keyPath := filepath.Join(dir, "server-key.pem")
	rootCertPath := tlsRootCertPathForClient(certPath)

	opts, err := BuildGRPCServerOptions(SecurityConfig{
		TLSEnabled:  true,
		TLSCertPath: certPath,
		TLSKeyPath:  keyPath,
	})
	if err != nil {
		t.Fatalf("BuildGRPCServerOptions returned error: %v", err)
	}
	if len(opts) == 0 {
		t.Fatal("BuildGRPCServerOptions returned no server options")
	}
	if !fileExists(certPath) {
		t.Fatalf("expected cert file %q to exist", certPath)
	}
	if !fileExists(keyPath) {
		t.Fatalf("expected key file %q to exist", keyPath)
	}
	if !fileExists(rootCertPath) {
		t.Fatalf("expected root cert file %q to exist", rootCertPath)
	}

	serverCert := mustReadCertificate(t, certPath)
	rootCert := mustReadCertificate(t, rootCertPath)
	if rootCert.IsCA != true {
		t.Fatal("expected generated root certificate to have CA:TRUE")
	}
	if serverCert.IsCA {
		t.Fatal("expected generated server certificate to have CA:FALSE")
	}

	roots := x509.NewCertPool()
	roots.AddCert(rootCert)
	if _, err := serverCert.Verify(x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "localhost",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		t.Fatalf("server certificate did not verify against generated root certificate: %v", err)
	}
}

func TestEnsureServerCertificateMigratesLegacySelfSignedLeaf(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server-cert.pem")
	keyPath := filepath.Join(dir, "server-key.pem")
	rootCertPath := tlsRootCertPathForClient(certPath)

	leafCertPEM, leafKeyPEM, err := generateLegacySelfSignedLeaf()
	if err != nil {
		t.Fatalf("generateLegacySelfSignedLeaf returned error: %v", err)
	}
	if err := os.WriteFile(certPath, leafCertPEM, 0o644); err != nil {
		t.Fatalf("WriteFile(certPath) returned error: %v", err)
	}
	if err := os.WriteFile(keyPath, leafKeyPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(keyPath) returned error: %v", err)
	}

	if err := ensureServerCertificate(certPath, keyPath); err != nil {
		t.Fatalf("ensureServerCertificate returned error: %v", err)
	}

	rootCert := mustReadCertificate(t, rootCertPath)
	serverCert := mustReadCertificate(t, certPath)
	if !rootCert.IsCA {
		t.Fatal("expected migrated root certificate to have CA:TRUE")
	}
	if serverCert.IsCA {
		t.Fatal("expected migrated server certificate to have CA:FALSE")
	}
}

func TestInspectTLSArtifacts(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server-cert.pem")
	keyPath := filepath.Join(dir, "server-key.pem")

	if err := ensureServerCertificate(certPath, keyPath); err != nil {
		t.Fatalf("ensureServerCertificate returned error: %v", err)
	}

	status, err := InspectTLSArtifacts(certPath, keyPath)
	if err != nil {
		t.Fatalf("InspectTLSArtifacts returned error: %v", err)
	}
	if !status.OverallValid {
		t.Fatalf("expected overall tls artifacts to be valid: %+v", status)
	}
	if !status.ServerValid || !status.RootValid {
		t.Fatalf("expected server/root artifacts to be valid: %+v", status)
	}
	if status.ServerNotAfterUnix <= time.Now().Unix() {
		t.Fatalf("server cert should not be expired: %+v", status)
	}
	if len(status.ServerDNSNames) == 0 {
		t.Fatalf("expected server dns names to be populated: %+v", status)
	}
}

func TestRotateTLSArtifactsRotatesLeafOnly(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server-cert.pem")
	keyPath := filepath.Join(dir, "server-key.pem")

	if err := ensureServerCertificate(certPath, keyPath); err != nil {
		t.Fatalf("ensureServerCertificate returned error: %v", err)
	}

	originalServer := mustReadCertificate(t, certPath)
	originalRoot := mustReadCertificate(t, tlsRootCertPathForClient(certPath))

	result, err := RotateTLSArtifacts(certPath, keyPath, TLSRotateOptions{})
	if err != nil {
		t.Fatalf("RotateTLSArtifacts returned error: %v", err)
	}
	if result.RotatedRootCA {
		t.Fatal("expected leaf-only rotation to keep existing root ca")
	}
	if result.BackupDir == "" {
		t.Fatal("expected backup dir to be created during rotation")
	}

	rotatedServer := mustReadCertificate(t, certPath)
	rotatedRoot := mustReadCertificate(t, tlsRootCertPathForClient(certPath))
	if originalRoot.SerialNumber.Cmp(rotatedRoot.SerialNumber) != 0 {
		t.Fatal("expected leaf-only rotation to preserve the root certificate")
	}
	if originalServer.SerialNumber.Cmp(rotatedServer.SerialNumber) == 0 {
		t.Fatal("expected leaf-only rotation to replace the server certificate")
	}
}

func TestExportTLSRootCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server-cert.pem")
	keyPath := filepath.Join(dir, "server-key.pem")
	exportPath := filepath.Join(dir, "exports", "client-ca.pem")

	if err := ensureServerCertificate(certPath, keyPath); err != nil {
		t.Fatalf("ensureServerCertificate returned error: %v", err)
	}
	if err := ExportTLSRootCertificate(certPath, exportPath); err != nil {
		t.Fatalf("ExportTLSRootCertificate returned error: %v", err)
	}

	rootPEM, err := os.ReadFile(tlsRootCertPathForClient(certPath))
	if err != nil {
		t.Fatalf("ReadFile(root cert) returned error: %v", err)
	}
	exportedPEM, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("ReadFile(export path) returned error: %v", err)
	}
	if string(rootPEM) != string(exportedPEM) {
		t.Fatal("expected exported ca file to match the server root certificate")
	}
}

func TestAuthInterceptorReturnsUnauthenticatedForMissingAndInvalidSecret(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "auth.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	opts, err := BuildGRPCServerOptions(SecurityConfig{
		AuthEnabled:  true,
		SharedSecret: "secret-123",
	})
	if err != nil {
		t.Fatalf("BuildGRPCServerOptions returned error: %v", err)
	}

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer(opts...)
	pb.RegisterCoreServiceServer(srv, NewCoreService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker()))
	go func() {
		_ = srv.Serve(lis)
	}()
	defer srv.Stop()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("grpc.DialContext returned error: %v", err)
	}
	defer conn.Close()

	client := pb.NewCoreServiceClient(conn)

	_, err = client.ListDir(context.Background(), &pb.ListDirRequest{Path: "."})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing secret code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}

	wrongCtx := metadata.AppendToOutgoingContext(context.Background(), sharedSecretHeader, "bad-secret")
	_, err = client.ListDir(wrongCtx, &pb.ListDirRequest{Path: "."})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("invalid secret code = %v, want %v", status.Code(err), codes.Unauthenticated)
	}

	okCtx := metadata.AppendToOutgoingContext(context.Background(), sharedSecretHeader, "secret-123")
	_, err = client.ListDir(okCtx, &pb.ListDirRequest{Path: "."})
	if err != nil {
		t.Fatalf("valid secret returned error: %v", err)
	}
}

func mustReadCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	cert, err := readCertificateFile(path)
	if err != nil {
		t.Fatalf("readCertificateFile(%q) returned error: %v", path, err)
	}
	return cert
}

func generateLegacySelfSignedLeaf() ([]byte, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	template, err := serverCertificateTemplate()
	if err != nil {
		return nil, nil, err
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	return certPEM, keyPEM, nil
}
