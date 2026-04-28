//go:build !windows

package main

import "roodox_server/internal/appconfig"

func runServer(cfg appconfig.Config, _ string) error {
	return runForegroundServer(cfg)
}
