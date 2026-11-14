package domain_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/seed"
	"aibuddy/backend/internal/storage/memory"
)

var now = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

func TestNewRecord(t *testing.T) {
	rec, err := domain.NewRecord(domain.KindMessage, "m1", "u1", now, map[string]string{"text": "привет"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != domain.KindMessage || rec.ID != "m1" || rec.OwnerID != "u1" || !strings.Contains(string(rec.Payload), "привет") {
		t.Fatalf("rec = %+v", rec)
	}
	if _, err := domain.NewRecord(domain.KindMessage, "", "u1", now, 1); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("пустой id: err = %v", err)
	}
	if _, err := domain.NewRecord(domain.KindMessage, "m2", "u1", now, make(chan int)); err == nil {
		t.Fatal("несериализуемое значение должно давать ошибку")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	grant := domain.AccessGrant{ID: "g1", InternID: "u1", Repository: "repo"}
	if err := domain.Save(ctx, store, domain.KindAccessGrant, grant.ID, "u1", now, grant); err != nil {
		t.Fatal(err)
	}
	if err := domain.Save(ctx, store, domain.KindAccessGrant, "", "u1", now, grant); err == nil {
		t.Fatal("Save с пустым id должен вернуть ошибку")
	}
	list, err := domain.Load[domain.AccessGrant](ctx, store, domain.KindAccessGrant, "u1")
	if err != nil || len(list) != 1 || list[0].Repository != "repo" {
		t.Fatalf("Load = %+v, %v", list, err)
	}
	one, err := domain.LoadOne[domain.AccessGrant](ctx, store, domain.KindAccessGrant, "g1")
	if err != nil || one.ID != "g1" {
		t.Fatalf("LoadOne = %+v, %v", one, err)
	}
	if _, err := domain.LoadOne[domain.AccessGrant](ctx, store, domain.KindAccessGrant, "нет"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ожидался ErrNotFound, получено %v", err)
	}

	// Повреждённый JSON в хранилище даёт понятную ошибку, а не панику.
	bad := domain.Record{Kind: domain.KindAccessGrant, ID: "bad", OwnerID: "u2", CreatedAt: now, Payload: []byte("{не json")}
	if err := store.Put(ctx, bad); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.Load[domain.AccessGrant](ctx, store, domain.KindAccessGrant, "u2"); err == nil {
		t.Fatal("Load должен вернуть ошибку десериализации")
	}
	if _, err := domain.LoadOne[domain.AccessGrant](ctx, store, domain.KindAccessGrant, "bad"); err == nil {
		t.Fatal("LoadOne должен вернуть ошибку десериализации")
	}
}

func TestChecklistPersistence(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	for _, it := range []domain.ChecklistItem{
		{ID: "b", Position: 2, Status: domain.StatusPending, UpdatedAt: now},
		{ID: "a", Position: 1, Status: domain.StatusCompleted, UpdatedAt: now},
	} {
		if err := domain.SaveChecklistItem(ctx, store, "u1", it); err != nil {
			t.Fatal(err)
		}
	}
	items, err := domain.LoadChecklist(ctx, store, "u1")
	if err != nil || len(items) != 2 || items[0].ID != "a" || items[1].ID != "b" {
		t.Fatalf("чек-лист должен быть отсортирован по позиции: %+v, %v", items, err)
	}
	if domain.ChecklistRecordID("u1", "a") != "u1:a" {
		t.Fatal("формат идентификатора записи чек-листа изменился")
	}
}

func TestSeedRecords(t *testing.T) {
	data := seed.Data()
	recs, err := data.Records(now)
	if err != nil {
		t.Fatal(err)
	}
	if want := 1 + len(data.Checklist) + 1 + len(data.Planner(now)); len(recs) != want {
		t.Fatalf("записей %d, want %d", len(recs), want)
	}
	kinds := map[domain.Kind]int{}
	for _, r := range recs {
		kinds[r.Kind]++
		if r.OwnerID != data.Intern.ID || !r.CreatedAt.Equal(now) {
			t.Fatalf("запись %s/%s: владелец или время неверны", r.Kind, r.ID)
		}
	}
	if kinds[domain.KindIntern] != 1 || kinds[domain.KindSprintTask] != 1 || kinds[domain.KindChecklist] != len(data.Checklist) {
		t.Fatalf("виды записей: %v", kinds)
	}

	broken := data
	broken.Intern.ID = ""
	if _, err := broken.Records(now); err == nil {
		t.Fatal("seed без id стажёра должен давать ошибку")
	}
}

func TestDirectory(t *testing.T) {
	dir := domain.NewDirectory(seed.Data().Employees)
	e, ok := dir.ByToken(seed.InternToken)
	if !ok || e.ID != seed.InternID || e.Role != domain.RoleInternBackend {
		t.Fatalf("ByToken = %+v, %v", e, ok)
	}
	if _, ok := dir.ByToken("нет-такого"); ok {
		t.Fatal("неизвестный токен не должен находиться")
	}
	if _, ok := dir.ByToken(""); ok {
		t.Fatal("сервисный аккаунт без токена не должен быть доступен по пустому токену")
	}
	if e, ok := dir.ByID(seed.CronID); !ok || e.Role != domain.RoleSystemCron {
		t.Fatalf("ByID(cron) = %+v, %v", e, ok)
	}
	if e, ok := dir.ByRole(domain.RoleMentor); !ok || e.ID != seed.MentorID {
		t.Fatalf("ByRole(mentor) = %+v, %v", e, ok)
	}
	if _, ok := dir.ByRole("ghost"); ok {
		t.Fatal("несуществующая роль")
	}
}

func TestClocksAndIDs(t *testing.T) {
	if (domain.FixedClock{T: now}).Now() != now {
		t.Fatal("FixedClock")
	}
	if time.Since(domain.SystemClock{}.Now()) > time.Minute {
		t.Fatal("SystemClock")
	}
	seq := &domain.SequentialIDs{}
	if a, b := seq.NewID("msg"), seq.NewID("msg"); a != "msg-1" || b != "msg-2" {
		t.Fatalf("SequentialIDs: %s, %s", a, b)
	}
	r := domain.RandomIDs{}
	a, b := r.NewID("evt"), r.NewID("evt")
	if a == b || !strings.HasPrefix(a, "evt-") || len(a) != len("evt-")+12 {
		t.Fatalf("RandomIDs: %s, %s", a, b)
	}
}
