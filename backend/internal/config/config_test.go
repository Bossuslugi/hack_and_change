package config

import (
	"reflect"
	"testing"
	"time"
)

func env(m map[string]string) Getenv {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c := Load(env(nil))
	if c.Port != "8080" || c.DatabaseURL != "" || c.RedisURL != "" || c.StorageStrict {
		t.Fatalf("значения по умолчанию: %+v", c)
	}
	if c.ConnectAttempts != 10 || c.ConnectBackoff != time.Second || c.LLMTimeout != 12*time.Second {
		t.Fatalf("таймауты по умолчанию: %+v", c)
	}
	if c.SmartDelay >= 0 {
		t.Fatalf("SmartDelay по умолчанию должен быть <0 (значение из спецификации): %v", c.SmartDelay)
	}
	if !reflect.DeepEqual(c.CORSOrigins, []string{"*"}) || c.StreamChunkDelay != 12*time.Millisecond {
		t.Fatalf("CORS/stream: %+v", c)
	}
	if c.LLMConfigured() {
		t.Fatal("без ключа LLM не настроена")
	}
}

func TestLoadOverrides(t *testing.T) {
	c := Load(env(map[string]string{
		"PORT": " 9000 ", "DATABASE_URL": "postgres://db", "REDIS_URL": "redis://cache",
		"STORAGE_STRICT": "TRUE", "STORAGE_CONNECT_ATTEMPTS": "0", "STORAGE_CONNECT_BACKOFF_MS": "50",
		"LLM_API_KEY": " key ", "LLM_BASE_URL": "https://llm.example/v1", "LLM_MODEL": "model-x", "LLM_TIMEOUT_MS": "3000",
		"SMART_DELAY_MS": "0", "STREAM_CHUNK_DELAY_MS": "не число",
		"CORS_ORIGINS": "http://localhost:5173, http://localhost:8080 ,",
	}))
	if c.Port != "9000" || c.DatabaseURL != "postgres://db" || c.RedisURL != "redis://cache" || !c.StorageStrict {
		t.Fatalf("%+v", c)
	}
	if c.ConnectAttempts != 1 {
		t.Fatalf("число попыток не может быть меньше 1: %d", c.ConnectAttempts)
	}
	if c.ConnectBackoff != 50*time.Millisecond || c.LLMTimeout != 3*time.Second || c.SmartDelay != 0 {
		t.Fatalf("%+v", c)
	}
	if c.StreamChunkDelay != 12*time.Millisecond {
		t.Fatalf("некорректное число должно заменяться значением по умолчанию: %v", c.StreamChunkDelay)
	}
	if !reflect.DeepEqual(c.CORSOrigins, []string{"http://localhost:5173", "http://localhost:8080"}) {
		t.Fatalf("CORS: %v", c.CORSOrigins)
	}
	if !c.LLMConfigured() || c.LLMAPIKey != "key" {
		t.Fatalf("LLM: %+v", c)
	}
}

func TestBoolEnv(t *testing.T) {
	for value, want := range map[string]bool{"1": true, "yes": true, "on": true, "true": true, "0": false, "": false, "нет": false} {
		if got := boolEnv(env(map[string]string{"K": value}), "K"); got != want {
			t.Errorf("boolEnv(%q) = %v, want %v", value, got, want)
		}
	}
}
