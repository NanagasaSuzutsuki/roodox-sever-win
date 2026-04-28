package serverapp

import (
	"time"

	"roodox_server/internal/cleanup"
	"roodox_server/internal/observability"
)

const defaultObservabilityReportInterval = 5 * time.Minute

type observabilityReporter struct {
	recorder *observability.Recorder
	interval time.Duration
	runner   *cleanup.Runner
}

func newObservabilityReporter(recorder *observability.Recorder, interval time.Duration) *observabilityReporter {
	if recorder == nil {
		return nil
	}
	if interval <= 0 {
		interval = defaultObservabilityReportInterval
	}

	r := &observabilityReporter{
		recorder: recorder,
		interval: interval,
	}
	r.runner = cleanup.NewRunner(0, r.report)
	return r
}

func (r *observabilityReporter) Close() {
	if r == nil || r.runner == nil {
		return
	}
	r.runner.Close()
}

func (r *observabilityReporter) Trigger() {
	if r == nil || r.runner == nil {
		return
	}
	r.runner.Trigger()
}

func (r *observabilityReporter) report(now time.Time) time.Time {
	if r == nil || r.recorder == nil {
		return time.Time{}
	}
	r.recorder.LogSnapshot()
	return now.Add(r.interval)
}
