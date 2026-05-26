package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pusher/internal/config"
	"pusher/internal/handler"
	"pusher/internal/log"
	"pusher/internal/push"
	"pusher/internal/registry"
	"pusher/internal/token"

	"github.com/gin-gonic/gin"
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

	reg := registry.New(cfg.Registry.ShardNum)
	validator := token.NewValidator(cfg.Token.Salt, cfg.Token.ExpireSeconds)
	pushQueue := push.New(reg, cfg.Push.QueueCapacity, cfg.Push.WorkerNum, cfg.Push.FanOutWorkers)
	pushQueue.Start()

	sseHandler := handler.NewSSEHandler(reg, validator, cfg.SSE.HeartbeatInterval, cfg.SSE.ReadTimeout)
	wsHandler := handler.NewWsHandler(reg, validator)
	pushHandler := handler.NewPushHandler(pushQueue, cfg.Push.Token)
	healthHandler := handler.NewHealthHandler(reg)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware(cfg.SSE.CORSOrigins))

	r.GET("/sse/connect", sseHandler.Connect)
	r.GET("/ws/connect", wsHandler.Connect)
	r.POST("/push", handler.RateLimitMiddleware(cfg.Push.RateLimit), pushHandler.Push)
	r.GET("/health", healthHandler.Health)

	addr := fmt.Sprintf("%s:%d", cfg.App.Host, cfg.App.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Infof("server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen error: %v", err)
		}
	}()

	pprofMux := http.NewServeMux()
	pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
	pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	pprofSrv := &http.Server{Addr: "localhost:6060", Handler: pprofMux}
	go func() {
		log.Infof("pprof server listening on localhost:6060")
		if err := pprofSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warnw("pprof server error", "error", err.Error())
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	pushQueue.Stop()
	reg.CloseAll()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Errorf("server forced to shutdown: %v", err)
	}
	if err := pprofSrv.Shutdown(ctx); err != nil {
		log.Warnw("pprof server shutdown error", "error", err.Error())
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
