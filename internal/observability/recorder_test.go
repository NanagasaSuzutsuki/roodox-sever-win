package observability

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
)

func TestRecorderTracksRangeWriteBurstsAndHotPaths(t *testing.T) {
	rec := NewRecorder(Config{
		RPCLatencySampleSize:   8,
		SmallWriteThreshold:    8,
		SmallWriteWindow:       time.Second,
		SmallWriteBurstMinimum: 3,
	})

	base := time.Unix(1700000000, 0)
	rec.recordRangeWriteAt("hot.txt", 4, false, base)
	rec.recordRangeWriteAt("hot.txt", 4, false, base.Add(100*time.Millisecond))
	rec.recordRangeWriteAt("hot.txt", 4, true, base.Add(200*time.Millisecond))
	rec.recordRangeWriteAt("cold.txt", 16, false, base.Add(300*time.Millisecond))

	snap := rec.Snapshot()
	if snap.RangeWriteCalls != 4 {
		t.Fatalf("RangeWriteCalls = %d, want 4", snap.RangeWriteCalls)
	}
	if snap.RangeWriteBytes != 28 {
		t.Fatalf("RangeWriteBytes = %d, want 28", snap.RangeWriteBytes)
	}
	if snap.RangeWriteConflictCalls != 1 {
		t.Fatalf("RangeWriteConflictCalls = %d, want 1", snap.RangeWriteConflictCalls)
	}
	if snap.SmallWriteBursts != 1 {
		t.Fatalf("SmallWriteBursts = %d, want 1", snap.SmallWriteBursts)
	}
	if len(snap.SmallWriteHotPaths) != 1 {
		t.Fatalf("SmallWriteHotPaths length = %d, want 1", len(snap.SmallWriteHotPaths))
	}
	if snap.SmallWriteHotPaths[0].Path != "hot.txt" {
		t.Fatalf("hot path = %q, want %q", snap.SmallWriteHotPaths[0].Path, "hot.txt")
	}
	if snap.SmallWriteHotPaths[0].Count != 1 {
		t.Fatalf("hot path burst count = %d, want 1", snap.SmallWriteHotPaths[0].Count)
	}
}

func TestRecorderTracksRPCAndBuildPercentiles(t *testing.T) {
	rec := NewRecorder(Config{
		RPCLatencySampleSize:   8,
		SmallWriteThreshold:    8,
		SmallWriteWindow:       time.Second,
		SmallWriteBurstMinimum: 3,
	})

	rec.RecordRPC("/roodox.core.v1.CoreService/WriteFileRange", codes.OK, 10*time.Millisecond)
	rec.RecordRPC("/roodox.core.v1.CoreService/WriteFileRange", codes.OK, 20*time.Millisecond)
	rec.RecordRPC("/roodox.core.v1.CoreService/WriteFileRange", codes.Internal, 30*time.Millisecond)
	rec.RecordRPC("/roodox.core.v1.CoreService/WriteFileRange", codes.OK, 40*time.Millisecond)

	rec.RecordBuildQueueWait(5 * time.Millisecond)
	rec.RecordBuildQueueWait(15 * time.Millisecond)
	rec.RecordBuildCompletion(true, 100*time.Millisecond, 128)
	rec.RecordBuildCompletion(false, 200*time.Millisecond, 256)

	snap := rec.Snapshot()
	if len(snap.RPCLatencies) != 1 {
		t.Fatalf("RPCLatencies length = %d, want 1", len(snap.RPCLatencies))
	}

	rpc := snap.RPCLatencies[0]
	if rpc.Count != 4 {
		t.Fatalf("rpc.Count = %d, want 4", rpc.Count)
	}
	if rpc.ErrorCount != 1 {
		t.Fatalf("rpc.ErrorCount = %d, want 1", rpc.ErrorCount)
	}
	if rpc.P50Ms != 20 {
		t.Fatalf("rpc.P50Ms = %d, want 20", rpc.P50Ms)
	}
	if rpc.P95Ms != 40 {
		t.Fatalf("rpc.P95Ms = %d, want 40", rpc.P95Ms)
	}
	if rpc.P99Ms != 40 {
		t.Fatalf("rpc.P99Ms = %d, want 40", rpc.P99Ms)
	}

	if snap.BuildQueueWait.Count != 2 {
		t.Fatalf("BuildQueueWait.Count = %d, want 2", snap.BuildQueueWait.Count)
	}
	if snap.BuildQueueWait.P50Ms != 5 {
		t.Fatalf("BuildQueueWait.P50Ms = %d, want 5", snap.BuildQueueWait.P50Ms)
	}
	if snap.BuildQueueWait.P95Ms != 15 {
		t.Fatalf("BuildQueueWait.P95Ms = %d, want 15", snap.BuildQueueWait.P95Ms)
	}
	if snap.BuildDuration.Count != 2 {
		t.Fatalf("BuildDuration.Count = %d, want 2", snap.BuildDuration.Count)
	}
	if snap.BuildDuration.P50Ms != 100 {
		t.Fatalf("BuildDuration.P50Ms = %d, want 100", snap.BuildDuration.P50Ms)
	}
	if snap.BuildDuration.P95Ms != 200 {
		t.Fatalf("BuildDuration.P95Ms = %d, want 200", snap.BuildDuration.P95Ms)
	}
	if snap.BuildSuccessCount != 1 || snap.BuildFailureCount != 1 {
		t.Fatalf("build counts = (%d,%d), want (1,1)", snap.BuildSuccessCount, snap.BuildFailureCount)
	}
	if snap.BuildLogBytes != 384 {
		t.Fatalf("BuildLogBytes = %d, want 384", snap.BuildLogBytes)
	}
}
