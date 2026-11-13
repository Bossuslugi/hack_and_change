package postgres

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"aibuddy/backend/internal/domain"
)

var (
	now    = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	errSQL = errors.New("connection reset")
)

func newMock(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("не все ожидания SQL выполнены: %v", err)
		}
	})
	return New(db), mock
}

func q(sql string) string { return regexp.QuoteMeta(sql) }

func TestMigrationStatements(t *testing.T) {
	stmts := MigrationStatements()
	if want := len(domain.AllKinds)*2 + 2; len(stmts) != want {
		t.Fatalf("операторов %d, want %d", len(stmts), want)
	}
	joined := strings.Join(stmts, "\n")
	for _, table := range []string{"interns", "checklist_items", "chat_messages", "mcp_tool_calls", "absence_requests", "telemetry_events"} {
		if !strings.Contains(joined, "CREATE TABLE IF NOT EXISTS "+table+" ") {
			t.Errorf("нет таблицы %s", table)
		}
	}
	for _, s := range stmts {
		if !strings.Contains(s, "IF NOT EXISTS") {
			t.Errorf("миграция должна быть идемпотентной: %s", s)
		}
	}
}

func TestMigrate(t *testing.T) {
	s, mock := newMock(t)
	for range MigrationStatements() {
		mock.ExpectExec("CREATE").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	s2, mock2 := newMock(t)
	mock2.ExpectExec("CREATE").WillReturnError(errSQL)
	if err := s2.Migrate(context.Background()); !errors.Is(err, errSQL) {
		t.Fatalf("ошибка миграции должна пробрасываться: %v", err)
	}
}

func TestPut(t *testing.T) {
	ctx := context.Background()
	rec := domain.Record{Kind: domain.KindAbsence, ID: "abs-1", OwnerID: "u1", CreatedAt: now, Payload: []byte(`{"number":"#ABS-2026-84"}`)}

	s, mock := newMock(t)
	mock.ExpectExec(q("INSERT INTO absence_requests")).
		WithArgs("abs-1", "u1", now, `{"number":"#ABS-2026-84"}`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := s.Put(ctx, rec); err != nil {
		t.Fatal(err)
	}

	mock.ExpectExec(q("INSERT INTO absence_requests")).WillReturnError(errSQL)
	if err := s.Put(ctx, rec); !errors.Is(err, errSQL) {
		t.Fatalf("err = %v", err)
	}

	rec.Kind = "users; DROP TABLE interns"
	if err := s.Put(ctx, rec); err == nil || !strings.Contains(err.Error(), "неизвестный вид") {
		t.Fatalf("произвольное имя таблицы должно отклоняться: %v", err)
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	s, mock := newMock(t)
	cols := []string{"id", "owner_id", "created_at", "payload"}

	mock.ExpectQuery(q("FROM chat_messages WHERE id = $1")).WithArgs("m1").
		WillReturnRows(sqlmock.NewRows(cols).AddRow("m1", "u1", now, `{"text":"привет"}`))
	rec, err := s.Get(ctx, domain.KindMessage, "m1")
	if err != nil || rec.ID != "m1" || rec.OwnerID != "u1" || rec.Kind != domain.KindMessage || string(rec.Payload) != `{"text":"привет"}` {
		t.Fatalf("rec = %+v, %v", rec, err)
	}

	mock.ExpectQuery(q("FROM chat_messages WHERE id = $1")).WithArgs("нет").WillReturnRows(sqlmock.NewRows(cols))
	if _, err := s.Get(ctx, domain.KindMessage, "нет"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ожидался ErrNotFound: %v", err)
	}

	mock.ExpectQuery(q("FROM chat_messages WHERE id = $1")).WillReturnError(errSQL)
	if _, err := s.Get(ctx, domain.KindMessage, "m1"); !errors.Is(err, errSQL) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Get(ctx, "ghost", "m1"); err == nil {
		t.Fatal("неизвестный вид записи")
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	s, mock := newMock(t)
	cols := []string{"id", "owner_id", "created_at", "payload"}

	mock.ExpectQuery(q("FROM telemetry_events WHERE owner_id = $1 ORDER BY seq")).WithArgs("u1").
		WillReturnRows(sqlmock.NewRows(cols).AddRow("e1", "u1", now, `{"n":1}`).AddRow("e2", "u1", now, `{"n":2}`))
	list, err := s.List(ctx, domain.KindEvent, "u1")
	if err != nil || len(list) != 2 || list[1].ID != "e2" || string(list[0].Payload) != `{"n":1}` {
		t.Fatalf("list = %+v, %v", list, err)
	}

	// Без владельца — все записи вида, без WHERE.
	mock.ExpectQuery(q("FROM telemetry_events ORDER BY seq")).WillReturnRows(sqlmock.NewRows(cols))
	list, err = s.List(ctx, domain.KindEvent, "")
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("пустой список должен быть не nil: %+v, %v", list, err)
	}

	mock.ExpectQuery(q("FROM telemetry_events")).WillReturnError(errSQL)
	if _, err := s.List(ctx, domain.KindEvent, "u1"); !errors.Is(err, errSQL) {
		t.Fatalf("err = %v", err)
	}

	mock.ExpectQuery(q("FROM telemetry_events")).
		WillReturnRows(sqlmock.NewRows(cols).AddRow("e1", "u1", "не-время", `{}`))
	if _, err := s.List(ctx, domain.KindEvent, "u1"); err == nil {
		t.Fatal("ошибка Scan должна пробрасываться")
	}

	mock.ExpectQuery(q("FROM telemetry_events")).
		WillReturnRows(sqlmock.NewRows(cols).AddRow("e1", "u1", now, `{}`).RowError(0, errSQL))
	if _, err := s.List(ctx, domain.KindEvent, "u1"); !errors.Is(err, errSQL) {
		t.Fatalf("ошибка обхода строк должна пробрасываться: %v", err)
	}
	if _, err := s.List(ctx, "ghost", ""); err == nil {
		t.Fatal("неизвестный вид записи")
	}
}

func TestReset(t *testing.T) {
	ctx := context.Background()
	seed := []domain.Record{
		{Kind: domain.KindIntern, ID: "u1", OwnerID: "u1", CreatedAt: now, Payload: []byte(`{"id":"u1"}`)},
		{Kind: domain.KindChecklist, ID: "u1:a", OwnerID: "u1", CreatedAt: now, Payload: []byte(`{"id":"a"}`)},
	}

	t.Run("успех", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectExec(q("TRUNCATE TABLE interns, checklist_items")).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(q("INSERT INTO interns")).WithArgs("u1", "u1", now, `{"id":"u1"}`).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(q("INSERT INTO checklist_items")).WithArgs("u1:a", "u1", now, `{"id":"a"}`).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		if err := s.Reset(ctx, seed); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("ошибка начала транзакции", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin().WillReturnError(errSQL)
		if err := s.Reset(ctx, seed); !errors.Is(err, errSQL) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("ошибка очистки откатывает транзакцию", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("TRUNCATE").WillReturnError(errSQL)
		mock.ExpectRollback()
		if err := s.Reset(ctx, seed); !errors.Is(err, errSQL) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("ошибка вставки откатывает транзакцию", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("TRUNCATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(q("INSERT INTO interns")).WillReturnError(errSQL)
		mock.ExpectRollback()
		if err := s.Reset(ctx, seed); !errors.Is(err, errSQL) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("неизвестный вид в seed", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("TRUNCATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		if err := s.Reset(ctx, []domain.Record{{Kind: "ghost", ID: "x"}}); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
	t.Run("ошибка фиксации", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("TRUNCATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit().WillReturnError(errSQL)
		if err := s.Reset(ctx, nil); !errors.Is(err, errSQL) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPingNameClose(t *testing.T) {
	s, mock := newMock(t)
	mock.ExpectPing()
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Name() != "postgres" {
		t.Fatal("Name")
	}
	mock.ExpectClose()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Порт 1 закрыт: Open должен вернуть ошибку недоступности, а не зависнуть.
	if _, err := Open(ctx, "postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"); err == nil || !strings.Contains(err.Error(), "недоступен") {
		t.Fatalf("err = %v", err)
	}
	if _, err := Open(ctx, "://не-dsn"); err == nil {
		t.Fatal("некорректный DSN должен давать ошибку")
	}
}
