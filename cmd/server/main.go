package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"health-dashboard/internal/auth"
	"health-dashboard/internal/config"
	"health-dashboard/internal/db"
	"health-dashboard/internal/monitor"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	hashPw := flag.String("hash-password", "", "hash a plaintext password and print the result, then exit")
	flag.Parse()

	if *hashPw != "" {
		h, err := auth.HashPassword(*hashPw)
		if err != nil {
			log.Fatalf("hash-password: %v", err)
		}
		fmt.Println(h)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if !strings.HasPrefix(cfg.Auth.Password, "$2a$") && !strings.HasPrefix(cfg.Auth.Password, "$2b$") {
		log.Println("WARNING: password in config.yaml is stored as plaintext — use --hash-password to generate a bcrypt hash")
	}

	if err := os.MkdirAll(cfg.Server.DataDir, 0o755); err != nil {
		log.Fatalf("data dir: %v", err)
	}

	database, err := db.Open(cfg.Server.DataDir)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	// Signal-aware context — cancels when the process receives SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	monitorStore := monitor.NewStore(database)
	checker := monitor.NewChecker(monitorStore)
	if err := checker.Start(ctx); err != nil {
		log.Fatalf("checker start: %v", err)
	}

	sessions := auth.NewStore()

	srv := &server{
		cfg:      cfg,
		db:       database,
		sessions: sessions,
		monitors: monitorStore,
		checker:  checker,
	}

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: srv.routes(),
	}

	// Serve in a goroutine so we can react to the shutdown signal.
	go func() {
		log.Printf("health-dashboard listening on %s", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	// Block until SIGINT/SIGTERM.
	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	checker.Stop()
}
