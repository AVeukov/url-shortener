# URL Shortener

[![CI](https://github.com/AVeukov/url-shortener/actions/workflows/ci.yml/badge.svg)](https://github.com/AVeukov/url-shortener/actions/workflows/ci.yml)

Учебный URL shortener на Go с PostgreSQL, Redis-кэшем, Prometheus-метриками и готовым Grafana dashboard.

## Возможности

- создание случайной короткой ссылки или собственного alias;
- редирект по короткому коду;
- необязательный срок жизни ссылки до одного года;
- счётчик переходов;
- cache-aside через Redis с TTL;
- health и readiness endpoints;
- метрики Prometheus и автоматически подключаемый dashboard Grafana;
- JSON-логи и graceful shutdown;
- OpenAPI-описание API.

## Быстрый запуск

Понадобится Docker с поддержкой Compose.

```bash
docker compose up --build
```

После запуска доступны:

| Сервис | Адрес |
|---|---|
| API | http://localhost:8080 |
| Метрики | http://localhost:8080/metrics |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 |

Для Grafana используются логин и пароль `admin` / `admin`. Dashboard `URL Shortener` появляется автоматически.

## Примеры API

Создать случайную короткую ссылку:

```bash
curl -X POST http://localhost:8080/api/v1/urls \
  -H "Content-Type: application/json" \
  -d '{"url":"https://go.dev/doc/"}'
```

Создать ссылку с alias и сроком жизни один час:

```bash
curl -X POST http://localhost:8080/api/v1/urls \
  -H "Content-Type: application/json" \
  -d '{"url":"https://go.dev/","custom_alias":"golang-docs","expires_in_seconds":3600}'
```

Ответ:

```json
{
  "code": "golang-docs",
  "url": "https://go.dev/",
  "created_at": "2026-10-01T09:00:00Z",
  "expires_at": "2026-10-01T10:00:00Z",
  "click_count": 0,
  "short_url": "http://localhost:8080/golang-docs"
}
```

Перейти по ссылке:

```bash
curl -i http://localhost:8080/golang-docs
```

Получить статистику:

```bash
curl http://localhost:8080/api/v1/urls/golang-docs
```

Полный контракт находится в [openapi.yaml](openapi.yaml).

## Локальный запуск без Docker

Нужно самостоятельно запустить PostgreSQL и Redis, применить `migrations/001_init.sql`, затем настроить переменные из `.env.example` и выполнить:

```bash
go run ./cmd/api
```

Приложение не читает `.env` автоматически: переменные должны быть экспортированы в окружение процесса.

## Проверки

```bash
go test -race -cover ./...
go vet ./...
```

## Как работает кэш

При создании ссылка записывается в PostgreSQL и прогревается в Redis. При редиректе приложение сначала ищет код в Redis. В случае промаха оно читает PostgreSQL и сохраняет результат в Redis. TTL кэша не может пережить `expires_at` самой ссылки. Статистика всегда читается из PostgreSQL, поэтому счётчик переходов не бывает устаревшим из-за кэша.

## Структура

```text
cmd/api/                 точка входа
internal/httpapi/        HTTP-маршруты и middleware
internal/service/        бизнес-логика
internal/repository/     PostgreSQL
internal/cache/          Redis
internal/metrics/        Prometheus-метрики
migrations/              схема базы данных
deploy/                  настройки Prometheus и Grafana
```
