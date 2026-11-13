#!/usr/bin/env bash
# Воспроизводимый отчёт покрытия бэкенда AI-Buddy.
#
#   ./scripts/coverage.sh            # нужен Go 1.26+ в PATH
#   docker compose --profile tests run --rm backend-tests   # без локального Go
#
# Результат (в backend/):
#   coverage.out   — профиль покрытия (go tool cover)
#   coverage.html  — HTML-отчёт по строкам
#   coverage.txt   — сводка по функциям и пакетам
#
# Считается весь прикладной код бэкенда (cmd + internal) без зависимостей и самих тестов.
# Скрипт завершается с ошибкой, если общее покрытие или покрытие ключевых пакетов
# (domain, usecase, mcp, rbac) ниже порога (по умолчанию 80%).
set -euo pipefail

THRESHOLD="${COVERAGE_THRESHOLD:-80}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT/backend"

echo "==> go test ./... (trimpath, covermode=atomic)"
# -trimpath: воспроизводимая сборка без локальных путей; -covermode=atomic: корректен при конкурентном доступе.
go test ./... -count=1 -trimpath -covermode=atomic -coverprofile=coverage.out | tee coverage.packages.tmp

go tool cover -html=coverage.out -o coverage.html
{
  echo "# Покрытие бэкенда AI-Buddy (go test ./... -trimpath -covermode=atomic)"
  echo "# Go: $(go version | awk '{print $3}')"
  echo
  echo "## По пакетам"
  grep -E '^(ok|FAIL|---)' coverage.packages.tmp || true
  echo
  echo "## По функциям"
  go tool cover -func=coverage.out
} > coverage.txt

TOTAL="$(go tool cover -func=coverage.out | awk '/^total:/ {gsub("%","",$3); print $3}')"
echo
echo "==> Общее покрытие строк: ${TOTAL}% (порог ${THRESHOLD}%)"

FAILED=0
below() { awk -v v="$1" -v t="$THRESHOLD" 'BEGIN { exit !(v + 0 < t + 0) }'; }
if below "$TOTAL"; then
  echo "ОШИБКА: общее покрытие ниже порога"
  FAILED=1
fi
for PKG in domain usecase mcp rbac; do
  PCT="$(awk -v p="/internal/${PKG}\t" 'index($0, p) { for (i = 1; i <= NF; i++) if ($i ~ /%$/) { gsub("%", "", $i); print $i } }' coverage.packages.tmp | head -1)"
  echo "    internal/${PKG}: ${PCT:-нет данных}%"
  if [ -z "${PCT}" ] || below "$PCT"; then
    echo "ОШИБКА: покрытие internal/${PKG} ниже порога"
    FAILED=1
  fi
done
rm -f coverage.packages.tmp

echo "==> Отчёты: backend/coverage.out, backend/coverage.html, backend/coverage.txt"
exit "$FAILED"
