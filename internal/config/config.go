package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Auth    AuthConfig    `yaml:"auth"`
	Agent   AgentConfig   `yaml:"agent"`
	Alerts  AlertsConfig  `yaml:"alerts"`
}

type ServerConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	DataDir string `yaml:"data_dir"`
}

type AuthConfig struct {
	// Password is either a bcrypt hash (starts with $2a$) or plaintext (dev only)
	Password      string `yaml:"password"`
	SessionSecret string `yaml:"session_secret"`
}

type AgentConfig struct {
	Token     string `yaml:"token"`
	ServerURL string `yaml:"server_url"`
}

type AlertsConfig struct {
	WebhookURL string `yaml:"webhook_url"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.DataDir == "" {
		c.Server.DataDir = "/data"
	}
}
