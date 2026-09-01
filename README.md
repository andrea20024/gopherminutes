# GoFerMinutes

CLI-утилита для записи, обработки и поиска материалов встреч.

## Возможности

- 🎤 **Загрузка аудио** — сохранение файлов в MongoDB GridFS
- 🤖 **Автоматическая обработка** — распознавание речи + LLM-суммаризация
- 🔍 **Полнотекстовый поиск** — по транскрипции и summary (GIN-индексы, russian)
- 📋 **Список встреч** — с фильтрацией по пользователю
- 🔄 **Retry** — повторная обработка failed-встреч
- 🗑 **Delete** — удаление встреч с каскадным удалением задач
- 💬 **Chat** — вопросы по материалам встреч через LLM
- ⚡ **Асинхронная обработка** — bounded concurrency (channel + semaphore, 3 workers)
- 🛡 **Graceful shutdown** — корректное завершение по SIGINT/SIGTERM
- 📝 **Structured logging** — zap logger с ключевыми событиями

## Архитектура

```
CLI (Cobra) ──→ MeetingService ──→ SpeechClient / LLMClient (mock)
                      │
                      ↓
                MeetingRepo ──→ PostgreSQL
                      │
                      ↓
                GridFSClient ──→ MongoDB (audio files)
```

### Слои

| Слой | Пакет | Назначение |
|------|-------|------------|
| CLI | `cmd/cli` | Cobra-команды, адаптеры |
| Service | `internal/service` | Бизнес-логика, async pipeline |
| Storage | `internal/storage` | Repository-слой (PostgreSQL) |
| Interfaces | `internal/interfaces` | SpeechClient, LLMClient |
| Speech | `internal/speech` | Mock + реальная реализация |
| AI | `internal/ai` | Mock + реальная реализация |
| Config | `internal/config` | Загрузка .env |
| Logger | `internal/logger` | Structured logging (zap) |
| Mongo | `internal/mongo` | GridFS клиент |

## Установка

### Требования

- Go 1.21+
- PostgreSQL 14+
- MongoDB 6.0+ (опционально, для GridFS)

### Сборка

```bash
go mod download
go build -o goferminutes ./cmd/cli/
```

### Запуск через Docker

Для удобства в проекте есть `docker-compose.yml` — поднимает PostgreSQL и MongoDB:

```bash
# Запустить сервисы
docker-compose up -d

# Проверить что всё работает
docker-compose ps

# Остановить (данные сохранятся)
docker-compose down
```

Параметры по умолчанию совпадают с `.env.example`:
- PostgreSQL: `localhost:5434`, `loader:1234`, база `truecode_db`
- MongoDB: `localhost:27017`, `admin:1234`

## Конфигурация

Скопируйте `.env.example` в `.env` и настройте:

```bash
cp .env.example .env
```

### Переменные

| Переменная | Описание | По умолчанию |
|------------|----------|--------------|
| `DATABASE_DSN` | PostgreSQL connection string | `postgres://loader:1234@localhost:5434/truecode_db?sslmode=disable` |
| `SPEECH_PROVIDER` | Распознавание речи: `mock` или `salute` | `mock` |
| `LLM_PROVIDER` | LLM: `mock` или `gigachat` | `mock` |
| `MONGODB_DSN` | MongoDB connection string | `mongodb://admin:1234@127.0.0.1:27017` |
| `MONGODB_DATABASE` | Имя MongoDB базы | `ContentStore` |
| `MONGODB_BUCKET` | Имя GridFS bucket | `Content` |

## Использование

### Базовые команды

Все команды принимают флаг `-u, --user-id` для указания ID пользователя (по умолчанию `1`).

```bash
# Регистрация пользователя
./goferminutes start -u 1

# Загрузка аудиофайла (асинхронная обработка)
./goferminutes load path/to/meeting.mp3 -u 1

# Список встреч
./goferminutes list -u 1

# Статус встречи
./goferminutes status <id> -u 1

# Получение транскрипции
./goferminutes get <id> -u 1

# Поиск по ключевому слову
./goferminutes find "проект" -u 1

# Вопрос по материалам
./goferminutes chat "О чём говорилось?" -u 1

# Повторная обработка failed-встречи
./goferminutes retry <id> -u 1

# Удаление встречи
./goferminutes delete <id> -u 1

# Скачивание аудио из GridFS
./goferminutes get-audio <id> -u 1
```

### Полный цикл

```bash
# 1. Регистрация
./goferminutes start -u 1

# 2. Загрузка аудио
./goferminutes load meeting.mp3 -u 1
# → Meeting created, async processing started

# 3. Проверка статуса
./goferminutes status <id> -u 1
# → Status: created → processing → transcribed → summarized → completed

# 4. Список всех встреч
./goferminutes list -u 1

# 5. Поиск по ключевому слову
./goferminutes find "проект" -u 1

# 6. Получение транскрипции
./goferminutes get <id> -u 1

# 7. Вопрос по материалам
./goferminutes chat "Какие были решения?" -u 1
```

## Обработка ошибок

### Жизненный цикл встречи

```
created → processing → transcribed → summarized → completed
                      ↓
                   failed (retry available)
```

### Статусы

| Статус | Описание |
|--------|----------|
| `created` | Встреча создана, обработка не начата |
| `processing` | Идёт распознавание речи |
| `transcribed` | Транскрипция готова |
| `summarized` | Summary сгенерирована |
| `completed` | Обработка завершена |
| `failed` | Ошибка обработки (доступен retry) |

### Retry

```bash
# Повторная обработка failed-встречи
./goferminutes retry <id> -u 1
# → Создаётся новая задача в meeting_tasks, встреча обрабатывается заново
```

## База данных

### Схема

```
users (1) ──< meetings (N) ──< meeting_tasks (N)
```

| Таблица | Поля | Описание |
|---------|------|----------|
| `users` | id, username, created_at | Пользователи |
| `meetings` | id, user_id, file_name, transcription, summary, gridfs_id, created_at, updated_at | Встречи |
| `meeting_tasks` | id, meeting_id, status, error_message, created_at, updated_at | Задачи обработки |

### Индексы

| Индекс | Тип | Назначение |
|--------|-----|------------|
| `idx_meetings_user_created` | B-tree | Фильтрация по user + сортировка |
| `idx_meeting_tasks_meeting_id` | B-tree | Связь с meeting |
| `idx_meeting_tasks_status` | B-tree | Фильтрация по статусу |
| `idx_meetings_transcription_fts` | GIN | Полнотекстовый поиск (transcription) |
| `idx_meetings_summary_fts` | GIN | Полнотекстовый поиск (summary) |

### Миграции

Миграции применяются автоматически при первом запуске. Директория: `migrations/`

| # | Файл | Описание |
|---|------|----------|
| 0001 | `0001_init.up.sql` | Создание таблиц: users, meetings, meeting_tasks |
| 0002 | `0002_add_fts_indexes.up.sql` | GIN-индексы для полнотекстового поиска |
| 0003 | `0003_remove_meeting_status.up.sql` | Удаление status из meetings (перенос в meeting_tasks) |
| 0004 | `0004_remove_meeting_error_message.up.sql` | Удаление error_message из meetings (перенос в meeting_tasks) |

## Тесты

### Unit-тесты

```bash
go test ./internal/service/... ./internal/cli/...
```

**Покрытие:** 100% бизнес-логики через MockMeetingRepo.

### Интеграционные тесты

```bash
$env:INTEGRATION_TEST="true"
go test ./internal/storage/... -tags integration
```

**Тесты:**
- `CreateMeetingWithTask` — транзакция: meeting + task
- `SaveTranscription` — сохранение + update status
- `SaveSummary` — сохранение + update status
- `SearchByKeyword` — полнотекстовый поиск
- `UserDataIsolation` — разграничение пользователей
- `FullLifecycle` — полный цикл: created → completed
- `FailedStatus` — обработка ошибок
- `ListMeetingsWithStatuses` — сортировка по created_at DESC

### Тесты конкурентности

```bash
go test ./internal/service/... -tags unit -run "Concurrent|Semaphore|Graceful|Context"
```

**Тесты:**
- `ConcurrentAccess` — 10 goroutines concurrently
- `SemaphoreLimit` — max 3 workers (bounded concurrency)
- `ContextCancellation` — slow-mock, context cancelled → no side effects
- `GracefulShutdown_NoGoroutineLeak` — корректное завершение
- `ShutdownStopsAcceptingTasks` — stop accepting + drain

### Тесты обработки ошибок

```bash
go test ./internal/service/... -tags unit -run "SpeechClientError|LLMClientError|Retry"
```

**Тесты:**
- Speech client error → status=failed
- LLM client error → status=failed after transcription
- Successful retry of failed meeting
- Retry rejected for non-failed meeting
- formatError — 11 test cases

## Ключевые решения

### Channel + Semaphore (bounded concurrency)

```go
taskQueue := make(chan *TaskContext, 100)
sem := make(chan struct{}, 3) // max 3 workers
```

- Очередь задач (capacity 100) — не блокирует CLI при загрузке
- Semaphore — ограничивает 3 параллельных worker'а
- Graceful shutdown: stop accepting → drain queue → close

### Mock-клиенты

```go
type SpeechClient interface {
    Recognize(ctx context.Context, data []byte, mime string) (string, error)
}

type LLMClient interface {
    Summarize(ctx context.Context, transcription string) (string, error)
    Ask(ctx context.Context, question string, contextText string) (string, error)
}
```

- Mock-реализации работают без внешних API
- SlowMock — для тестов cancellation/timeout
- CountingMock — для проверки semaphore (max concurrent calls)

### Lazy DB init

CLI не падает без PostgreSQL — подключение происходит при первой команде, требующей БД.

### Status в meeting_tasks

Статус хранится только в `meeting_tasks`, а не в `meetings`. Это позволяет:
- Отслеживать историю статусов
- Retry создаёт новую запись (а не перезаписывает старую)
- JOIN LATERAL для получения актуального статуса

## Структура проекта

```
cmd/cli/main.go              ← Точка входа, Cobra root command
internal/
├── cli/                     ← Cobra commands (start, load, list, ...)
├── service/                 ← Business logic (MeetingService)
├── storage/                 ← Repository layer (PostgreSQL)
├── interfaces/              ← SpeechClient, LLMClient interfaces
├── speech/                  ← Speech client implementations
├── ai/                      ← LLM client implementations
├── config/                  ← .env configuration
├── logger/                  ← Structured logging (zap)
├── mongo/                   ← MongoDB GridFS client
├── model/                   ← Data models
├── queue/                   ← Task queue (channel + semaphore)
└── handler/                 ← HTTP handlers (if needed)
migrations/                  ← SQL migrations (goose format)
.env.example                 ← Configuration template
```

## Лицензия

Учебный проект.
