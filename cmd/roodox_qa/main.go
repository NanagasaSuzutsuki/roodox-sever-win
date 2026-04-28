package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"roodox_server/internal/appconfig"
	"roodox_server/internal/qasuite"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "live":
		runLive(os.Args[2:])
	case "soak":
		runSoak(os.Args[2:])
	case "faults":
		runFaults(os.Args[2:])
	case "probe":
		runProbe(os.Args[2:])
	default:
		printUsage()
		os.Exit(2)
	}
}

func runLive(args []string) {
	fs := flag.NewFlagSet("live", flag.ExitOnError)
	override := bindOverrideFlags(fs)
	keepArtifacts := fs.Bool("keep-artifacts", false, "keep QA files under root_dir after completion")
	fs.Parse(args)

	rt := mustLoadRuntime(*override)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	must(qasuite.RunLive(ctx, rt, qasuite.LiveOptions{
		KeepArtifacts: *keepArtifacts,
	}))
}

func runSoak(args []string) {
	fs := flag.NewFlagSet("soak", flag.ExitOnError)
	override := bindOverrideFlags(fs)
	duration := fs.Duration("duration", 2*time.Minute, "how long to run the soak loop")
	workers := fs.Int("workers", 4, "number of file workers")
	buildInterval := fs.Duration("build-interval", 20*time.Second, "how often to trigger a build; 0 disables builds")
	backupOnce := fs.Bool("backup-once", true, "trigger one backup during the soak run")
	keepArtifacts := fs.Bool("keep-artifacts", false, "keep QA files under root_dir after completion")
	fs.Parse(args)

	rt := mustLoadRuntime(*override)
	ctx, cancel := context.WithTimeout(context.Background(), *duration+45*time.Second)
	defer cancel()
	must(qasuite.RunSoak(ctx, rt, qasuite.SoakOptions{
		Duration:      *duration,
		Workers:       *workers,
		BuildInterval: *buildInterval,
		BackupOnce:    *backupOnce,
		KeepArtifacts: *keepArtifacts,
	}))
}

func runFaults(args []string) {
	fs := flag.NewFlagSet("faults", flag.ExitOnError)
	override := bindOverrideFlags(fs)
	keepArtifacts := fs.Bool("keep-artifacts", false, "keep QA files under root_dir after completion")
	fs.Parse(args)

	rt := mustLoadRuntime(*override)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	must(qasuite.RunFaults(ctx, rt, qasuite.FaultOptions{
		KeepArtifacts: *keepArtifacts,
	}))
}

func runProbe(args []string) {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	override := bindOverrideFlags(fs)
	pre := fs.Duration("pre", 5*time.Second, "steady-state probe window before restart")
	down := fs.Duration("down", 7*time.Second, "expected outage window")
	post := fs.Duration("post", 14*time.Second, "recovery probe window after restart")
	interval := fs.Duration("interval", 1*time.Second, "probe interval")
	fs.Parse(args)

	rt := mustLoadRuntime(*override)
	ctx, cancel := context.WithTimeout(context.Background(), *pre+*down+*post+15*time.Second)
	defer cancel()
	must(qasuite.RunRestartProbe(ctx, rt, qasuite.ProbeOptions{
		Pre:      *pre,
		Down:     *down,
		Post:     *post,
		Interval: *interval,
	}))
}

func bindOverrideFlags(fs *flag.FlagSet) *qasuite.Override {
	override := &qasuite.Override{}
	fs.StringVar(&override.ConfigPath, "config", appconfig.ConfigPath, "path to roodox.config.json")
	fs.StringVar(&override.Addr, "addr", "", "override dial address")
	fs.StringVar(&override.RootDir, "root-dir", "", "override root_dir for QA fixtures and cleanup")
	fs.StringVar(&override.SharedSecret, "shared-secret", "", "override shared secret used for QA connections")
	fs.StringVar(&override.TLSRootCertPath, "tls-root-cert", "", "override TLS root certificate path")
	fs.StringVar(&override.TLSServerName, "tls-server-name", "", "override TLS server name")
	fs.StringVar(&override.ServerID, "server-id", "", "override server_id used during device registration")
	return override
}

func mustLoadRuntime(override qasuite.Override) qasuite.Runtime {
	rt, err := qasuite.LoadRuntime(override)
	must(err)
	return rt
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <live|soak|faults|probe> [flags]\n", os.Args[0])
}
