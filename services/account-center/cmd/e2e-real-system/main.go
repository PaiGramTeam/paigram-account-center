//go:build integration

package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"paigram/internal/e2ereal"
)

func main() {
	var cfg e2ereal.Config
	flag.StringVar(&cfg.RepositoryRoot, "repository-root", "", "absolute repository root")
	flag.StringVar(&cfg.StateFile, "state-file", "", "path for the ready-state JSON file")
	flag.StringVar(&cfg.FrontendImage, "frontend-image", "", "immutable frontend image built from the current checkout")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			stop()
		}
	}()
	if err := e2ereal.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}
