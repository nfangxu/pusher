package config

import (
	"os"
	"testing"
)

func TestLoad_Success(t *testing.T) {
	content := `
app:
  host: "127.0.0.1"
  port: 9090
log:
  level: "debug"
  path: "/tmp/test.log"
  max_days: 3
sse:
  heartbeat_interval: 15
  read_timeout: 30
  cors_origins: "http://localhost:3000"
  worker_num: 4
  push_queue_capacity: 5000
  shard_num: 16
token:
  salt: "test-salt"
  expire_seconds: 1800
push:
  token: "test-push-token"
  rate_limit: 50
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Host != "127.0.0.1" {
		t.Errorf("App.Host = %v, want 127.0.0.1", cfg.App.Host)
	}
	if cfg.App.Port != 9090 {
		t.Errorf("App.Port = %v, want 9090", cfg.App.Port)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %v, want debug", cfg.Log.Level)
	}
	if cfg.SSE.HeartbeatInterval != 15 {
		t.Errorf("SSE.HeartbeatInterval = %v, want 15", cfg.SSE.HeartbeatInterval)
	}
	if cfg.Token.Salt != "test-salt" {
		t.Errorf("Token.Salt = %v, want test-salt", cfg.Token.Salt)
	}
	if cfg.Push.Token != "test-push-token" {
		t.Errorf("Push.Token = %v, want test-push-token", cfg.Push.Token)
	}
}

func TestLoad_Defaults(t *testing.T) {
	content := `
app:
  port: 8080
token:
  salt: "test-salt"
push:
  token: "test-push-token"
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Host != "0.0.0.0" {
		t.Errorf("App.Host = %v, want 0.0.0.0", cfg.App.Host)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %v, want info", cfg.Log.Level)
	}
	if cfg.SSE.HeartbeatInterval != 30 {
		t.Errorf("SSE.HeartbeatInterval = %v, want 30", cfg.SSE.HeartbeatInterval)
	}
	if cfg.Token.ExpireSeconds != 3600 {
		t.Errorf("Token.ExpireSeconds = %v, want 3600", cfg.Token.ExpireSeconds)
	}
}

func TestLoad_AllowsNonEmptyPlaceholderSecrets(t *testing.T) {
	content := `
app:
  port: 8080
token:
  salt: "your-secret-salt-here"
push:
  token: "your-push-token-here"
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Token.Salt != "your-secret-salt-here" {
		t.Errorf("Token.Salt = %v, want your-secret-salt-here", cfg.Token.Salt)
	}
	if cfg.Push.Token != "your-push-token-here" {
		t.Errorf("Push.Token = %v, want your-push-token-here", cfg.Push.Token)
	}
}

func TestLoad_ValidationError(t *testing.T) {
	content := `
token:
  salt: "test-salt"
push:
  token: "test-push-token"
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	_, err = Load(tmpFile.Name())
	if err == nil {
		t.Error("Load() expected error, got nil")
	}
}
