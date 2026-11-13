// Package postgres — реализация хранилища на PostgreSQL (database/sql + pgx).
// Каждый вид записи — отдельная таблица с индексируемыми колонками и JSONB-документом.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // драйвер "pgx" для database/sql

	"aibuddy/backend/internal/domain"
)

// Store — хранилище на PostgreSQL.
type Store struct {
	db *sql.DB
}

// Open подключается к PostgreSQL, проверяет соединение и применяет миграции.
func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: открытие: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxIdleTime(5 * time.Minute)
	s := New(db)
	if err := s.Ping(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: недоступен: %w", err)
	}
	if err := s.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// New оборачивает готовое соединение (используется в тестах с sqlmock).
func New(db *sql.DB) *Store { return &Store{db: db} }

// tables — допустимые таблицы (защита от подстановки имени таблицы).
var tables = func() map[domain.Kind]string {
	m := map[domain.Kind]string{}
	for _, k := range domain.AllKinds {
		m[k] = string(k)
	}
	return m
}()

func table(kind domain.Kind) (string, error) {
	t, ok := tables[kind]
	if !ok {
		return "", fmt.Errorf("postgres: неизвестный вид записи %q", kind)
	}
	return t, nil
}

// MigrationStatements возвращает DDL автоматической миграции (идемпотентный).
func MigrationStatements() []string {
	stmts := make([]string, 0, len(domain.AllKinds)*2+2)
	for _, k := range domain.AllKinds {
		t := string(k)
		stmts = append(stmts,
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	seq        BIGSERIAL,
	id         TEXT PRIMARY KEY,
	owner_id   TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	payload    JSONB NOT NULL
)`, t),
			fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_owner_seq_idx ON %s (owner_id, seq)`, t, t),
		)
	}
	// Аналитические индексы по статусам: чек-лист и заявки на отгул.
	stmts = append(stmts,
		`CREATE INDEX IF NOT EXISTS checklist_items_status_idx ON checklist_items ((payload->>'status'))`,
		`CREATE INDEX IF NOT EXISTS absence_requests_status_idx ON absence_requests ((payload->>'status'))`,
	)
	return stmts
}

// Migrate применяет миграции схемы.
func (s *Store) Migrate(ctx context.Context) error {
	for _, stmt := range MigrationStatements() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("postgres: миграция: %w", err)
		}
	}
	return nil
}

// Put создаёт или обновляет запись.
func (s *Store) Put(ctx context.Context, rec domain.Record) error {
	t, err := table(rec.Kind)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO `+t+` (id, owner_id, created_at, payload) VALUES ($1, $2, $3, $4::jsonb)
ON CONFLICT (id) DO UPDATE SET owner_id = EXCLUDED.owner_id, payload = EXCLUDED.payload`,
		rec.ID, rec.OwnerID, rec.CreatedAt, string(rec.Payload))
	if err != nil {
		return fmt.Errorf("postgres: запись %s/%s: %w", t, rec.ID, err)
	}
	return nil
}

// Get возвращает запись или domain.ErrNotFound.
func (s *Store) Get(ctx context.Context, kind domain.Kind, id string) (domain.Record, error) {
	t, err := table(kind)
	if err != nil {
		return domain.Record{}, err
	}
	rec := domain.Record{Kind: kind}
	var payload string
	err = s.db.QueryRowContext(ctx,
		`SELECT id, owner_id, created_at, payload::text FROM `+t+` WHERE id = $1`, id).
		Scan(&rec.ID, &rec.OwnerID, &rec.CreatedAt, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Record{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Record{}, fmt.Errorf("postgres: чтение %s/%s: %w", t, id, err)
	}
	rec.Payload = []byte(payload)
	return rec, nil
}

// List возвращает записи владельца в порядке вставки.
func (s *Store) List(ctx context.Context, kind domain.Kind, ownerID string) ([]domain.Record, error) {
	t, err := table(kind)
	if err != nil {
		return nil, err
	}
	query := `SELECT id, owner_id, created_at, payload::text FROM ` + t
	var args []any
	if ownerID != "" {
		query += ` WHERE owner_id = $1`
		args = append(args, ownerID)
	}
	query += ` ORDER BY seq`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: список %s: %w", t, err)
	}
	defer rows.Close()
	out := []domain.Record{}
	for rows.Next() {
		rec := domain.Record{Kind: kind}
		var payload string
		if err := rows.Scan(&rec.ID, &rec.OwnerID, &rec.CreatedAt, &payload); err != nil {
			return nil, fmt.Errorf("postgres: чтение строки %s: %w", t, err)
		}
		rec.Payload = []byte(payload)
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: обход %s: %w", t, err)
	}
	return out, nil
}

// Reset атомарно очищает все таблицы и загружает seed.
func (s *Store) Reset(ctx context.Context, seed []domain.Record) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres: начало транзакции: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	names := make([]string, 0, len(domain.AllKinds))
	for _, k := range domain.AllKinds {
		names = append(names, string(k))
	}
	if _, err := tx.ExecContext(ctx, `TRUNCATE TABLE `+strings.Join(names, ", ")+` RESTART IDENTITY`); err != nil {
		return fmt.Errorf("postgres: очистка: %w", err)
	}
	for _, rec := range seed {
		t, err := table(rec.Kind)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO `+t+` (id, owner_id, created_at, payload) VALUES ($1, $2, $3, $4::jsonb)`,
			rec.ID, rec.OwnerID, rec.CreatedAt, string(rec.Payload)); err != nil {
			return fmt.Errorf("postgres: seed %s/%s: %w", t, rec.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("postgres: фиксация seed: %w", err)
	}
	return nil
}

// Ping проверяет соединение.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Name возвращает имя реализации.
func (s *Store) Name() string { return "postgres" }

// Close закрывает пул соединений.
func (s *Store) Close() error { return s.db.Close() }
