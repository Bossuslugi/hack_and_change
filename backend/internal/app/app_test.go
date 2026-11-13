package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"aibuddy/backend/internal/cache"
	"aibuddy/backend/internal/config"
	"aibuddy/backend/internal/seed"
	"aibuddy/backend/internal/spec"
	"aibuddy/backend/internal/storage/memory"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

const (
	deadPostgres = "postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"
	deadRedis    = "redis://127.0.0.1:1/0"
)

func fastCfg() config.Config {
	return config.Config{ConnectAttempts: 1, ConnectBackoff: time.Millisecond, SmartDelay: 0}
}

// Запуск без внешних зависимостей: in-memory fallback, seed загружен, API отвечает.
func TestBuildWithoutExternalDependencies(t *testing.T) {
	a, err := Build(context.Background(), fastCfg(), quiet, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.Store.Name() != "memory" || a.Cache.Name() != "memory" {
		t.Fatalf("ожидался in-memory fallback: %s/%s", a.Store.Name(), a.Cache.Name())
	}
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/workspace", nil)
	req.Header.Set("Authorization", "Bearer "+seed.InternToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Алексей Смирнов") || !strings.Contains(string(body), `"mode":"smart"`) {
		t.Fatalf("workspace: %d %s", resp.StatusCode, body)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}

// PostgreSQL и Redis заданы, но недоступны: нестрогий режим продолжает работу на in-memory.
func TestBuildFallsBackWhenStorageUnavailable(t *testing.T) {
	cfg := fastCfg()
	cfg.DatabaseURL, cfg.RedisURL = deadPostgres, deadRedis
	a, err := Build(context.Background(), cfg, quiet, Overrides{})
	if err != nil {
		t.Fatalf("нестрогий режим не должен падать: %v", err)
	}
	defer a.Close()
	if a.Store.Name() != "memory" || a.Cache.Name() != "memory" {
		t.Fatalf("fallback: %s/%s", a.Store.Name(), a.Cache.Name())
	}
}

func TestBuildStrictModeFails(t *testing.T) {
	cfg := fastCfg()
	cfg.StorageStrict = true
	cfg.DatabaseURL = deadPostgres
	if _, err := Build(context.Background(), cfg, quiet, Overrides{}); err == nil || !strings.Contains(err.Error(), "PostgreSQL недоступен") {
		t.Fatalf("строгий режим, PostgreSQL: %v", err)
	}
	cfg.DatabaseURL, cfg.RedisURL = "", deadRedis
	if _, err := Build(context.Background(), cfg, quiet, Overrides{}); err == nil || !strings.Contains(err.Error(), "Redis недоступен") {
		t.Fatalf("строгий режим, Redis: %v", err)
	}
}

func TestBuildWithRedisAndLLM(t *testing.T) {
	srv := miniredis.RunT(t)
	cfg := fastCfg()
	cfg.RedisURL = "redis://" + srv.Addr() + "/0"
	cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMBaseURL = "test-key", "test-model", "http://127.0.0.1:1/v1"
	a, err := Build(context.Background(), cfg, quiet, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.Cache.Name() != "redis" {
		t.Fatalf("кэш: %s", a.Cache.Name())
	}
	p, _ := a.Service.Authenticate(seed.InternToken)
	ws, err := a.Service.Workspace(context.Background(), p)
	if err != nil || ws.Engine.Mode != "llm" || ws.Engine.Model != "test-model" || ws.Storage.Cache != "redis" {
		t.Fatalf("режим LLM: %+v, %v", ws.Engine, err)
	}
	// LLM недоступна (порт закрыт) — сценарий всё равно проходит, флаги живут в Redis.
	res, err := a.Service.Chat(context.Background(), p, "Дай доступ к репозиторию avito-autoteka-api и тестовому стенду", nil)
	if err != nil || res.Message.Engine != "smart" || len(res.ChecklistUpdates) != 1 {
		t.Fatalf("fallback на Smart Engine: %+v, %v", res.Message, err)
	}
	if !srv.Exists("session:" + seed.InternID) {
		t.Fatal("контекст сессии должен кэшироваться в Redis")
	}
}

func TestAssembleDetectsSpecDrift(t *testing.T) {
	bundle := spec.MustLoad()
	bundle.Tools.Tools = append(bundle.Tools.Tools, spec.Tool{Name: "tracker.close_task", Server: "tracker"})
	if _, err := Assemble(bundle, memory.New(), cache.NewMemory(), fastCfg(), quiet, Overrides{}); err == nil {
		t.Fatal("инструмент спецификации без обработчика должен останавливать сборку")
	}
}

func TestRetry(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	v, err := retry(ctx, 5, time.Millisecond, func(context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", errors.New("ещё не готов")
		}
		return "ok", nil
	})
	if err != nil || v != "ok" || attempts != 3 {
		t.Fatalf("retry: %q, %v, попыток %d", v, err, attempts)
	}

	attempts = 0
	if _, err := retry(ctx, 2, time.Millisecond, func(context.Context) (int, error) {
		attempts++
		return 0, errors.New("недоступен")
	}); err == nil || attempts != 2 {
		t.Fatalf("исчерпание попыток: %v, попыток %d", err, attempts)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := retry(cancelled, 3, time.Hour, func(context.Context) (int, error) {
		return 0, errors.New("недоступен")
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("отмена контекста должна прерывать ожидание: %v", err)
	}
}
