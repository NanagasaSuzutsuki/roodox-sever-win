//go:build !windows

package serverapp

import (
	"fmt"

	"roodox_server/internal/appconfig"
)

func RunWindowsService(_ string, _ appconfig.Config) error {
	return fmt.Errorf("windows service mode is only supported on Windows")
}
