package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kaileying/agent-tally.nvim/sidecar/internal/config"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/daemon"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg := config.Default()

	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "path to SQLite database")
	flag.StringVar(&cfg.SocketPath, "socket", cfg.SocketPath, "path to UNIX domain socket")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d, err := daemon.New(cfg)
	if err != nil {
		log.Fatalf("failed to create daemon: %v", err)
	}

	if err := d.Start(ctx); err != nil {
		log.Fatalf("failed to start daemon: %v", err)
	}

	<-ctx.Done()
	d.Stop()
}
