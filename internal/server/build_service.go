package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"roodox_server/internal/cleanup"
	"roodox_server/internal/fs"
	pb "roodox_server/proto"
)

type BuildMode string

const (
	BuildModeRemote BuildMode = "remote"
	BuildModeLocal  BuildMode = "local"
)

const (
	defaultBuildJobTTL          = 30 * time.Minute
	defaultBuildCleanupInterval = time.Minute
	defaultMaxRetainedJobs      = 200
	defaultBuildLogLimitBytes   = 2 << 20
	logTruncationMarker         = "[... build log truncated ...]\n"
)

type BuildConfig struct {
	RootDir          string
	RemoteEnabled    bool
	MaxWorkers       int
	JobTTL           time.Duration
	MaxRetainedJobs  int
	MaxRetainedBytes int64
	CleanupInterval  time.Duration
	MaxLogBytes      int
	TempRoot         string
	RunBuild         func(unitAbs, buildRoot, target string, appendLog func(string, ...any)) (string, error)
	Metrics          RuntimeMetrics
}

type buildJob struct {
	id          string
	unitPath    string
	target      string
	status      string
	createdAt   time.Time
	queuedAt    time.Time
	startedAt   time.Time
	finishedAt  time.Time
	logBuf      []byte
	maxLogBytes int
	productPath string
	workDir     string
	err         error
	done        chan struct{}
	mu          sync.Mutex
}

type buildJobSnapshot struct {
	id         string
	status     string
	createdAt  time.Time
	finishedAt time.Time
	workDir    string
	sizeBytes  int64
}

type BuildService struct {
	pb.UnimplementedBuildServiceServer
	cfg   BuildConfig
	slots chan struct{}
	locks *PathLocker

	mu               sync.RWMutex
	jobs             map[string]*buildJob
	jobTTL           time.Duration
	maxRetainedJobs  int
	maxRetainedBytes int64
	cleanupInterval  time.Duration
	maxLogBytes      int
	tempRoot         string
	runBuildFn       func(unitAbs, buildRoot, target string, appendLog func(string, ...any)) (string, error)
	cleanupRunner    *cleanup.Runner
	closeOnce        sync.Once
	metrics          RuntimeMetrics
}

func NewBuildService(cfg BuildConfig) *BuildService {
	workers := cfg.MaxWorkers
	if workers <= 0 {
		workers = defaultBuildWorkers()
	}

	jobTTL := cfg.JobTTL
	if jobTTL <= 0 {
		jobTTL = defaultBuildJobTTL
	}

	maxRetainedJobs := cfg.MaxRetainedJobs
	if maxRetainedJobs <= 0 {
		maxRetainedJobs = defaultMaxRetainedJobs
	}

	cleanupInterval := cfg.CleanupInterval
	if cleanupInterval <= 0 {
		cleanupInterval = defaultBuildCleanupInterval
	}

	maxLogBytes := cfg.MaxLogBytes
	if maxLogBytes <= 0 {
		maxLogBytes = defaultBuildLogLimitBytes
	}

	tempRoot := strings.TrimSpace(cfg.TempRoot)
	if tempRoot == "" {
		tempRoot = filepath.Join(os.TempDir(), "roodox-builds")
	}

	svc := &BuildService{
		cfg:              cfg,
		slots:            make(chan struct{}, workers),
		locks:            NewPathLocker(),
		jobs:             make(map[string]*buildJob),
		jobTTL:           jobTTL,
		maxRetainedJobs:  maxRetainedJobs,
		maxRetainedBytes: cfg.MaxRetainedBytes,
		cleanupInterval:  cleanupInterval,
		maxLogBytes:      maxLogBytes,
		tempRoot:         tempRoot,
		runBuildFn:       runBuildCommands,
		metrics:          cfg.Metrics,
	}
	if cfg.RunBuild != nil {
		svc.runBuildFn = cfg.RunBuild
	}
	svc.cleanupRunner = cleanup.NewRunner(0, svc.cleanupJobs)
	return svc
}

func (s *BuildService) Close() {
	s.closeOnce.Do(func() {
		if s.cleanupRunner != nil {
			s.cleanupRunner.Close()
		}
	})
}

func (s *BuildService) StartBuild(ctx context.Context, req *pb.StartBuildRequest) (*pb.StartBuildResponse, error) {
	if !s.cfg.RemoteEnabled {
		return nil, status.Error(codes.FailedPrecondition, "remote build is disabled by server configuration")
	}
	if req.UnitPath == "" {
		return nil, status.Error(codes.InvalidArgument, "unit_path is required")
	}

	unitPath, err := normalizeProjectPath(s.cfg.RootDir, req.UnitPath)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	unitAbs, err := s.resolveUnitPath(unitPath)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	id, err := newBuildID()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	now := time.Now()
	job := &buildJob{
		id:          id,
		unitPath:    unitPath,
		target:      req.Target,
		status:      "queued",
		createdAt:   now,
		queuedAt:    now,
		maxLogBytes: s.maxLogBytes,
		done:        make(chan struct{}),
	}

	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()
	s.triggerCleanup()

	LogRequestEvent(ctx, "component=build op=StartBuild build_id=%q unit_path=%q target=%q queue_depth=%d", id, unitPath, req.Target, len(s.slots))

	go s.runBuild(job, unitAbs)

	return &pb.StartBuildResponse{BuildId: id}, nil
}

func (s *BuildService) GetBuildStatus(ctx context.Context, req *pb.GetBuildStatusRequest) (*pb.GetBuildStatusResponse, error) {
	job, err := s.getJob(req.BuildId)
	if err != nil {
		return nil, err
	}

	job.mu.Lock()
	defer job.mu.Unlock()

	errMsg := ""
	if job.err != nil {
		errMsg = job.err.Error()
	}

	productName := ""
	if job.productPath != "" {
		productName = filepath.Base(job.productPath)
	}

	startedAtUnix := int64(0)
	if !job.startedAt.IsZero() {
		startedAtUnix = job.startedAt.Unix()
	}

	finishedAtUnix := int64(0)
	if !job.finishedAt.IsZero() {
		finishedAtUnix = job.finishedAt.Unix()
	}

	return &pb.GetBuildStatusResponse{
		Status:         job.status,
		Error:          errMsg,
		StartedAtUnix:  startedAtUnix,
		FinishedAtUnix: finishedAtUnix,
		ProductName:    productName,
	}, nil
}

func (s *BuildService) FetchBuildLog(ctx context.Context, req *pb.FetchBuildLogRequest) (*pb.FetchBuildLogResponse, error) {
	job, err := s.getJob(req.BuildId)
	if err != nil {
		return nil, err
	}

	job.mu.Lock()
	defer job.mu.Unlock()

	text := fmt.Sprintf("[status=%s]\n%s", job.status, string(job.logBuf))
	if job.err != nil {
		text += "\n[error] " + job.err.Error()
	}

	return &pb.FetchBuildLogResponse{Text: text}, nil
}

func (s *BuildService) GetBuildProduct(ctx context.Context, req *pb.GetBuildProductRequest) (*pb.GetBuildProductResponse, error) {
	job, err := s.getJob(req.BuildId)
	if err != nil {
		return nil, err
	}

	job.mu.Lock()
	statusVal := job.status
	productPath := job.productPath
	jobErr := job.err
	job.mu.Unlock()

	switch statusVal {
	case "queued", "running":
		return nil, status.Error(codes.FailedPrecondition, "build is still running")
	case "failed":
		if jobErr != nil {
			return nil, status.Error(codes.FailedPrecondition, "build failed: "+jobErr.Error())
		}
		return nil, status.Error(codes.FailedPrecondition, "build failed")
	}

	if productPath == "" {
		return nil, status.Error(codes.NotFound, "build finished but no product found")
	}

	data, err := os.ReadFile(productPath)
	if err != nil {
		return nil, toGrpcError(err)
	}

	return &pb.GetBuildProductResponse{
		Name: filepath.Base(productPath),
		Data: data,
	}, nil
}

func (s *BuildService) getJob(buildID string) (*buildJob, error) {
	if buildID == "" {
		return nil, status.Error(codes.InvalidArgument, "build_id is required")
	}

	s.mu.RLock()
	job, ok := s.jobs[buildID]
	s.mu.RUnlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "build_id not found")
	}
	return job, nil
}

func (s *BuildService) runBuild(job *buildJob, unitAbs string) {
	defer close(job.done)

	log.Printf("component=build build_id=%q status=queued unit_path=%q target=%q", job.id, job.unitPath, job.target)
	unlock := s.locks.Lock(unitAbs)
	defer unlock()

	s.slots <- struct{}{}
	defer func() {
		<-s.slots
	}()

	startedAt := time.Now()
	workDir := filepath.Join(s.tempRoot, job.id)

	job.mu.Lock()
	job.status = "running"
	job.startedAt = startedAt
	job.workDir = workDir
	queueWait := startedAt.Sub(job.queuedAt).Round(time.Millisecond)
	job.mu.Unlock()

	if queueWait > 0 {
		job.appendLog("queued for %s before starting\n", queueWait)
	}
	if s.metrics != nil {
		s.metrics.RecordBuildQueueWait(queueWait)
	}
	log.Printf("component=build build_id=%q status=running unit_path=%q target=%q queue_wait_ms=%d", job.id, job.unitPath, job.target, queueWait.Milliseconds())
	job.appendLog("start build: unit=%s target=%s\n", job.unitPath, job.target)

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		s.failJob(job, startedAt, err)
		return
	}

	productPath, err := s.runBuildFn(unitAbs, workDir, job.target, job.appendLog)
	if err != nil {
		s.failJob(job, startedAt, err)
		return
	}

	s.completeJob(job, startedAt, productPath)
}

func (s *BuildService) completeJob(job *buildJob, startedAt time.Time, productPath string) {
	finishedAt := time.Now()

	job.mu.Lock()
	job.status = "success"
	job.productPath = productPath
	job.finishedAt = finishedAt
	job.mu.Unlock()

	job.appendLog("build success in %s\n", finishedAt.Sub(startedAt).Round(time.Millisecond))
	if productPath != "" {
		job.appendLog("product: %s\n", productPath)
	}
	if s.metrics != nil {
		s.metrics.RecordBuildCompletion(true, finishedAt.Sub(startedAt), job.logSize())
	}
	log.Printf("component=build build_id=%q status=success unit_path=%q target=%q product=%q duration_ms=%d", job.id, job.unitPath, job.target, productPath, finishedAt.Sub(startedAt).Milliseconds())
	s.triggerCleanup()
}

func (s *BuildService) failJob(job *buildJob, startedAt time.Time, err error) {
	finishedAt := time.Now()

	job.mu.Lock()
	job.status = "failed"
	job.err = err
	job.finishedAt = finishedAt
	job.mu.Unlock()

	job.appendLog("build failed: %v\n", err)
	if s.metrics != nil {
		s.metrics.RecordBuildCompletion(false, finishedAt.Sub(startedAt), job.logSize())
	}
	log.Printf("component=build build_id=%q status=failed unit_path=%q target=%q error=%q duration_ms=%d", job.id, job.unitPath, job.target, err.Error(), finishedAt.Sub(startedAt).Milliseconds())
	s.triggerCleanup()
}

func (s *BuildService) triggerCleanup() {
	if s == nil || s.cleanupRunner == nil {
		return
	}
	s.cleanupRunner.Trigger()
}

func (s *BuildService) cleanupJobs(now time.Time) time.Time {
	type removal struct {
		id      string
		workDir string
	}

	s.mu.Lock()
	if len(s.jobs) == 0 {
		s.mu.Unlock()
		return time.Time{}
	}

	removals := make(map[string]removal)
	terminalJobs := make([]buildJobSnapshot, 0, len(s.jobs))
	for id, job := range s.jobs {
		snap := snapshotBuildJob(job)
		if !isTerminalBuildStatus(snap.status) {
			continue
		}
		if s.jobTTL > 0 && !snap.finishedAt.IsZero() && now.Sub(snap.finishedAt) > s.jobTTL {
			removals[id] = removal{id: id, workDir: snap.workDir}
			continue
		}
		terminalJobs = append(terminalJobs, snap)
	}

	remainingJobs := len(s.jobs) - len(removals)
	if s.maxRetainedJobs > 0 && remainingJobs > s.maxRetainedJobs {
		sort.Slice(terminalJobs, func(i, j int) bool {
			if terminalJobs[i].finishedAt.Equal(terminalJobs[j].finishedAt) {
				return terminalJobs[i].createdAt.Before(terminalJobs[j].createdAt)
			}
			return terminalJobs[i].finishedAt.Before(terminalJobs[j].finishedAt)
		})

		extra := remainingJobs - s.maxRetainedJobs
		for _, snap := range terminalJobs {
			if extra == 0 {
				break
			}
			if _, exists := removals[snap.id]; exists {
				continue
			}
			removals[snap.id] = removal{id: snap.id, workDir: snap.workDir}
			extra--
		}
	}

	terminalCandidates := make([]buildJobSnapshot, 0, len(terminalJobs))
	for _, snap := range terminalJobs {
		if _, exists := removals[snap.id]; exists {
			continue
		}
		terminalCandidates = append(terminalCandidates, snap)
	}
	s.mu.Unlock()

	if s.maxRetainedBytes > 0 {
		for _, candidate := range buildRemovalsForByteLimit(terminalCandidates, s.maxRetainedBytes) {
			removals[candidate.id] = removal{id: candidate.id, workDir: candidate.workDir}
		}
	}

	retainedTerminalJobs := terminalCandidates[:0]
	for _, snap := range terminalCandidates {
		if _, exists := removals[snap.id]; exists {
			continue
		}
		retainedTerminalJobs = append(retainedTerminalJobs, snap)
	}
	nextDue := nextBuildCleanupDue(retainedTerminalJobs, s.jobTTL)

	workDirs := make([]string, 0, len(removals))
	s.mu.Lock()
	for id, removal := range removals {
		delete(s.jobs, id)
		if removal.workDir != "" {
			workDirs = append(workDirs, removal.workDir)
		}
	}
	s.mu.Unlock()

	for _, workDir := range workDirs {
		if err := os.RemoveAll(workDir); err != nil {
			log.Printf("component=build op=cleanup work_dir=%q error=%q", workDir, err.Error())
		}
	}
	return nextDue
}

func snapshotBuildJob(job *buildJob) buildJobSnapshot {
	job.mu.Lock()
	defer job.mu.Unlock()

	return buildJobSnapshot{
		id:         job.id,
		status:     job.status,
		createdAt:  job.createdAt,
		finishedAt: job.finishedAt,
		workDir:    job.workDir,
	}
}

func buildRemovalsForByteLimit(snapshots []buildJobSnapshot, maxRetainedBytes int64) []struct {
	id      string
	workDir string
} {
	if maxRetainedBytes <= 0 || len(snapshots) == 0 {
		return nil
	}

	withSizes := make([]buildJobSnapshot, 0, len(snapshots))
	var totalBytes int64
	for _, snap := range snapshots {
		snap.sizeBytes = buildPathSize(snap.workDir)
		withSizes = append(withSizes, snap)
		totalBytes += snap.sizeBytes
	}
	if totalBytes <= maxRetainedBytes {
		return nil
	}

	sort.Slice(withSizes, func(i, j int) bool {
		if withSizes[i].finishedAt.Equal(withSizes[j].finishedAt) {
			return withSizes[i].createdAt.Before(withSizes[j].createdAt)
		}
		return withSizes[i].finishedAt.Before(withSizes[j].finishedAt)
	})

	removals := make([]struct {
		id      string
		workDir string
	}, 0, len(withSizes))
	for _, snap := range withSizes {
		if totalBytes <= maxRetainedBytes {
			break
		}
		removals = append(removals, struct {
			id      string
			workDir string
		}{
			id:      snap.id,
			workDir: snap.workDir,
		})
		totalBytes -= snap.sizeBytes
	}
	return removals
}

func buildPathSize(path string) int64 {
	if strings.TrimSpace(path) == "" {
		return 0
	}

	info, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return info.Size()
	}

	var total int64
	_ = filepath.Walk(path, func(current string, currentInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if currentInfo.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if currentInfo.IsDir() {
			return nil
		}
		total += currentInfo.Size()
		return nil
	})
	return total
}

func isTerminalBuildStatus(status string) bool {
	return status == "success" || status == "failed"
}

func nextBuildCleanupDue(snapshots []buildJobSnapshot, jobTTL time.Duration) time.Time {
	if jobTTL <= 0 || len(snapshots) == 0 {
		return time.Time{}
	}

	nextDue := time.Time{}
	for _, snap := range snapshots {
		if snap.finishedAt.IsZero() {
			continue
		}
		expireAt := snap.finishedAt.Add(jobTTL)
		if nextDue.IsZero() || expireAt.Before(nextDue) {
			nextDue = expireAt
		}
	}
	return nextDue
}

func (s *BuildService) resolveUnitPath(unitPath string) (string, error) {
	path, err := normalizeProjectPath(s.cfg.RootDir, unitPath)
	if err != nil {
		return "", err
	}
	return fs.ResolvePath(s.cfg.RootDir, path)
}

func (j *buildJob) appendLog(format string, args ...any) {
	entry := []byte(fmt.Sprintf(format, args...))

	j.mu.Lock()
	defer j.mu.Unlock()

	j.logBuf = appendLogWithLimit(j.logBuf, entry, j.maxLogBytes)
}

func (j *buildJob) logSize() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.logBuf)
}

func appendLogWithLimit(existing, entry []byte, limit int) []byte {
	if limit <= 0 {
		return append(existing, entry...)
	}

	buf := make([]byte, 0, len(existing)+len(entry))
	buf = append(buf, existing...)
	buf = append(buf, entry...)
	if len(buf) <= limit {
		return buf
	}

	if limit <= len(logTruncationMarker) {
		return append([]byte(nil), buf[len(buf)-limit:]...)
	}

	tailLen := limit - len(logTruncationMarker)
	start := len(buf) - tailLen
	if start < 0 {
		start = 0
	}

	trimmed := make([]byte, 0, limit)
	trimmed = append(trimmed, logTruncationMarker...)
	trimmed = append(trimmed, buf[start:]...)
	return trimmed
}

func newBuildID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func defaultBuildWorkers() int {
	workers := runtime.NumCPU() / 2
	if workers < 1 {
		workers = 1
	}
	if workers > 4 {
		workers = 4
	}
	return workers
}

func runBuildCommands(unitAbs, buildRoot, target string, appendLog func(string, ...any)) (string, error) {
	detected := detectBuildKind(unitAbs)
	switch detected {
	case "cmake":
		buildDir := filepath.Join(buildRoot, "build")
		if err := os.MkdirAll(buildDir, 0o755); err != nil {
			return "", err
		}
		if err := ensureCMakeFileAPIQuery(buildDir); err != nil {
			return "", err
		}
		args := []string{"-S", ".", "-B", buildDir}
		if err := runCmd(unitAbs, "cmake", args, appendLog); err != nil {
			return "", err
		}
		targets, err := listCMakeTargets(buildDir)
		if err != nil {
			appendLog("warn: list cmake targets failed: %v\n", err)
		}
		resolvedTarget, err := resolveCMakeBuildTarget(target, targets)
		if err != nil {
			return "", err
		}
		buildArgs := []string{"--build", buildDir}
		if resolvedTarget != "" {
			buildArgs = append(buildArgs, "--target", resolvedTarget)
		} else if strings.TrimSpace(target) != "" {
			appendLog("info: normalize cmake target %q to default build\n", target)
		}
		if err := runCmd(unitAbs, "cmake", buildArgs, appendLog); err != nil {
			return "", err
		}
		return findLatestProduct(buildDir)
	case "make":
		workDir, err := prepareIsolatedBuildWorkspace(unitAbs, buildRoot)
		if err != nil {
			return "", err
		}
		args := []string{}
		if target != "" {
			args = append(args, target)
		}
		if err := runCmd(workDir, "make", args, appendLog); err != nil {
			return "", err
		}
		return findLatestProduct(workDir)
	case "msbuild":
		workDir, err := prepareIsolatedBuildWorkspace(unitAbs, buildRoot)
		if err != nil {
			return "", err
		}
		args := []string{}
		if target != "" {
			args = append(args, "/t:"+target)
		}
		sln := firstMatch(workDir, "*.sln")
		if sln == "" {
			return "", errors.New("no .sln found under unit_path")
		}
		args = append([]string{sln}, args...)
		if err := runCmd(workDir, "msbuild", args, appendLog); err != nil {
			return "", err
		}
		return findLatestProduct(workDir)
	default:
		return "", errors.New("unsupported build unit: need CMakeLists.txt, Makefile, or .sln")
	}
}

func prepareIsolatedBuildWorkspace(unitAbs, buildRoot string) (string, error) {
	workDir := filepath.Join(buildRoot, "workspace")
	if err := copyDir(unitAbs, workDir); err != nil {
		return "", err
	}
	return workDir, nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel != "." && fs.ShouldIgnoreInProjectScan(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		targetPath := dst
		if rel != "." {
			targetPath = filepath.Join(dst, rel)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}

		return copyFile(path, targetPath, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func detectBuildKind(unitAbs string) string {
	if exists(filepath.Join(unitAbs, "CMakeLists.txt")) {
		return "cmake"
	}
	if exists(filepath.Join(unitAbs, "Makefile")) {
		return "make"
	}
	if firstMatch(unitAbs, "*.sln") != "" {
		return "msbuild"
	}
	return ""
}

func runCmd(dir, name string, args []string, appendLog func(string, ...any)) error {
	appendLog("run: %s %s\n", name, strings.Join(args, " "))

	cmd := exec.Command(name, args...)
	cmd.Dir = dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		copyStream(stdout, appendLog)
	}()
	go func() {
		defer wg.Done()
		copyStream(stderr, appendLog)
	}()

	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func copyStream(r io.Reader, appendLog func(string, ...any)) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			appendLog("%s", string(buf[:n]))
		}
		if err != nil {
			if err == io.EOF {
				return
			}
			appendLog("log stream read error: %v\n", err)
			return
		}
	}
}

type fileWithTime struct {
	path string
	mt   time.Time
}

func findLatestProduct(root string) (string, error) {
	candidates := make([]fileWithTime, 0)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !looksLikeProduct(path) {
			return nil
		}
		st, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		candidates = append(candidates, fileWithTime{
			path: path,
			mt:   st.ModTime(),
		})
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].mt.After(candidates[j].mt)
	})
	return candidates[0].path, nil
}

func looksLikeProduct(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if strings.HasSuffix(base, ".obj") || strings.HasSuffix(base, ".o") || strings.HasSuffix(base, ".pdb") {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))
	if runtime.GOOS == "windows" {
		return ext == ".exe" || ext == ".dll"
	}
	return ext == "" || ext == ".so" || ext == ".a" || ext == ".dylib"
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func firstMatch(root, pattern string) string {
	matches, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil || len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func ensureCMakeFileAPIQuery(buildDir string) error {
	queryFile := filepath.Join(buildDir, ".cmake", "api", "v1", "query", "codemodel-v2")
	if err := os.MkdirAll(filepath.Dir(queryFile), 0o755); err != nil {
		return err
	}
	if exists(queryFile) {
		return nil
	}
	return os.WriteFile(queryFile, []byte{}, 0o644)
}

func resolveCMakeBuildTarget(requested string, available []string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" || isCMakeDefaultTargetAlias(requested) {
		return "", nil
	}
	if len(available) == 0 {
		return requested, nil
	}

	for _, candidate := range available {
		if candidate == requested {
			return candidate, nil
		}
	}
	for _, candidate := range available {
		if strings.EqualFold(candidate, requested) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf(
		"cmake target %q not found; available targets: %s",
		requested,
		formatAvailableCMakeTargets(available),
	)
}

func isCMakeDefaultTargetAlias(target string) bool {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "", "all", "default":
		return true
	default:
		return false
	}
}

func listCMakeTargets(buildDir string) ([]string, error) {
	targets, err := listCMakeTargetsFromFileAPI(buildDir)
	if err == nil && len(targets) > 0 {
		return targets, nil
	}

	fallbackTargets, fallbackErr := listCMakeTargetsFromVisualStudioProjects(buildDir)
	if fallbackErr == nil && len(fallbackTargets) > 0 {
		return fallbackTargets, nil
	}

	if err != nil {
		if fallbackErr != nil {
			return nil, fmt.Errorf("file_api=%v; vcxproj_scan=%v", err, fallbackErr)
		}
		return nil, err
	}
	return fallbackTargets, fallbackErr
}

type cmakeFileAPIIndex struct {
	Objects []cmakeFileAPIObject `json:"objects"`
}

type cmakeFileAPIObject struct {
	Kind     string `json:"kind"`
	JsonFile string `json:"jsonFile"`
}

type cmakeFileAPICodemodel struct {
	Configurations []cmakeFileAPIConfiguration `json:"configurations"`
}

type cmakeFileAPIConfiguration struct {
	Targets []cmakeFileAPITarget `json:"targets"`
}

type cmakeFileAPITarget struct {
	Name string `json:"name"`
}

func listCMakeTargetsFromFileAPI(buildDir string) ([]string, error) {
	replyDir := filepath.Join(buildDir, ".cmake", "api", "v1", "reply")
	indexPath, err := latestCMakeFileAPIIndex(replyDir)
	if err != nil {
		return nil, err
	}

	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var index cmakeFileAPIIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		return nil, err
	}

	codemodelFile := ""
	for _, object := range index.Objects {
		if object.Kind == "codemodel" && strings.TrimSpace(object.JsonFile) != "" {
			codemodelFile = object.JsonFile
			break
		}
	}
	if codemodelFile == "" {
		return nil, errors.New("cmake codemodel reply not found")
	}

	codemodelData, err := os.ReadFile(filepath.Join(replyDir, codemodelFile))
	if err != nil {
		return nil, err
	}

	var codemodel cmakeFileAPICodemodel
	if err := json.Unmarshal(codemodelData, &codemodel); err != nil {
		return nil, err
	}

	return dedupeSortedTargets(flattenCMakeTargets(codemodel.Configurations)), nil
}

func latestCMakeFileAPIIndex(replyDir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(replyDir, "index-*.json"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", os.ErrNotExist
	}

	sort.Slice(matches, func(i, j int) bool {
		leftInfo, leftErr := os.Stat(matches[i])
		rightInfo, rightErr := os.Stat(matches[j])
		if leftErr == nil && rightErr == nil && !leftInfo.ModTime().Equal(rightInfo.ModTime()) {
			return leftInfo.ModTime().After(rightInfo.ModTime())
		}
		return matches[i] > matches[j]
	})

	return matches[0], nil
}

func flattenCMakeTargets(configurations []cmakeFileAPIConfiguration) []string {
	targets := make([]string, 0, len(configurations))
	for _, configuration := range configurations {
		for _, target := range configuration.Targets {
			if trimmed := strings.TrimSpace(target.Name); trimmed != "" {
				targets = append(targets, trimmed)
			}
		}
	}
	return targets
}

func listCMakeTargetsFromVisualStudioProjects(buildDir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(buildDir, "*.vcxproj"))
	if err != nil {
		return nil, err
	}
	targets := make([]string, 0, len(matches))
	for _, match := range matches {
		target := strings.TrimSpace(strings.TrimSuffix(filepath.Base(match), filepath.Ext(match)))
		if target == "" {
			continue
		}
		targets = append(targets, target)
	}
	return dedupeSortedTargets(targets), nil
}

func dedupeSortedTargets(targets []string) []string {
	seen := make(map[string]struct{}, len(targets))
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		key := strings.ToLower(strings.TrimSpace(target))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, strings.TrimSpace(target))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func formatAvailableCMakeTargets(targets []string) string {
	display := make([]string, 0, len(targets))
	for _, target := range dedupeSortedTargets(targets) {
		if isHiddenCMakeMetaTarget(target) {
			continue
		}
		display = append(display, target)
	}
	if len(display) == 0 {
		display = dedupeSortedTargets(targets)
	}
	if len(display) == 0 {
		return "(unavailable)"
	}
	if len(display) > 12 {
		return strings.Join(display[:12], ", ") + ", ..."
	}
	return strings.Join(display, ", ")
}

func isHiddenCMakeMetaTarget(target string) bool {
	switch strings.ToUpper(strings.TrimSpace(target)) {
	case "ZERO_CHECK", "INSTALL", "RUN_TESTS", "PACKAGE", "PACKAGE_SOURCE", "EDIT_CACHE", "REBUILD_CACHE":
		return true
	default:
		return false
	}
}
