// Package app собирает приложение: спецификации → RBAC → MCP → агент → usecase → HTTP.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"aibuddy/backend/internal/agent"
	"aibuddy/backend/internal/cache"
	"aibuddy/backend/internal/config"
	"aibuddy/backend/internal/domain"
	httpapi "aibuddy/backend/internal/http"
	"aibuddy/backend/internal/mcp"
	"aibuddy/backend/internal/rbac"
	"aibuddy/backend/internal/seed"
	"aibuddy/backend/internal/spec"
	"aibuddy/backend/internal/storage/memory"
	"aibuddy/backend/internal/storage/postgres"
	"aibuddy/backend/internal/usecase"
)

// App — собранное приложение.
type App struct {
	Handler http.Handler
	Service *usecase.Service
	Store   domain.Store
	Cache   domain.Cache
}

// Close освобождает ресурсы.
func (a *App) Close() error {
	return errors.Join(a.Store.Close(), a.Cache.Close())
}

// Overrides — подмены зависимостей для тестов.
type Overrides struct {
	Clock domain.Clock
	IDs   domain.IDGenerator
	LLM   agent.LLM
}

// retry повторяет подключение с паузой (PostgreSQL/Redis в docker compose стартуют не мгновенно).
func retry[T any](ctx context.Context, attempts int, backoff time.Duration, open func(context.Context) (T, error)) (T, error) {
	var zero T
	var err error
	for i := 0; i < attempts; i++ {
		var v T
		if v, err = open(ctx); err == nil {
			return v, nil
		}
		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return zero, err
}

func openStore(ctx context.Context, cfg config.Config, log *slog.Logger) (domain.Store, error) {
	if cfg.DatabaseURL == "" {
		log.Info("хранилище: in-memory (DATABASE_URL не задан)")
		return memory.New(), nil
	}
	store, err := retry(ctx, cfg.ConnectAttempts, cfg.ConnectBackoff, func(ctx context.Context) (*postgres.Store, error) {
		return postgres.Open(ctx, cfg.DatabaseURL)
	})
	if err == nil {
		log.Info("хранилище: PostgreSQL, миграции применены")
		return store, nil
	}
	if cfg.StorageStrict {
		return nil, fmt.Errorf("PostgreSQL недоступен (STORAGE_STRICT=true): %w", err)
	}
	log.Warn("PostgreSQL недоступен — откат на in-memory хранилище", "error", err.Error())
	return memory.New(), nil
}

func openCache(ctx context.Context, cfg config.Config, log *slog.Logger) (domain.Cache, error) {
	if cfg.RedisURL == "" {
		log.Info("кэш: in-memory (REDIS_URL не задан)")
		return cache.NewMemory(), nil
	}
	c, err := retry(ctx, cfg.ConnectAttempts, cfg.ConnectBackoff, func(ctx context.Context) (*cache.Redis, error) {
		return cache.OpenRedis(ctx, cfg.RedisURL)
	})
	if err == nil {
		log.Info("кэш: Redis")
		return c, nil
	}
	if cfg.StorageStrict {
		return nil, fmt.Errorf("Redis недоступен (STORAGE_STRICT=true): %w", err)
	}
	log.Warn("Redis недоступен — откат на in-memory кэш", "error", err.Error())
	return cache.NewMemory(), nil
}

// Build собирает приложение по конфигурации.
func Build(ctx context.Context, cfg config.Config, log *slog.Logger, o Overrides) (*App, error) {
	bundle, err := spec.Load()
	if err != nil {
		return nil, err
	}
	store, err := openStore(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	kv, err := openCache(ctx, cfg, log)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	application, err := Assemble(bundle, store, kv, cfg, log, o)
	if err != nil {
		_ = store.Close()
		_ = kv.Close()
		return nil, err
	}
	if err := application.Service.EnsureSeed(ctx); err != nil {
		_ = application.Close()
		return nil, fmt.Errorf("загрузка синтетических данных: %w", err)
	}
	return application, nil
}

// Assemble связывает компоненты поверх готовых хранилища и кэша.
func Assemble(bundle *spec.Bundle, store domain.Store, kv domain.Cache, cfg config.Config, log *slog.Logger, o Overrides) (*App, error) {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	clock, ids := o.Clock, o.IDs
	if clock == nil {
		clock = domain.SystemClock{}
	}
	if ids == nil {
		ids = domain.RandomIDs{}
	}
	data := seed.Data()

	registry := mcp.NewRegistry(bundle.Tools)
	services := mcp.NewServices(mcp.Deps{Store: store, Cache: kv, Clock: clock, IDs: ids, Seed: data})
	if err := services.BindAll(registry); err != nil {
		return nil, err
	}
	engine := rbac.New(bundle.RBAC)
	telemetry := mcp.NewTelemetry(store, clock, ids)
	gateway := mcp.NewGateway(registry, engine, telemetry, store, clock, ids)

	llm := o.LLM
	if llm == nil {
		if client := agent.NewOpenAIClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMTimeout); client != nil {
			llm = client
			log.Info("LLM подключена", "model", cfg.LLMModel)
		} else {
			log.Info("LLM не настроена — работает детерминированный Smart Engine")
		}
	}
	buddy := agent.New(agent.Options{
		Spec: &bundle.Agent, Tools: gateway, Catalog: registry, Flags: services.Flags,
		LLM: llm, Clock: clock, Delay: cfg.SmartDelay,
	})
	service := usecase.New(usecase.Deps{
		Spec: bundle, Store: store, Cache: kv, Directory: domain.NewDirectory(data.Employees),
		RBAC: engine, Gateway: gateway, Services: services, Agent: buddy, Events: telemetry,
		Clock: clock, IDs: ids, Seed: data, CronID: seed.CronID,
	})
	handler := httpapi.New(httpapi.Options{
		Service: service, Logger: log, CORSOrigins: cfg.CORSOrigins, StreamChunkDelay: cfg.StreamChunkDelay,
	})
	return &App{Handler: handler, Service: service, Store: store, Cache: kv}, nil
}
