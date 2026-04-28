//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/svc"

	"roodox_server/internal/appconfig"
	"roodox_server/internal/serverapp"
)

func runServer(cfg appconfig.Config, serviceName string) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("detect windows service mode failed: %w", err)
	}
	if !isService {
		return runForegroundServer(cfg)
	}

	restoreLog, err := installWindowsServiceLog(cfg)
	if err != nil {
		return fmt.Errorf("install windows service log failed: %w", err)
	}
	defer restoreLog()

	log.Printf("component=service op=start service_name=%q", serviceName)
	return serverapp.RunWindowsService(serviceName, cfg)
}

func installWindowsServiceLog(cfg appconfig.Config) (func(), error) {
	logDir := strings.TrimSpace(cfg.Runtime.LogDir)
	if logDir == "" {
		logDir = "runtime/logs"
	}
	if !filepath.IsAbs(logDir) {
		if cwd, err := os.Getwd(); err == nil {
			logDir = filepath.Join(cwd, logDir)
		}
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}

	logName := strings.TrimSpace(cfg.Runtime.StderrLogName)
	if logName == "" {
		logName = "server.stderr.log"
	}
	logPath := filepath.Join(logDir, logName)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	original := log.Writer()
	log.SetOutput(file)

	return func() {
		log.SetOutput(original)
		_ = file.Close()
	}, nil
}
