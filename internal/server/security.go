package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const sharedSecretHeader = "x-roodox-secret"

type SecurityConfig struct {
	AuthEnabled  bool
	SharedSecret string
	TLSEnabled   bool
	TLSCertPath  string
	TLSKeyPath   string
	Metrics      RuntimeMetrics
}

type TLSArtifactsStatus struct {
	CertPath            string   `json:"cert_path"`
	KeyPath             string   `json:"key_path"`
	RootCertPath        string   `json:"root_cert_path"`
	RootKeyPath         string   `json:"root_key_path"`
	ServerCertExists    bool     `json:"server_cert_exists"`
	ServerKeyExists     bool     `json:"server_key_exists"`
	RootCertExists      bool     `json:"root_cert_exists"`
	RootKeyExists       bool     `json:"root_key_exists"`
	ServerSubject       string   `json:"server_subject"`
	RootSubject         string   `json:"root_subject"`
	ServerDNSNames      []string `json:"server_dns_names"`
	ServerNotBeforeUnix int64    `json:"server_not_before_unix"`
	ServerNotAfterUnix  int64    `json:"server_not_after_unix"`
	RootNotBeforeUnix   int64    `json:"root_not_before_unix"`
	RootNotAfterUnix    int64    `json:"root_not_after_unix"`
	RootIsCA            bool     `json:"root_is_ca"`
	ServerValid         bool     `json:"server_valid"`
	RootValid           bool     `json:"root_valid"`
	OverallValid        bool     `json:"overall_valid"`
}

type TLSRotateOptions struct {
	RotateRootCA bool
	BackupDir    string
}

type TLSRotateResult struct {
	RotatedRootCA bool               `json:"rotated_root_ca"`
	BackupDir     string             `json:"backup_dir"`
	Status        TLSArtifactsStatus `json:"status"`
}

const (
	defaultTLSRootCertFileName = "roodox-ca-cert.pem"
	defaultTLSRootKeyFileName  = "roodox-ca-key.pem"
)

func BuildGRPCServerOptions(cfg SecurityConfig) ([]grpc.ServerOption, error) {
	opts := make([]grpc.ServerOption, 0, 3)
	unaryInterceptors := []grpc.UnaryServerInterceptor{loggingUnaryInterceptor(cfg.Metrics)}
	streamInterceptors := []grpc.StreamServerInterceptor{loggingStreamInterceptor(cfg.Metrics)}

	if cfg.AuthEnabled {
		secret := strings.TrimSpace(cfg.SharedSecret)
		if secret == "" {
			return nil, errors.New("auth is enabled but shared_secret is empty")
		}
		unaryInterceptors = append(unaryInterceptors, authUnaryInterceptor(secret))
		streamInterceptors = append(streamInterceptors, authStreamInterceptor(secret))
	}

	opts = append(opts,
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	if cfg.TLSEnabled {
		creds, err := loadServerTLSCredentials(cfg.TLSCertPath, cfg.TLSKeyPath)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(creds))
	}

	return opts, nil
}

func authUnaryInterceptor(expected string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := validateSharedSecret(ctx, expected); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func authStreamInterceptor(expected string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := validateSharedSecret(ss.Context(), expected); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func validateSharedSecret(ctx context.Context, expected string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		LogRequestEvent(ctx, "component=auth secret_check_result=missing_metadata")
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get(sharedSecretHeader)
	if len(values) == 0 {
		LogRequestEvent(ctx, "component=auth secret_check_result=missing_secret")
		return status.Error(codes.Unauthenticated, "missing shared secret")
	}
	if subtle.ConstantTimeCompare([]byte(values[0]), []byte(expected)) != 1 {
		LogRequestEvent(ctx, "component=auth secret_check_result=invalid")
		return status.Error(codes.Unauthenticated, "invalid shared secret")
	}
	LogRequestEvent(ctx, "component=auth secret_check_result=ok")
	return nil
}

func loadServerTLSCredentials(certPath, keyPath string) (credentials.TransportCredentials, error) {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" || keyPath == "" {
		return nil, errors.New("tls is enabled but cert/key path is empty")
	}
	if err := ensureServerCertificate(certPath, keyPath); err != nil {
		return nil, err
	}
	creds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load tls certificate failed: %w", err)
	}
	return creds, nil
}

func ensureServerCertificate(certPath, keyPath string) error {
	rootCertPath := tlsRootCertPath(certPath)
	rootKeyPath := tlsRootKeyPath(certPath)

	valid, err := hasValidTLSArtifacts(certPath, keyPath, rootCertPath)
	if err != nil {
		return err
	}
	if valid {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return fmt.Errorf("create cert dir failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		return fmt.Errorf("create key dir failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(rootCertPath), 0o755); err != nil {
		return fmt.Errorf("create root cert dir failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(rootKeyPath), 0o755); err != nil {
		return fmt.Errorf("create root key dir failed: %w", err)
	}

	rootCert, rootKey, err := loadOrCreateRootCA(rootCertPath, rootKeyPath)
	if err != nil {
		return err
	}

	serverCertPEM, serverKeyPEM, err := generateServerCertificate(rootCert, rootKey)
	if err != nil {
		return err
	}

	if err := os.WriteFile(certPath, serverCertPEM, 0o644); err != nil {
		return fmt.Errorf("write tls certificate failed: %w", err)
	}
	if err := os.WriteFile(keyPath, serverKeyPEM, 0o600); err != nil {
		return fmt.Errorf("write tls key failed: %w", err)
	}

	return nil
}

func loadOrCreateRootCA(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	if fileExists(certPath) && fileExists(keyPath) {
		cert, err := readCertificateFile(certPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read tls root certificate failed: %w", err)
		}
		key, err := readRSAPrivateKeyFile(keyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read tls root key failed: %w", err)
		}
		if cert.IsCA {
			return cert, key, nil
		}
	}

	rootCertPEM, rootKeyPEM, err := generateRootCA()
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(certPath, rootCertPEM, 0o644); err != nil {
		return nil, nil, fmt.Errorf("write tls root certificate failed: %w", err)
	}
	if err := os.WriteFile(keyPath, rootKeyPEM, 0o600); err != nil {
		return nil, nil, fmt.Errorf("write tls root key failed: %w", err)
	}

	cert, err := parsePEMCertificate(rootCertPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("parse generated tls root certificate failed: %w", err)
	}
	key, err := parsePEMRSAPrivateKey(rootKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("parse generated tls root key failed: %w", err)
	}
	return cert, key, nil
}

func generateRootCA() ([]byte, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate tls root key failed: %w", err)
	}

	template, err := rootCATemplate()
	if err != nil {
		return nil, nil, err
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create tls root certificate failed: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	return certPEM, keyPEM, nil
}

func generateServerCertificate(rootCert *x509.Certificate, rootKey *rsa.PrivateKey) ([]byte, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate tls key failed: %w", err)
	}

	template, err := serverCertificateTemplate()
	if err != nil {
		return nil, nil, err
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, rootCert, &privateKey.PublicKey, rootKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create tls certificate failed: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	return certPEM, keyPEM, nil
}

func rootCATemplate() (*x509.Certificate, error) {
	serialNumber, err := newCertificateSerialNumber()
	if err != nil {
		return nil, fmt.Errorf("generate tls root serial failed: %w", err)
	}

	return &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "Roodox Root CA",
			Organization: []string{"Roodox"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}, nil
}

func serverCertificateTemplate() (*x509.Certificate, error) {
	serialNumber, err := newCertificateSerialNumber()
	if err != nil {
		return nil, fmt.Errorf("generate tls serial failed: %w", err)
	}

	dnsNames := []string{"localhost"}
	if host, err := os.Hostname(); err == nil {
		host = strings.TrimSpace(host)
		if host != "" && !containsString(dnsNames, host) {
			dnsNames = append(dnsNames, host)
		}
	}

	return &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "Roodox Server",
			Organization: []string{"Roodox"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(3, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
	}, nil
}

func newCertificateSerialNumber() (*big.Int, error) {
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialLimit)
}

func hasValidTLSArtifacts(certPath, keyPath, rootCertPath string) (bool, error) {
	if !fileExists(certPath) || !fileExists(keyPath) || !fileExists(rootCertPath) {
		return false, nil
	}

	serverCert, err := readCertificateFile(certPath)
	if err != nil {
		return false, fmt.Errorf("read existing tls certificate failed: %w", err)
	}
	rootCert, err := readCertificateFile(rootCertPath)
	if err != nil {
		return false, fmt.Errorf("read existing tls root certificate failed: %w", err)
	}
	serverKey, err := readRSAPrivateKeyFile(keyPath)
	if err != nil {
		return false, fmt.Errorf("read existing tls key failed: %w", err)
	}

	if rootCert == nil || !rootCert.IsCA {
		return false, nil
	}
	if serverCert == nil || serverCert.IsCA {
		return false, nil
	}
	if err := verifyPrivateKeyMatchesCertificate(serverKey, serverCert); err != nil {
		return false, nil
	}

	roots := x509.NewCertPool()
	roots.AddCert(rootCert)
	if _, err := serverCert.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		return false, nil
	}
	return true, nil
}

func readCertificateFile(path string) (*x509.Certificate, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePEMCertificate(pemData)
}

func parsePEMCertificate(pemData []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("missing pem certificate block")
	}
	return x509.ParseCertificate(block.Bytes)
}

func readRSAPrivateKeyFile(path string) (*rsa.PrivateKey, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePEMRSAPrivateKey(pemData)
}

func parsePEMRSAPrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("missing pem private key block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not rsa")
	}
	return rsaKey, nil
}

func verifyPrivateKeyMatchesCertificate(privateKey *rsa.PrivateKey, cert *x509.Certificate) error {
	certKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("certificate public key is not rsa")
	}
	if certKey.N.Cmp(privateKey.N) != 0 || certKey.E != privateKey.E {
		return errors.New("certificate and private key do not match")
	}
	return nil
}

func tlsRootCertPath(certPath string) string {
	return filepath.Join(filepath.Dir(certPath), defaultTLSRootCertFileName)
}

func tlsRootKeyPath(certPath string) string {
	return filepath.Join(filepath.Dir(certPath), defaultTLSRootKeyFileName)
}

func tlsRootCertPathForClient(certPath string) string {
	return tlsRootCertPath(certPath)
}

func InspectTLSArtifacts(certPath, keyPath string) (TLSArtifactsStatus, error) {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" || keyPath == "" {
		return TLSArtifactsStatus{}, errors.New("tls cert/key path is empty")
	}

	status := TLSArtifactsStatus{
		CertPath:         certPath,
		KeyPath:          keyPath,
		RootCertPath:     tlsRootCertPath(certPath),
		RootKeyPath:      tlsRootKeyPath(certPath),
		ServerCertExists: fileExists(certPath),
		ServerKeyExists:  fileExists(keyPath),
		RootCertExists:   fileExists(tlsRootCertPath(certPath)),
		RootKeyExists:    fileExists(tlsRootKeyPath(certPath)),
		ServerDNSNames:   []string{},
	}

	var (
		serverCert *x509.Certificate
		rootCert   *x509.Certificate
		serverKey  *rsa.PrivateKey
		errs       []string
	)

	if status.ServerCertExists {
		cert, err := readCertificateFile(certPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("read server cert: %v", err))
		} else {
			serverCert = cert
			status.ServerSubject = cert.Subject.String()
			status.ServerDNSNames = append([]string(nil), cert.DNSNames...)
			status.ServerNotBeforeUnix = cert.NotBefore.Unix()
			status.ServerNotAfterUnix = cert.NotAfter.Unix()
		}
	}
	if status.RootCertExists {
		cert, err := readCertificateFile(status.RootCertPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("read root cert: %v", err))
		} else {
			rootCert = cert
			status.RootSubject = cert.Subject.String()
			status.RootNotBeforeUnix = cert.NotBefore.Unix()
			status.RootNotAfterUnix = cert.NotAfter.Unix()
			status.RootIsCA = cert.IsCA
			status.RootValid = cert.IsCA
		}
	}
	if status.ServerKeyExists {
		key, err := readRSAPrivateKeyFile(keyPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("read server key: %v", err))
		} else {
			serverKey = key
		}
	}

	if serverCert != nil && serverKey != nil && rootCert != nil && rootCert.IsCA {
		roots := x509.NewCertPool()
		roots.AddCert(rootCert)
		if err := verifyPrivateKeyMatchesCertificate(serverKey, serverCert); err == nil && !serverCert.IsCA {
			if _, err := serverCert.Verify(x509.VerifyOptions{
				Roots:     roots,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			}); err == nil {
				status.ServerValid = true
			} else {
				errs = append(errs, fmt.Sprintf("verify server cert: %v", err))
			}
		} else if err != nil {
			errs = append(errs, fmt.Sprintf("match server cert/key: %v", err))
		}
	}

	status.OverallValid = status.ServerValid && status.RootValid && status.ServerKeyExists
	if len(errs) > 0 {
		return status, errors.New(strings.Join(errs, "; "))
	}
	return status, nil
}

func RotateTLSArtifacts(certPath, keyPath string, options TLSRotateOptions) (TLSRotateResult, error) {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" || keyPath == "" {
		return TLSRotateResult{}, errors.New("tls cert/key path is empty")
	}

	rootCertPath := tlsRootCertPath(certPath)
	rootKeyPath := tlsRootKeyPath(certPath)
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return TLSRotateResult{}, fmt.Errorf("create cert dir failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		return TLSRotateResult{}, fmt.Errorf("create key dir failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(rootCertPath), 0o755); err != nil {
		return TLSRotateResult{}, fmt.Errorf("create root cert dir failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(rootKeyPath), 0o755); err != nil {
		return TLSRotateResult{}, fmt.Errorf("create root key dir failed: %w", err)
	}

	backupDir, err := backupTLSArtifacts(certPath, keyPath, rootCertPath, rootKeyPath, options.BackupDir)
	if err != nil {
		return TLSRotateResult{}, err
	}

	var (
		rootCert    *x509.Certificate
		rootKey     *rsa.PrivateKey
		rootCertPEM []byte
		rootKeyPEM  []byte
	)
	if options.RotateRootCA {
		rootCertPEM, rootKeyPEM, err = generateRootCA()
		if err != nil {
			return TLSRotateResult{}, err
		}
		rootCert, err = parsePEMCertificate(rootCertPEM)
		if err != nil {
			return TLSRotateResult{}, fmt.Errorf("parse generated tls root certificate failed: %w", err)
		}
		rootKey, err = parsePEMRSAPrivateKey(rootKeyPEM)
		if err != nil {
			return TLSRotateResult{}, fmt.Errorf("parse generated tls root key failed: %w", err)
		}
	} else {
		if !fileExists(rootCertPath) || !fileExists(rootKeyPath) {
			return TLSRotateResult{}, errors.New("existing root ca artifacts are missing; rotate root ca instead")
		}
		rootCert, err = readCertificateFile(rootCertPath)
		if err != nil {
			return TLSRotateResult{}, fmt.Errorf("read tls root certificate failed: %w", err)
		}
		rootKey, err = readRSAPrivateKeyFile(rootKeyPath)
		if err != nil {
			return TLSRotateResult{}, fmt.Errorf("read tls root key failed: %w", err)
		}
		if !rootCert.IsCA {
			return TLSRotateResult{}, errors.New("existing root certificate is not a ca; rotate root ca instead")
		}
	}

	serverCertPEM, serverKeyPEM, err := generateServerCertificate(rootCert, rootKey)
	if err != nil {
		return TLSRotateResult{}, err
	}

	if options.RotateRootCA {
		if err := writeFileAtomically(rootCertPath, rootCertPEM, 0o644); err != nil {
			return TLSRotateResult{}, fmt.Errorf("write tls root certificate failed: %w", err)
		}
		if err := writeFileAtomically(rootKeyPath, rootKeyPEM, 0o600); err != nil {
			return TLSRotateResult{}, fmt.Errorf("write tls root key failed: %w", err)
		}
	}
	if err := writeFileAtomically(certPath, serverCertPEM, 0o644); err != nil {
		return TLSRotateResult{}, fmt.Errorf("write tls certificate failed: %w", err)
	}
	if err := writeFileAtomically(keyPath, serverKeyPEM, 0o600); err != nil {
		return TLSRotateResult{}, fmt.Errorf("write tls key failed: %w", err)
	}

	status, err := InspectTLSArtifacts(certPath, keyPath)
	if err != nil {
		return TLSRotateResult{}, err
	}
	return TLSRotateResult{
		RotatedRootCA: options.RotateRootCA,
		BackupDir:     backupDir,
		Status:        status,
	}, nil
}

func ExportTLSRootCertificate(certPath, destPath string) error {
	certPath = strings.TrimSpace(certPath)
	destPath = strings.TrimSpace(destPath)
	if certPath == "" || destPath == "" {
		return errors.New("tls cert path and destination path are required")
	}

	rootCertPath := tlsRootCertPath(certPath)
	rootPEM, err := os.ReadFile(rootCertPath)
	if err != nil {
		return fmt.Errorf("read tls root certificate failed: %w", err)
	}
	return writeFileAtomically(destPath, rootPEM, 0o644)
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func backupTLSArtifacts(certPath, keyPath, rootCertPath, rootKeyPath, backupDir string) (string, error) {
	paths := []string{certPath, keyPath, rootCertPath, rootKeyPath}
	hasAny := false
	for _, path := range paths {
		if fileExists(path) {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return "", nil
	}

	backupDir = strings.TrimSpace(backupDir)
	if backupDir == "" {
		backupDir = filepath.Join(filepath.Dir(certPath), "archive", "tls-rotation-"+time.Now().Format("20060102-150405"))
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	for _, path := range paths {
		if !fileExists(path) {
			continue
		}
		if err := copySmallFile(path, filepath.Join(backupDir, filepath.Base(path))); err != nil {
			return "", err
		}
	}
	return backupDir, nil
}

func copySmallFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return writeFileAtomically(dst, data, info.Mode().Perm())
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tempPath := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+fmt.Sprintf(".tmp-%d", time.Now().UnixNano()))
	if err := os.WriteFile(tempPath, data, perm); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}
