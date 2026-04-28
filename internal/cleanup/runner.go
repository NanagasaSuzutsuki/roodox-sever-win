package cleanup

import (
	"sync"
	"time"
)

type Runner struct {
	interval  time.Duration
	run       func(time.Time) time.Time
	triggerCh chan struct{}
	stopCh    chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once
}

func NewRunner(interval time.Duration, run func(time.Time) time.Time) *Runner {
	r := &Runner{
		interval:  interval,
		run:       run,
		triggerCh: make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	go r.loop()
	return r
}

func (r *Runner) Trigger() {
	if r == nil {
		return
	}
	select {
	case r.triggerCh <- struct{}{}:
	default:
	}
}

func (r *Runner) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		close(r.stopCh)
		<-r.doneCh
	})
}

func (r *Runner) loop() {
	defer close(r.doneCh)

	var (
		lastRun     time.Time
		timer       *time.Timer
		timerCh     <-chan time.Time
		scheduledAt time.Time
	)

	stopTimer := func() {
		if timer == nil {
			timerCh = nil
			scheduledAt = time.Time{}
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerCh = nil
		scheduledAt = time.Time{}
	}

	scheduleAt := func(at time.Time) {
		if at.IsZero() {
			stopTimer()
			return
		}
		if !scheduledAt.IsZero() && !at.Before(scheduledAt) {
			return
		}

		wait := time.Until(at)
		if wait < 0 {
			wait = 0
		}

		if timer == nil {
			timer = time.NewTimer(wait)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(wait)
		}
		timerCh = timer.C
		scheduledAt = at
	}

	runNow := func(now time.Time) {
		stopTimer()
		if r.run == nil {
			lastRun = now
			return
		}
		nextAt := r.run(now)
		lastRun = now
		if !nextAt.IsZero() {
			if nextAt.Before(now) {
				nextAt = now
			}
			scheduleAt(nextAt)
		}
	}

	runNow(time.Now())

	for {
		select {
		case <-r.triggerCh:
			now := time.Now()
			if r.interval > 0 && !lastRun.IsZero() {
				allowedAt := lastRun.Add(r.interval)
				if now.Before(allowedAt) {
					scheduleAt(allowedAt)
					continue
				}
			}
			runNow(now)
		case now := <-timerCh:
			runNow(now)
		case <-r.stopCh:
			stopTimer()
			return
		}
	}
}
