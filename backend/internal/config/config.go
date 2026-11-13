// Package config читает конфигурацию бэкенда из переменных окружения.
package config

import (
	"strconv"
	"strings"
	"time"
)

// Config — конфигурация бэкенда.
type Config struct {
	Port        string
	DatabaseURL string // пусто → in-memory хранилище
	RedisURL    string // пусто → in-memory кэш
	// StorageStrict: true — падать, если PostgreSQL/Redis заданы, но недоступны;
	// false (по умолчанию) — откатиться на in-memory и продолжить работу.
	StorageStrict   bool
	ConnectAttempts int
	ConnectBackoff  time.Duration

	LLMAPIKey  string
	LLMBaseURL string
	LLMModel   string
	LLMTimeout time.Duration

	// SmartDelay — имитация задержки Smart Engine; <0 — значение из спецификации (300 мс).
	SmartDelay       time.Duration
	StreamChunkDelay time.Duration
	CORSOrigins      []string
}

// Getenv — функция чтения переменной окружения (os.Getenv в бою).
type Getenv func(string) string

func intEnv(get Getenv, key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(get(key))); err == nil {
		return v
	}
	return def
}

func boolEnv(get Getenv, key string) bool {
	switch strings.ToLower(strings.TrimSpace(get(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func strEnv(get Getenv, key, def string) string {
	if v := strings.TrimSpace(get(key)); v != "" {
		return v
	}
	return def
}

// Load собирает конфигурацию из окружения.
func Load(get Getenv) Config {
	c := Config{
		Port:             strEnv(get, "PORT", "8080"),
		DatabaseURL:      strings.TrimSpace(get("DATABASE_URL")),
		RedisURL:         strings.TrimSpace(get("REDIS_URL")),
		StorageStrict:    boolEnv(get, "STORAGE_STRICT"),
		ConnectAttempts:  intEnv(get, "STORAGE_CONNECT_ATTEMPTS", 10),
		ConnectBackoff:   time.Duration(intEnv(get, "STORAGE_CONNECT_BACKOFF_MS", 1000)) * time.Millisecond,
		LLMAPIKey:        strings.TrimSpace(get("LLM_API_KEY")),
		LLMBaseURL:       strings.TrimSpace(get("LLM_BASE_URL")),
		LLMModel:         strings.TrimSpace(get("LLM_MODEL")),
		LLMTimeout:       time.Duration(intEnv(get, "LLM_TIMEOUT_MS", 12000)) * time.Millisecond,
		SmartDelay:       time.Duration(intEnv(get, "SMART_DELAY_MS", -1)) * time.Millisecond,
		StreamChunkDelay: time.Duration(intEnv(get, "STREAM_CHUNK_DELAY_MS", 12)) * time.Millisecond,
	}
	for _, o := range strings.Split(strEnv(get, "CORS_ORIGINS", "*"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.CORSOrigins = append(c.CORSOrigins, o)
		}
	}
	if c.ConnectAttempts < 1 {
		c.ConnectAttempts = 1
	}
	return c
}

// LLMConfigured сообщает, заданы ли ключ и модель внешней LLM.
func (c Config) LLMConfigured() bool {
	return c.LLMAPIKey != "" && c.LLMModel != ""
}
