// Package cache — кэш контекста сессии и runtime feature flags:
// Redis в бою и in-memory fallback для тестов и запуска без зависимостей.
package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Memory — in-memory кэш с TTL.
type Memory struct {
	mu    sync.Mutex
	items map[string]memItem
	now   func() time.Time
}

type memItem struct {
	value   string
	expires time.Time
}

// NewMemory создаёт in-memory кэш.
func NewMemory() *Memory {
	return &Memory{items: map[string]memItem{}, now: time.Now}
}

// SetClock подменяет источник времени (для тестов TTL).
func (m *Memory) SetClock(now func() time.Time) { m.now = now }

// Get возвращает значение, если оно есть и не истекло.
func (m *Memory) Get(_ context.Context, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[key]
	if !ok {
		return "", false, nil
	}
	if !it.expires.IsZero() && !m.now().Before(it.expires) {
		delete(m.items, key)
		return "", false, nil
	}
	return it.value, true, nil
}

// Set сохраняет значение; ttl <= 0 — без срока жизни.
func (m *Memory) Set(_ context.Context, key, value string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	it := memItem{value: value}
	if ttl > 0 {
		it.expires = m.now().Add(ttl)
	}
	m.items[key] = it
	return nil
}

// Del удаляет ключи.
func (m *Memory) Del(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.items, k)
	}
	return nil
}

// Ping всегда успешен.
func (m *Memory) Ping(context.Context) error { return nil }

// Name возвращает имя реализации.
func (m *Memory) Name() string { return "memory" }

// Close ничего не делает.
func (m *Memory) Close() error { return nil }

// Redis — кэш на Redis.
type Redis struct {
	client *redis.Client
}

// OpenRedis подключается к Redis по URL (redis://host:port/db) и проверяет соединение.
func OpenRedis(ctx context.Context, url string) (*Redis, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: разбор URL: %w", err)
	}
	opts.DialTimeout = 2 * time.Second
	opts.ReadTimeout = time.Second
	opts.WriteTimeout = time.Second
	r := &Redis{client: redis.NewClient(opts)}
	if err := r.Ping(ctx); err != nil {
		_ = r.client.Close()
		return nil, fmt.Errorf("redis: недоступен: %w", err)
	}
	return r, nil
}

// Get возвращает значение ключа.
func (r *Redis) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("redis: get %s: %w", key, err)
	}
	return v, true, nil
}

// Set сохраняет значение с TTL (0 — бессрочно).
func (r *Redis) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if ttl < 0 {
		ttl = 0
	}
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis: set %s: %w", key, err)
	}
	return nil
}

// Del удаляет ключи.
func (r *Redis) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("redis: del: %w", err)
	}
	return nil
}

// Ping проверяет соединение.
func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }

// Name возвращает имя реализации.
func (r *Redis) Name() string { return "redis" }

// Close закрывает клиент.
func (r *Redis) Close() error { return r.client.Close() }
