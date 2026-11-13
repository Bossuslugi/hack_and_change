# Бэкенд AI-Buddy. Контекст сборки — корень репозитория (go.mod и /specs лежат там):
#   docker build -f backend/Dockerfile .
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# SDD-спецификации вкомпилируются в бинарник (specs/embed.go).
COPY specs ./specs
COPY backend ./backend
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./backend/cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget ca-certificates
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/server /server
ENV PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=10 \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["/server"]
