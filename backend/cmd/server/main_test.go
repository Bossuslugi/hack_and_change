package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"aibuddy/backend/internal/config"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// Сервер стартует без внешних зависимостей, отвечает на /healthz и корректно завершается.
func TestRunServesAndShutsDownGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready, done := make(chan string, 1), make(chan error, 1)
	go func() { done <- run(ctx, config.Config{Port: "0", ConnectAttempts: 1}, quiet, ready) }()

	var addr string
	select {
	case addr = <-ready:
	case err := <-done:
		t.Fatalf("сервер не стартовал: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("сервер не стартовал за 10 секунд")
	}
	port := addr[strings.LastIndex(addr, ":")+1:]
	resp, err := http.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("healthz: %d %s", resp.StatusCode, body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graceful shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("сервер не остановился")
	}
}

func TestRunFailsFast(t *testing.T) {
	// Строгий режим и недоступный PostgreSQL: запуск завершается понятной ошибкой.
	cfg := config.Config{
		Port: "0", StorageStrict: true, ConnectAttempts: 1,
		DatabaseURL: "postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1",
	}
	if err := run(context.Background(), cfg, quiet, nil); err == nil || !strings.Contains(err.Error(), "не удалось запустить") {
		t.Fatalf("err = %v", err)
	}
	// Некорректный порт.
	if err := run(context.Background(), config.Config{Port: "не-порт", ConnectAttempts: 1}, quiet, nil); err == nil || !strings.Contains(err.Error(), "порт") {
		t.Fatalf("err = %v", err)
	}
}

func TestHealthcheckHelper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready, done := make(chan string, 1), make(chan error, 1)
	go func() { done <- run(ctx, config.Config{Port: "0", ConnectAttempts: 1}, quiet, ready) }()

	var addr string
	select {
	case addr = <-ready:
	case err := <-done:
		t.Fatalf("сервер не стартовал: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("таймаут старта сервера")
	}
	port := addr[strings.LastIndex(addr, ":")+1:]
	resp, err := http.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthcheck check failed: %v", err)
	}
	resp.Body.Close()
	cancel()
	<-done
}
