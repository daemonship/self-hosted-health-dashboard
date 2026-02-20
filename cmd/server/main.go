package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"health-dashboard/internal/auth"
	"health-dashboard/internal/config"
	"health-dashboard/internal/db"
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

	sessions := auth.NewStore()

	srv := &server{
		cfg:      cfg,
		db:       database,
		sessions: sessions,
	}

	mux := srv.routes()

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("health-dashboard listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}
