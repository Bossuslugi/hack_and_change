// Package memory — in-memory реализация хранилища.
// Используется как fallback без PostgreSQL и в unit-тестах (ноль внешних зависимостей).
package memory

import (
	"context"
	"sync"

	"aibuddy/backend/internal/domain"
)

// Store — потокобезопасное хранилище в памяти.
type Store struct {
	mu   sync.RWMutex
	data map[domain.Kind][]domain.Record
}

// New создаёт пустое хранилище.
func New() *Store {
	return &Store{data: map[domain.Kind][]domain.Record{}}
}

// Put создаёт или обновляет запись, сохраняя позицию при обновлении.
func (s *Store) Put(_ context.Context, rec domain.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec.Payload = append([]byte(nil), rec.Payload...)
	list := s.data[rec.Kind]
	for i := range list {
		if list[i].ID == rec.ID {
			rec.CreatedAt = list[i].CreatedAt
			list[i] = rec
			return nil
		}
	}
	s.data[rec.Kind] = append(list, rec)
	return nil
}

// Get возвращает запись или domain.ErrNotFound.
func (s *Store) Get(_ context.Context, kind domain.Kind, id string) (domain.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.data[kind] {
		if r.ID == id {
			return r, nil
		}
	}
	return domain.Record{}, domain.ErrNotFound
}

// List возвращает записи владельца в порядке вставки.
func (s *Store) List(_ context.Context, kind domain.Kind, ownerID string) ([]domain.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Record, 0, len(s.data[kind]))
	for _, r := range s.data[kind] {
		if ownerID == "" || r.OwnerID == ownerID {
			out = append(out, r)
		}
	}
	return out, nil
}

// Reset очищает хранилище и загружает seed.
func (s *Store) Reset(_ context.Context, seed []domain.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = map[domain.Kind][]domain.Record{}
	for _, r := range seed {
		s.data[r.Kind] = append(s.data[r.Kind], r)
	}
	return nil
}

// Ping всегда успешен.
func (s *Store) Ping(context.Context) error { return nil }

// Name возвращает имя реализации.
func (s *Store) Name() string { return "memory" }

// Close ничего не делает.
func (s *Store) Close() error { return nil }
