package qasuite

import (
	"context"
	"fmt"
	"time"
)

type ProbeOptions struct {
	Pre      time.Duration
	Down     time.Duration
	Post     time.Duration
	Interval time.Duration
}

func RunRestartProbe(ctx context.Context, rt Runtime, opts ProbeOptions) error {
	if opts.Pre <= 0 {
		opts.Pre = 5 * time.Second
	}
	if opts.Down <= 0 {
		opts.Down = 7 * time.Second
	}
	if opts.Post <= 0 {
		opts.Post = 14 * time.Second
	}
	if opts.Interval <= 0 {
		opts.Interval = 1 * time.Second
	}

	type phaseStats struct {
		Success int
		Fail    int
	}
	preStats := phaseStats{}
	downStats := phaseStats{}
	postStats := phaseStats{}

	startedAt := time.Now()
	total := opts.Pre + opts.Down + opts.Post
	for time.Since(startedAt) < total {
		elapsed := time.Since(startedAt)
		phase := "post"
		switch {
		case elapsed < opts.Pre:
			phase = "pre"
		case elapsed < opts.Pre+opts.Down:
			phase = "down"
		}

		err := probeOnce(ctx, rt)
		switch phase {
		case "pre":
			if err != nil {
				preStats.Fail++
			} else {
				preStats.Success++
			}
		case "down":
			if err != nil {
				downStats.Fail++
			} else {
				downStats.Success++
			}
		default:
			if err != nil {
				postStats.Fail++
			} else {
				postStats.Success++
			}
		}

		if err != nil {
			fmt.Printf("[probe] phase=%s result=fail err=%v\n", phase, err)
		} else {
			fmt.Printf("[probe] phase=%s result=ok\n", phase)
		}
		time.Sleep(opts.Interval)
	}

	fmt.Printf("[probe] pre_success=%d pre_fail=%d down_success=%d down_fail=%d post_success=%d post_fail=%d\n",
		preStats.Success,
		preStats.Fail,
		downStats.Success,
		downStats.Fail,
		postStats.Success,
		postStats.Fail,
	)

	if preStats.Success == 0 {
		return fmt.Errorf("restart probe failed: no success before restart window")
	}
	if downStats.Fail == 0 {
		return fmt.Errorf("restart probe failed: no failure during restart window")
	}
	if postStats.Success == 0 {
		return fmt.Errorf("restart probe failed: no success after restart window")
	}
	return nil
}

func probeOnce(ctx context.Context, rt Runtime) error {
	c, err := rt.Dial()
	if err != nil {
		return err
	}
	defer c.Close()

	opCtx, cancel := OpContext(ctx, 2*time.Second)
	defer cancel()
	if _, err := c.HealthCheck(opCtx); err != nil {
		return err
	}
	if _, err := c.ListDevices(opCtx); err != nil {
		return err
	}
	return nil
}
