package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"pusher/internal/config"
	"pusher/internal/handler"
	"pusher/internal/log"
	"pusher/internal/push"
	"pusher/internal/registry"
	"pusher/internal/token"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := log.Init(cfg.Log.Level, cfg.Log.Path, cfg.Log.MaxDays); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}

	log.Info("server starting...")

	reg := registry.New(cfg.SSE.ShardNum)
	validator := token.NewValidator(cfg.Token.Salt, cfg.Token.ExpireSeconds)
	pushQueue := push.New(reg, cfg.SSE.PushQueueCapacity, cfg.SSE.WorkerNum)
	pushQueue.Start()

	sseHandler := handler.NewSSEHandler(reg, validator, cfg.SSE.HeartbeatInterval, cfg.SSE.ReadTimeout)
	pushHandler := handler.NewPushHandler(pushQueue, cfg.Push.Token)
	healthHandler := handler.NewHealthHandler(reg)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware(cfg.SSE.CORSOrigins))

	r.GET("/sse/connect", sseHandler.Connect)
	r.POST("/push", handler.RateLimitMiddleware(cfg.Push.RateLimit), pushHandler.Push)
	r.GET("/health", healthHandler.Health)

	addr := fmt.Sprintf("%s:%d", cfg.App.Host, cfg.App.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Infof("server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	pushQueue.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Errorf("server forced to shutdown: %v", err)
	}

	connections, groups, channels := reg.Stats()
	log.Infow("server stopped",
		"connections", connections,
		"groups", groups,
		"channels", channels,
	)
}

func corsMiddleware(origins string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", origins)
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
