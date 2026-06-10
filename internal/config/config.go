package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type ModelConfig struct {
	ID                    string  `yaml:"id"`
	PriceInputPerMillion  float64 `yaml:"price_input_per_million"`
	PriceOutputPerMillion float64 `yaml:"price_output_per_million"`
}

type Config struct {
	PlatformURL      string        `yaml:"platform_url"`
	ProviderToken    string        `yaml:"provider_token"`
	Runtime          string        `yaml:"runtime"`
	RuntimeURL       string        `yaml:"runtime_url"`
	Models           []ModelConfig `yaml:"models"`
	HeartbeatSeconds int           `yaml:"heartbeat_seconds"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyEnvOverrides()

	if cfg.PlatformURL == "" {
		return Config{}, fmt.Errorf("platform_url is required")
	}
	if cfg.ProviderToken == "" {
		return Config{}, fmt.Errorf("provider_token is required")
	}
	if cfg.Runtime == "" {
		return Config{}, fmt.Errorf("runtime is required")
	}
	if cfg.RuntimeURL == "" {
		return Config{}, fmt.Errorf("runtime_url is required")
	}
	if len(cfg.Models) == 0 {
		return Config{}, fmt.Errorf("at least one model is required")
	}

	return cfg, nil
}

func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("LLMSHARE_PLATFORM_URL"); v != "" {
		c.PlatformURL = v
	}
	if v := os.Getenv("LLMSHARE_PROVIDER_TOKEN"); v != "" {
		c.ProviderToken = v
	}
	if v := os.Getenv("LLMSHARE_RUNTIME"); v != "" {
		c.Runtime = v
	}
	if v := os.Getenv("LLMSHARE_RUNTIME_URL"); v != "" {
		c.RuntimeURL = v
	}
	if v := os.Getenv("LLMSHARE_HEARTBEAT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.HeartbeatSeconds = n
		}
	}
}

func (c Config) HeartbeatInterval() time.Duration {
	if c.HeartbeatSeconds <= 0 {
		return 10 * time.Second
	}
	return time.Duration(c.HeartbeatSeconds) * time.Second
}
