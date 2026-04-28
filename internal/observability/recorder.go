package observability

import (
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
)

const (
	defaultRPCLatencySampleSize   = 256
	defaultSmallWriteThreshold    = 64 << 10
	defaultSmallWriteWindow       = time.Second
	defaultSmallWriteBurstMinimum = 4
)

type Config struct {
	RPCLatencySampleSize   int
	SmallWriteThreshold    int64
	SmallWriteWindow       time.Duration
	SmallWriteBurstMinimum int
}

type Snapshot struct {
	RangeWriteCalls         int64
	RangeWriteBytes         int64
	RangeWriteConflictCalls int64
	SmallWriteBursts        int64
	SmallWriteHotPaths      []PathCount
	BuildSuccessCount       int64
	BuildFailureCount       int64
	BuildLogBytes           int64
	BuildQueueWait          DurationStats
	BuildDuration           DurationStats
	RPCLatencies            []RPCLatency
}

type PathCount struct {
	Path  string
	Count int64
}

type DurationStats struct {
	Count int64
	P50Ms int64
	P95Ms int64
	P99Ms int64
}

type RPCLatency struct {
	Method     string
	Count      int64
	ErrorCount int64
	P50Ms      int64
	P95Ms      int64
	P99Ms      int64
}

type Recorder struct {
	cfg Config

	mu sync.Mutex

	rangeWriteCalls         int64
	rangeWriteBytes         int64
	rangeWriteConflictCalls int64
	smallWriteBursts        int64
	smallWriteState         map[string]smallWriteWindow
	smallWritePathCounts    map[string]int64

	buildSuccessCount int64
	buildFailureCount int64
	buildLogBytes     int64
	buildQueueWait    durationSampler
	buildDuration     durationSampler

	rpc map[string]*rpcStats
}

type rpcStats struct {
	count      int64
	errorCount int64
	latency    durationSampler
}

type smallWriteWindow struct {
	start time.Time
	last  time.Time
	count int
}

type durationSampler struct {
	values []int64
	limit  int
	next   int
	count  int64
}

func DefaultConfig() Config {
	return Config{
		RPCLatencySampleSize:   defaultRPCLatencySampleSize,
		SmallWriteThreshold:    defaultSmallWriteThreshold,
		SmallWriteWindow:       defaultSmallWriteWindow,
		SmallWriteBurstMinimum: defaultSmallWriteBurstMinimum,
	}
}

func NewRecorder(cfg Config) *Recorder {
	if cfg.RPCLatencySampleSize <= 0 {
		cfg.RPCLatencySampleSize = defaultRPCLatencySampleSize
	}
	if cfg.SmallWriteThreshold <= 0 {
		cfg.SmallWriteThreshold = defaultSmallWriteThreshold
	}
	if cfg.SmallWriteWindow <= 0 {
		cfg.SmallWriteWindow = defaultSmallWriteWindow
	}
	if cfg.SmallWriteBurstMinimum <= 1 {
		cfg.SmallWriteBurstMinimum = defaultSmallWriteBurstMinimum
	}

	r := &Recorder{
		cfg:                  cfg,
		smallWriteState:      make(map[string]smallWriteWindow),
		smallWritePathCounts: make(map[string]int64),
		rpc:                  make(map[string]*rpcStats),
	}
	r.buildQueueWait.limit = cfg.RPCLatencySampleSize
	r.buildDuration.limit = cfg.RPCLatencySampleSize
	return r
}

func (r *Recorder) RecordRPC(method string, code codes.Code, duration time.Duration) {
	if r == nil || strings.TrimSpace(method) == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	stats := r.rpc[method]
	if stats == nil {
		stats = &rpcStats{}
		stats.latency.limit = r.cfg.RPCLatencySampleSize
		r.rpc[method] = stats
	}
	stats.count++
	if code != codes.OK {
		stats.errorCount++
	}
	stats.latency.Add(duration)
}

func (r *Recorder) RecordRangeWrite(path string, bytes int64, conflicted bool) {
	r.recordRangeWriteAt(path, bytes, conflicted, time.Now())
}

func (r *Recorder) RecordBuildQueueWait(wait time.Duration) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buildQueueWait.Add(wait)
}

func (r *Recorder) RecordBuildCompletion(success bool, duration time.Duration, logBytes int) {
	if r == nil {
		return
	}
	if logBytes < 0 {
		logBytes = 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if success {
		r.buildSuccessCount++
	} else {
		r.buildFailureCount++
	}
	r.buildLogBytes += int64(logBytes)
	r.buildDuration.Add(duration)
}

func (r *Recorder) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	snap := Snapshot{
		RangeWriteCalls:         r.rangeWriteCalls,
		RangeWriteBytes:         r.rangeWriteBytes,
		RangeWriteConflictCalls: r.rangeWriteConflictCalls,
		SmallWriteBursts:        r.smallWriteBursts,
		BuildSuccessCount:       r.buildSuccessCount,
		BuildFailureCount:       r.buildFailureCount,
		BuildLogBytes:           r.buildLogBytes,
		BuildQueueWait:          r.buildQueueWait.Snapshot(),
		BuildDuration:           r.buildDuration.Snapshot(),
	}

	for path, count := range r.smallWritePathCounts {
		snap.SmallWriteHotPaths = append(snap.SmallWriteHotPaths, PathCount{
			Path:  path,
			Count: count,
		})
	}
	sort.Slice(snap.SmallWriteHotPaths, func(i, j int) bool {
		if snap.SmallWriteHotPaths[i].Count == snap.SmallWriteHotPaths[j].Count {
			return snap.SmallWriteHotPaths[i].Path < snap.SmallWriteHotPaths[j].Path
		}
		return snap.SmallWriteHotPaths[i].Count > snap.SmallWriteHotPaths[j].Count
	})

	for method, stats := range r.rpc {
		latency := stats.latency.Snapshot()
		snap.RPCLatencies = append(snap.RPCLatencies, RPCLatency{
			Method:     method,
			Count:      stats.count,
			ErrorCount: stats.errorCount,
			P50Ms:      latency.P50Ms,
			P95Ms:      latency.P95Ms,
			P99Ms:      latency.P99Ms,
		})
	}
	sort.Slice(snap.RPCLatencies, func(i, j int) bool {
		return snap.RPCLatencies[i].Method < snap.RPCLatencies[j].Method
	})

	return snap
}

func (r *Recorder) LogSnapshot() bool {
	snap := r.Snapshot()
	if snap.isZero() {
		return false
	}

	log.Printf(
		"component=observability scope=file write_file_range_calls=%d write_file_range_bytes=%d write_file_range_conflicts=%d small_write_bursts=%d hot_paths=%q",
		snap.RangeWriteCalls,
		snap.RangeWriteBytes,
		snap.RangeWriteConflictCalls,
		snap.SmallWriteBursts,
		formatHotPaths(snap.SmallWriteHotPaths, 3),
	)
	log.Printf(
		"component=observability scope=build success_count=%d failure_count=%d queue_wait_count=%d queue_wait_p50_ms=%d queue_wait_p95_ms=%d queue_wait_p99_ms=%d duration_count=%d duration_p50_ms=%d duration_p95_ms=%d duration_p99_ms=%d log_bytes=%d",
		snap.BuildSuccessCount,
		snap.BuildFailureCount,
		snap.BuildQueueWait.Count,
		snap.BuildQueueWait.P50Ms,
		snap.BuildQueueWait.P95Ms,
		snap.BuildQueueWait.P99Ms,
		snap.BuildDuration.Count,
		snap.BuildDuration.P50Ms,
		snap.BuildDuration.P95Ms,
		snap.BuildDuration.P99Ms,
		snap.BuildLogBytes,
	)
	for _, rpc := range snap.RPCLatencies {
		log.Printf(
			"component=observability scope=rpc method=%q count=%d error_count=%d p50_ms=%d p95_ms=%d p99_ms=%d",
			rpc.Method,
			rpc.Count,
			rpc.ErrorCount,
			rpc.P50Ms,
			rpc.P95Ms,
			rpc.P99Ms,
		)
	}
	return true
}

func (r *Recorder) recordRangeWriteAt(path string, bytes int64, conflicted bool, now time.Time) {
	if r == nil {
		return
	}
	if bytes < 0 {
		bytes = 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rangeWriteCalls++
	r.rangeWriteBytes += bytes
	if conflicted {
		r.rangeWriteConflictCalls++
	}

	if strings.TrimSpace(path) == "" || bytes == 0 || bytes > r.cfg.SmallWriteThreshold {
		return
	}

	window := r.smallWriteState[path]
	if window.start.IsZero() || now.Sub(window.last) > r.cfg.SmallWriteWindow {
		window = smallWriteWindow{
			start: now,
			last:  now,
			count: 1,
		}
		r.smallWriteState[path] = window
		return
	}

	window.last = now
	window.count++
	if window.count == r.cfg.SmallWriteBurstMinimum {
		r.smallWriteBursts++
		r.smallWritePathCounts[path]++
	}
	r.smallWriteState[path] = window
}

func (s *durationSampler) Add(duration time.Duration) {
	if duration < 0 {
		duration = 0
	}
	ms := duration.Milliseconds()
	s.count++

	if s.limit <= 0 {
		s.values = append(s.values, ms)
		return
	}
	if len(s.values) < s.limit {
		s.values = append(s.values, ms)
		return
	}
	s.values[s.next] = ms
	s.next = (s.next + 1) % s.limit
}

func (s *durationSampler) Snapshot() DurationStats {
	values := append([]int64(nil), s.values...)
	if len(values) == 0 {
		return DurationStats{Count: s.count}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return DurationStats{
		Count: s.count,
		P50Ms: percentile(values, 50),
		P95Ms: percentile(values, 95),
		P99Ms: percentile(values, 99),
	}
}

func percentile(values []int64, p int) int64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		return values[0]
	}
	if p >= 100 {
		return values[len(values)-1]
	}

	index := (len(values)*p + 99) / 100
	if index <= 0 {
		index = 1
	}
	if index > len(values) {
		index = len(values)
	}
	return values[index-1]
}

func formatHotPaths(paths []PathCount, limit int) string {
	if len(paths) == 0 || limit == 0 {
		return ""
	}
	if limit < 0 || limit > len(paths) {
		limit = len(paths)
	}
	parts := make([]string, 0, limit)
	for _, item := range paths[:limit] {
		parts = append(parts, item.Path)
	}
	return strings.Join(parts, ",")
}

func (s Snapshot) isZero() bool {
	return s.RangeWriteCalls == 0 &&
		s.RangeWriteBytes == 0 &&
		s.RangeWriteConflictCalls == 0 &&
		s.SmallWriteBursts == 0 &&
		s.BuildSuccessCount == 0 &&
		s.BuildFailureCount == 0 &&
		s.BuildLogBytes == 0 &&
		len(s.RPCLatencies) == 0
}
