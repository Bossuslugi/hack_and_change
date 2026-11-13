package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"aibuddy/backend/internal/domain"
)

// Обе реализации обязаны удовлетворять порту domain.Cache.
var (
	_ domain.Cache = (*Memory)(nil)
	_ domain.Cache = (*Redis)(nil)
)

// contract — общий контракт кэша, проверяется на обеих реализациях.
func contract(t *testing.T, c domain.Cache, advance func(time.Duration)) {
	t.Helper()
	ctx := context.Background()

	if _, ok, err := c.Get(ctx, "missing"); ok || err != nil {
		t.Fatalf("отсутствующий ключ: ok=%v err=%v", ok, err)
	}
	if err := c.Set(ctx, "ff:flag", "1", 0); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := c.Get(ctx, "ff:flag"); !ok || v != "1" || err != nil {
		t.Fatalf("Get = %q, %v, %v", v, ok, err)
	}
	if err := c.Set(ctx, "ff:flag", "0", -time.Second); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := c.Get(ctx, "ff:flag"); v != "0" {
		t.Fatalf("перезапись: %q", v)
	}

	// TTL: ключ сессии исчезает по истечении срока.
	if err := c.Set(ctx, "session:u1", "ctx", time.Minute); err != nil {
		t.Fatal(err)
	}
	advance(30 * time.Second)
	if _, ok, _ := c.Get(ctx, "session:u1"); !ok {
		t.Fatal("ключ истёк раньше срока")
	}
	advance(31 * time.Second)
	if _, ok, _ := c.Get(ctx, "session:u1"); ok {
		t.Fatal("ключ должен истечь по TTL")
	}

	_ = c.Set(ctx, "a", "1", 0)
	_ = c.Set(ctx, "b", "2", 0)
	if err := c.Del(ctx, "a", "b", "нет"); err != nil {
		t.Fatal(err)
	}
	if err := c.Del(ctx); err != nil {
		t.Fatalf("Del без ключей: %v", err)
	}
	if _, ok, _ := c.Get(ctx, "a"); ok {
		t.Fatal("ключ не удалён")
	}
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryContract(t *testing.T) {
	m := NewMemory()
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	m.SetClock(func() time.Time { return now })
	contract(t, m, func(d time.Duration) { now = now.Add(d) })
	if m.Name() != "memory" || m.Close() != nil {
		t.Fatal("служебные методы")
	}
}

func TestRedisContract(t *testing.T) {
	srv := miniredis.RunT(t)
	r, err := OpenRedis(context.Background(), "redis://"+srv.Addr()+"/0")
	if err != nil {
		t.Fatal(err)
	}
	contract(t, r, srv.FastForward)
	if r.Name() != "redis" {
		t.Fatal("Name")
	}

	// После остановки сервера операции возвращают ошибку, а не паникуют.
	srv.Close()
	ctx := context.Background()
	if _, _, err := r.Get(ctx, "k"); err == nil {
		t.Fatal("Get при недоступном Redis должен вернуть ошибку")
	}
	if err := r.Set(ctx, "k", "v", 0); err == nil {
		t.Fatal("Set при недоступном Redis должен вернуть ошибку")
	}
	if err := r.Del(ctx, "k"); err == nil {
		t.Fatal("Del при недоступном Redis должен вернуть ошибку")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRedisErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := OpenRedis(ctx, "не-url"); err == nil {
		t.Fatal("некорректный URL")
	}
	if _, err := OpenRedis(ctx, "redis://127.0.0.1:1/0"); err == nil {
		t.Fatal("недоступный Redis")
	}
}
