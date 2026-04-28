package server

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	envBuildToolDirs      = "ROODOX_BUILD_TOOL_DIRS"
	envRequiredBuildTools = "ROODOX_BUILD_REQUIRED_TOOLS"
)

// StartupCheckResult keeps the validated runtime context for remote build.
type StartupCheckResult struct {
	CurrentUser string
	Effective   map[string]string
}

// RunStartupChecks validates root directory and remote build prerequisites.
func RunStartupChecks(rootDir string, remoteBuildEnabled bool) (*StartupCheckResult, error) {
	if err := ensureRootDirWritable(rootDir); err != nil {
		return nil, fmt.Errorf("startup check failed: root dir is not writable: %w", err)
	}

	cur, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("startup check failed: resolve current user: %w", err)
	}

	result := &StartupCheckResult{
		CurrentUser: normalizeUser(cur.Username),
		Effective:   map[string]string{},
	}

	if !remoteBuildEnabled {
		return result, nil
	}

	if err := ensureBuildToolPath(); err != nil {
		return nil, err
	}
	if err := verifySameUser(result.CurrentUser); err != nil {
		return nil, err
	}

	requiredTools := requiredBuildTools()
	for _, tool := range requiredTools {
		if tool == "build-essential" {
			if err := verifyBuildEssential(); err != nil {
				return nil, err
			}
			result.Effective[tool] = "ok"
			continue
		}

		path, err := exec.LookPath(tool)
		if err != nil {
			return nil, fmt.Errorf("startup check failed: required tool %q not found in PATH", tool)
		}
		result.Effective[tool] = path
	}
	return result, nil
}

func ensureRootDirWritable(rootDir string) error {
	if rootDir == "" {
		return errors.New("root dir is empty")
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return err
	}

	f, err := os.CreateTemp(rootDir, ".roodox-write-check-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := f.Close(); err != nil {
		return err
	}
	return os.Remove(tmp)
}

func ensureBuildToolPath() error {
	raw := strings.TrimSpace(os.Getenv(envBuildToolDirs))
	if raw == "" {
		return nil
	}

	current := os.Getenv("PATH")
	seen := map[string]struct{}{}
	for _, p := range filepath.SplitList(current) {
		seen[strings.ToLower(filepath.Clean(strings.TrimSpace(p)))] = struct{}{}
	}

	merged := filepath.SplitList(current)
	for _, dir := range filepath.SplitList(raw) {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			continue
		}
		absDir, err := filepath.Abs(trimmed)
		if err != nil {
			return fmt.Errorf("startup check failed: resolve %s entry %q: %w", envBuildToolDirs, trimmed, err)
		}
		if st, err := os.Stat(absDir); err != nil || !st.IsDir() {
			return fmt.Errorf("startup check failed: build tool directory %q is invalid", absDir)
		}
		key := strings.ToLower(filepath.Clean(absDir))
		if _, ok := seen[key]; ok {
			continue
		}
		merged = append([]string{absDir}, merged...)
		seen[key] = struct{}{}
	}
	return os.Setenv("PATH", strings.Join(merged, string(os.PathListSeparator)))
}

func requiredBuildTools() []string {
	raw := strings.TrimSpace(os.Getenv(envRequiredBuildTools))
	if raw != "" {
		return parseCommaList(raw)
	}
	return []string{"cmake", "make", "build-essential"}
}

func parseCommaList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		name := strings.TrimSpace(strings.ToLower(p))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func verifyBuildEssential() error {
	if runtime.GOOS != "linux" {
		// build-essential is a Debian package concept. On non-Linux we skip this check.
		return nil
	}

	if _, err := exec.LookPath("dpkg-query"); err == nil {
		out, cmdErr := exec.Command("dpkg-query", "-W", "-f=${Status}", "build-essential").CombinedOutput()
		if cmdErr == nil && strings.Contains(strings.ToLower(string(out)), "install ok installed") {
			return nil
		}
	}

	// Fallback check for non-Debian Linux distributions.
	for _, tool := range []string{"gcc", "g++", "make"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("startup check failed: %q not found; install build-essential/toolchain first", tool)
		}
	}
	return nil
}

func verifySameUser(expected string) error {
	name, err := detectExecUser()
	if err != nil {
		return fmt.Errorf("startup check failed: detect exec user: %w", err)
	}
	actual := normalizeUser(name)
	if !sameNormalizedUser(expected, actual, runtime.GOOS, os.Getenv("COMPUTERNAME")) {
		return fmt.Errorf("startup check failed: process user mismatch, expected=%q actual=%q", expected, actual)
	}
	return nil
}

func detectExecUser() (string, error) {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("whoami").CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	out, err := exec.Command("id", "-un").CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func normalizeUser(v string) string {
	name := strings.TrimSpace(strings.ToLower(v))
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

func sameNormalizedUser(expected string, actual string, goos string, computerName string) bool {
	if expected == actual {
		return true
	}
	if goos != "windows" {
		return false
	}
	return isWindowsLocalSystemIdentity(expected, computerName) && isWindowsLocalSystemIdentity(actual, computerName)
}

func isWindowsLocalSystemIdentity(userName string, computerName string) bool {
	name := normalizeUser(userName)
	switch name {
	case "system", "localsystem":
		return true
	}

	host := normalizeUser(computerName)
	return host != "" && name == host+"$"
}
