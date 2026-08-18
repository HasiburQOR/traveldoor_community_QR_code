// Package config loads application configuration from the environment,
// optionally seeded from a .env file in the working directory.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	AppEnv              string
	Addr                string
	BaseURL             string
	DatabasePath        string
	SessionSecret       string
	AdminBootstrapEmail string
	AdminBootstrapPass  string
	UploadDir           string
	SeedDefault         bool
	SeedForce           bool
}

// LoadDotEnv reads key=value pairs from path and sets any that are not
// already present in the process environment. A missing file is not an error.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
	return sc.Err()
}

func Load() (*Config, error) {
	c := &Config{
		AppEnv:              env("APP_ENV", "development"),
		Addr:                env("ADDR", ":8080"),
		BaseURL:             strings.TrimRight(env("BASE_URL", "http://localhost:8080"), "/"),
		DatabasePath:        env("DATABASE_PATH", "data/app.db"),
		SessionSecret:       env("SESSION_SECRET", ""),
		AdminBootstrapEmail: env("ADMIN_BOOTSTRAP_EMAIL", ""),
		AdminBootstrapPass:  env("ADMIN_BOOTSTRAP_PASSWORD", ""),
		UploadDir:           env("UPLOAD_DIR", "uploads"),
		SeedDefault:         truthy(env("SEED_DEFAULT", "false")) || strings.EqualFold(env("SEED_DEFAULT", ""), "force"),
		SeedForce:           strings.EqualFold(env("SEED_DEFAULT", ""), "force"),
	}

	if c.SessionSecret == "" {
		if c.IsProduction() {
			return nil, fmt.Errorf("SESSION_SECRET is required when APP_ENV=production")
		}
		c.SessionSecret = "dev-insecure-session-secret"
	}
	if c.IsProduction() && !strings.HasPrefix(c.BaseURL, "https://") {
		return nil, fmt.Errorf("BASE_URL must use https:// when APP_ENV=production")
	}
	return c, nil
}

func (c *Config) IsProduction() bool { return c.AppEnv == "production" }

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

// truthy accepts the usual boolean spellings used in .env files.
func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
