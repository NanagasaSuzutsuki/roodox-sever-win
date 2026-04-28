package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "roodox_server/proto"
)

func TestCopyDirSkipsIgnoredProjectArtifacts(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")

	if err := os.MkdirAll(filepath.Join(src, "project"), 0o755); err != nil {
		t.Fatalf("MkdirAll(project) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "project", "main.c"), []byte("int main() { return 0; }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.c) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "project", "main.c.roodox-conflict-20260322-120102.123456789-ab12cd"), []byte("conflict\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict) returned error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(src, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, ".git", "Makefile"), []byte("all:\n\t@echo git\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.git/Makefile) returned error: %v", err)
	}

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "project", "main.c")); err != nil {
		t.Fatalf("expected main.c to be copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "project", "main.c.roodox-conflict-20260322-120102.123456789-ab12cd")); !os.IsNotExist(err) {
		t.Fatalf("expected conflict file to be skipped, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); !os.IsNotExist(err) {
		t.Fatalf("expected .git to be skipped, stat err=%v", err)
	}
}

func TestResolveCMakeBuildTarget(t *testing.T) {
	targets := []string{"ALL_BUILD", "smoke", "ZERO_CHECK"}

	resolved, err := resolveCMakeBuildTarget("", targets)
	if err != nil {
		t.Fatalf("resolveCMakeBuildTarget(empty) returned error: %v", err)
	}
	if resolved != "" {
		t.Fatalf("resolveCMakeBuildTarget(empty) = %q, want empty", resolved)
	}

	resolved, err = resolveCMakeBuildTarget("all", targets)
	if err != nil {
		t.Fatalf("resolveCMakeBuildTarget(all) returned error: %v", err)
	}
	if resolved != "" {
		t.Fatalf("resolveCMakeBuildTarget(all) = %q, want empty", resolved)
	}

	resolved, err = resolveCMakeBuildTarget("SMOKE", targets)
	if err != nil {
		t.Fatalf("resolveCMakeBuildTarget(case-insensitive) returned error: %v", err)
	}
	if resolved != "smoke" {
		t.Fatalf("resolveCMakeBuildTarget(case-insensitive) = %q, want %q", resolved, "smoke")
	}

	_, err = resolveCMakeBuildTarget("fixture", targets)
	if err == nil {
		t.Fatal("resolveCMakeBuildTarget(fixture) unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), `cmake target "fixture" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "smoke") {
		t.Fatalf("error does not include available target list: %v", err)
	}
	if strings.Contains(err.Error(), "ZERO_CHECK") {
		t.Fatalf("error should hide internal meta target ZERO_CHECK: %v", err)
	}
}

func TestListCMakeTargetsFromFileAPI(t *testing.T) {
	buildDir := t.TempDir()
	replyDir := filepath.Join(buildDir, ".cmake", "api", "v1", "reply")
	if err := os.MkdirAll(replyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(replyDir) returned error: %v", err)
	}

	indexJSON := `{"objects":[{"kind":"codemodel","jsonFile":"codemodel-v2-test.json"}]}`
	if err := os.WriteFile(filepath.Join(replyDir, "index-123.json"), []byte(indexJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(index) returned error: %v", err)
	}

	codemodelJSON := `{
  "configurations":[
    {"targets":[{"name":"smoke"},{"name":"ALL_BUILD"}]},
    {"targets":[{"name":"smoke"},{"name":"fixture"}]}
  ]
}`
	if err := os.WriteFile(filepath.Join(replyDir, "codemodel-v2-test.json"), []byte(codemodelJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(codemodel) returned error: %v", err)
	}

	targets, err := listCMakeTargets(buildDir)
	if err != nil {
		t.Fatalf("listCMakeTargets returned error: %v", err)
	}
	if got, want := strings.Join(targets, ","), "ALL_BUILD,fixture,smoke"; got != want {
		t.Fatalf("listCMakeTargets = %q, want %q", got, want)
	}
}

func TestListCMakeTargetsFallsBackToVisualStudioProjects(t *testing.T) {
	buildDir := t.TempDir()
	for _, name := range []string{"smoke.vcxproj", "ALL_BUILD.vcxproj", "ZERO_CHECK.vcxproj"} {
		if err := os.WriteFile(filepath.Join(buildDir, name), []byte("<Project />"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) returned error: %v", name, err)
		}
	}

	targets, err := listCMakeTargets(buildDir)
	if err != nil {
		t.Fatalf("listCMakeTargets returned error: %v", err)
	}
	if got, want := strings.Join(targets, ","), "ALL_BUILD,smoke,ZERO_CHECK"; got != want {
		t.Fatalf("listCMakeTargets fallback = %q, want %q", got, want)
	}
}

func TestBuildServiceCleanupRemovesOldestJobsWhenByteLimitExceeded(t *testing.T) {
	rootDir := t.TempDir()
	buildTempRoot := filepath.Join(t.TempDir(), "roodox-builds")
	buildCount := 0

	svc := NewBuildService(BuildConfig{
		RootDir:          rootDir,
		RemoteEnabled:    true,
		MaxWorkers:       1,
		JobTTL:           time.Hour,
		CleanupInterval:  time.Hour,
		MaxRetainedJobs:  200,
		MaxRetainedBytes: 0,
		TempRoot:         buildTempRoot,
		RunBuild: func(unitAbs, buildRoot, target string, appendLog func(string, ...any)) (string, error) {
			buildCount++
			if err := os.MkdirAll(buildRoot, 0o755); err != nil {
				return "", err
			}
			content := []byte("12345678")
			if err := os.WriteFile(filepath.Join(buildRoot, "artifact.txt"), content, 0o644); err != nil {
				return "", err
			}
			return "", nil
		},
	})
	defer svc.Close()

	first, err := svc.StartBuild(context.Background(), &pb.StartBuildRequest{UnitPath: "build-one"})
	if err != nil {
		t.Fatalf("first StartBuild returned error: %v", err)
	}
	second, err := svc.StartBuild(context.Background(), &pb.StartBuildRequest{UnitPath: "build-two"})
	if err != nil {
		t.Fatalf("second StartBuild returned error: %v", err)
	}

	waitForBuildDoneLocal(t, svc, first.BuildId, 5*time.Second)
	waitForBuildDoneLocal(t, svc, second.BuildId, 5*time.Second)

	svc.maxRetainedBytes = 10
	svc.cleanupJobs(time.Now())

	firstDir := filepath.Join(buildTempRoot, first.BuildId)
	secondDir := filepath.Join(buildTempRoot, second.BuildId)

	firstMissing := false
	if _, err := svc.GetBuildStatus(context.Background(), &pb.GetBuildStatusRequest{BuildId: first.BuildId}); err != nil {
		if status.Code(err) != codes.NotFound {
			t.Fatalf("first build returned unexpected status error: %v", err)
		}
		firstMissing = true
	}

	secondMissing := false
	if _, err := svc.GetBuildStatus(context.Background(), &pb.GetBuildStatusRequest{BuildId: second.BuildId}); err != nil {
		if status.Code(err) != codes.NotFound {
			t.Fatalf("second build returned unexpected status error: %v", err)
		}
		secondMissing = true
	}

	if firstMissing == secondMissing {
		t.Fatalf("expected exactly one build to be removed, firstMissing=%t secondMissing=%t", firstMissing, secondMissing)
	}

	if firstMissing {
		if _, err := os.Stat(firstDir); !os.IsNotExist(err) {
			t.Fatalf("expected first build dir to be removed, stat err=%v", err)
		}
		if _, err := os.Stat(secondDir); err != nil {
			t.Fatalf("expected second build dir to remain, stat err=%v", err)
		}
		return
	}

	if _, err := os.Stat(secondDir); !os.IsNotExist(err) {
		t.Fatalf("expected second build dir to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(firstDir); err != nil {
		t.Fatalf("expected first build dir to remain, stat err=%v", err)
	}
}

func waitForBuildDoneLocal(t *testing.T, svc *BuildService, buildID string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := svc.GetBuildStatus(context.Background(), &pb.GetBuildStatusRequest{BuildId: buildID})
		if err != nil {
			t.Fatalf("GetBuildStatus(%s) returned error: %v", buildID, err)
		}
		if resp.Status == "success" {
			return
		}
		if resp.Status == "failed" {
			t.Fatalf("build %s failed: %s", buildID, resp.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("build %s did not finish within %v", buildID, timeout)
}
