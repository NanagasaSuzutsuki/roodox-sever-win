package server

import (
	"time"

	"google.golang.org/grpc/codes"
)

type RuntimeMetrics interface {
	RecordRPC(method string, code codes.Code, duration time.Duration)
	RecordRangeWrite(path string, bytes int64, conflicted bool)
	RecordBuildQueueWait(wait time.Duration)
	RecordBuildCompletion(success bool, duration time.Duration, logBytes int)
}
