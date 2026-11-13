// Command server — точка входа бэкенда Avito Start AI-Buddy.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibuddy/backend/internal/app"
	"aibuddy/backend/internal/config"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, config.Load(os.Getenv), log, nil); err != nil {
		log.Error("сервер остановлен с ошибкой", "error", err.Error())
		os.Exit(1)
	}
}

// run собирает приложение и обслуживает HTTP до отмены контекста (graceful shutdown).
// ready, если задан, получает фактический адрес после начала прослушивания.
func run(ctx context.Context, cfg config.Config, log *slog.Logger, ready chan<- string) error {
	application, err := app.Build(ctx, cfg, log, app.Overrides{})
	if err != nil {
		return fmt.Errorf("не удалось запустить приложение: %w", err)
	}
	defer application.Close()

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return fmt.Errorf("не удалось занять порт %s: %w", cfg.Port, err)
	}
	srv := &http.Server{Handler: application.Handler, ReadHeaderTimeout: 5 * time.Second}

	stopped := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stopped <- srv.Shutdown(shutdownCtx)
	}()

	log.Info("AI-Buddy backend запущен", "addr", listener.Addr().String())
	if ready != nil {
		ready <- listener.Addr().String()
	}
	if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return <-stopped
}
