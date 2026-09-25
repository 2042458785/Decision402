package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"Decision402/internal/probe"
)

func main() {
	envFile := flag.String("env", ".env", "local configuration file; existing environment variables take precedence")
	output := flag.String("out", "artifacts", "parent directory for a new timestamped report")
	timeout := flag.Duration("timeout", 15*time.Second, "timeout per request, including response body")
	flag.Parse()
	if flag.NArg() != 0 || *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "usage: intercepta-probe [-env .env] [-out artifacts] [-timeout 15s]")
		os.Exit(2)
	}
	cfg, err := probe.LoadConfig(*envFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Configuration:", err)
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	client := probe.NewClient(cfg.APIKey, *timeout)
	dir, err := probe.Run(ctx, cfg, client, *output, os.Stdout)
	if dir != "" {
		fmt.Println("Evidence directory:", dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Probe:", err)
		os.Exit(1)
	}
}
