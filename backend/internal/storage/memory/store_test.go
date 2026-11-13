package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"aibuddy/backend/internal/domain"
)

var now = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

func rec(kind domain.Kind, id, owner, payload string) domain.Record {
	return domain.Record{Kind: kind, ID: id, OwnerID: owner, CreatedAt: now, Payload: []byte(payload)}
}

func TestPutGetList(t *testing.T) {
	ctx := context.Background()
	s := New()
	for _, r := range []domain.Record{
		rec(domain.KindMessage, "m1", "u1", `{"n":1}`),
		rec(domain.KindMessage, "m2", "u2", `{"n":2}`),
		rec(domain.KindMessage, "m3", "u1", `{"n":3}`),
		rec(domain.KindEvent, "m1", "u1", `{"other":true}`),
	} {
		if err := s.Put(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Get(ctx, domain.KindMessage, "m2")
	if err != nil || string(got.Payload) != `{"n":2}` {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if _, err := s.Get(ctx, domain.KindMessage, "нет"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ожидался ErrNotFound: %v", err)
	}
	if _, err := s.Get(ctx, domain.KindBrief, "m1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("виды записей должны быть изолированы: %v", err)
	}

	cases := []struct {
		owner string
		want  []string
	}{
		{"u1", []string{"m1", "m3"}},
		{"u2", []string{"m2"}},
		{"", []string{"m1", "m2", "m3"}},
		{"никто", nil},
	}
	for _, tc := range cases {
		list, err := s.List(ctx, domain.KindMessage, tc.owner)
		if err != nil || len(list) != len(tc.want) {
			t.Fatalf("List(%q) = %d записей, %v; want %d", tc.owner, len(list), err, len(tc.want))
		}
		for i, id := range tc.want {
			if list[i].ID != id {
				t.Fatalf("List(%q)[%d] = %s, want %s (порядок вставки)", tc.owner, i, list[i].ID, id)
			}
		}
	}
}

func TestPutIsUpsertKeepingOrderAndCreatedAt(t *testing.T) {
	ctx := context.Background()
	s := New()
	_ = s.Put(ctx, rec(domain.KindMessage, "m1", "u1", `{"v":1}`))
	_ = s.Put(ctx, rec(domain.KindMessage, "m2", "u1", `{"v":2}`))
	updated := rec(domain.KindMessage, "m1", "u1", `{"v":"new"}`)
	updated.CreatedAt = now.Add(time.Hour)
	if err := s.Put(ctx, updated); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(ctx, domain.KindMessage, "u1")
	if len(list) != 2 || list[0].ID != "m1" || string(list[0].Payload) != `{"v":"new"}` {
		t.Fatalf("обновление должно сохранять позицию: %+v", list)
	}
	if !list[0].CreatedAt.Equal(now) {
		t.Fatalf("created_at не должен меняться при обновлении: %v", list[0].CreatedAt)
	}
}

func TestPutCopiesPayload(t *testing.T) {
	ctx := context.Background()
	s := New()
	payload := []byte(`{"v":1}`)
	_ = s.Put(ctx, domain.Record{Kind: domain.KindMessage, ID: "m1", OwnerID: "u1", Payload: payload})
	payload[2] = 'X'
	got, _ := s.Get(ctx, domain.KindMessage, "m1")
	if string(got.Payload) != `{"v":1}` {
		t.Fatalf("хранилище должно копировать payload: %s", got.Payload)
	}
}

func TestResetAndMeta(t *testing.T) {
	ctx := context.Background()
	s := New()
	_ = s.Put(ctx, rec(domain.KindMessage, "old", "u1", `{}`))
	seed := []domain.Record{rec(domain.KindIntern, "u1", "u1", `{}`), rec(domain.KindChecklist, "u1:a", "u1", `{}`)}
	if err := s.Reset(ctx, seed); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(ctx, domain.KindMessage, ""); len(list) != 0 {
		t.Fatal("Reset должен очищать старые данные")
	}
	if _, err := s.Get(ctx, domain.KindChecklist, "u1:a"); err != nil {
		t.Fatalf("seed не загружен: %v", err)
	}
	if s.Name() != "memory" || s.Ping(ctx) != nil || s.Close() != nil {
		t.Fatal("служебные методы")
	}
}

func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	s := New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.Put(ctx, rec(domain.KindEvent, fmt.Sprintf("e%d", i), "u1", `{}`))
			_, _ = s.List(ctx, domain.KindEvent, "u1")
		}(i)
	}
	wg.Wait()
	if list, _ := s.List(ctx, domain.KindEvent, "u1"); len(list) != 50 {
		t.Fatalf("записей %d, want 50", len(list))
	}
}
