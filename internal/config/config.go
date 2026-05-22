package config

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
	return &Config{}, nil
}
