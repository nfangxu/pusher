package main

import (
	"fmt"
	"log"
	"pusher/internal/config"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	fmt.Printf("server starting on %s:%d\n", cfg.App.Host, cfg.App.Port)
}
