package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App   AppConfig   `yaml:"app"`
	Log   LogConfig   `yaml:"log"`
	SSE   SSEConfig   `yaml:"sse"`
	Token TokenConfig `yaml:"token"`
	Push  PushConfig  `yaml:"push"`
}

type AppConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type LogConfig struct {
	Level   string `yaml:"level"`
	Path    string `yaml:"path"`
	MaxDays int    `yaml:"max_days"`
}

type SSEConfig struct {
	HeartbeatInterval int    `yaml:"heartbeat_interval"`
	ReadTimeout       int    `yaml:"read_timeout"`
	CORSOrigins       string `yaml:"cors_origins"`
	WorkerNum         int    `yaml:"worker_num"`
	PushQueueCapacity int    `yaml:"push_queue_capacity"`
	ShardNum          int    `yaml:"shard_num"`
	FanOutWorkers     int    `yaml:"fan_out_workers"`
}

type TokenConfig struct {
	Salt          string `yaml:"salt"`
	ExpireSeconds int    `yaml:"expire_seconds"`
}

type PushConfig struct {
	Token     string `yaml:"token"`
	RateLimit int    `yaml:"rate_limit"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	cfg.setDefaults()
	return cfg, nil
}

func (c *Config) validate() error {
	if c.App.Port == 0 {
		return fmt.Errorf("app.port is required")
	}
	if c.Token.Salt == "" {
		return fmt.Errorf("token.salt is required")
	}
	if c.Push.Token == "" {
		return fmt.Errorf("push.token is required")
	}
	return nil
}

func (c *Config) setDefaults() {
	if c.App.Host == "" {
		c.App.Host = "0.0.0.0"
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Path == "" {
		c.Log.Path = "logs/pusher.log"
	}
	if c.Log.MaxDays == 0 {
		c.Log.MaxDays = 7
	}
	if c.SSE.HeartbeatInterval == 0 {
		c.SSE.HeartbeatInterval = 30
	}
	if c.SSE.ReadTimeout == 0 {
		c.SSE.ReadTimeout = 60
	}
	if c.SSE.CORSOrigins == "" {
		c.SSE.CORSOrigins = "*"
	}
	if c.SSE.WorkerNum == 0 {
		c.SSE.WorkerNum = 8
	}
	if c.SSE.PushQueueCapacity == 0 {
		c.SSE.PushQueueCapacity = 10000
	}
	if c.SSE.ShardNum == 0 {
		c.SSE.ShardNum = 32
	}
	if c.SSE.FanOutWorkers == 0 {
		c.SSE.FanOutWorkers = 200
	}
	if c.Token.ExpireSeconds == 0 {
		c.Token.ExpireSeconds = 3600
	}
	if c.Push.RateLimit == 0 {
		c.Push.RateLimit = 100
	}
}
