# Этап 1: Сборка
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Копируем файлы модулей и скачиваем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь остальной код (включая папку locales)
COPY . .

# Компилируем приложение (отключаем CGO для работы в Alpine)
RUN CGO_ENABLED=0 GOOS=linux go build -o gachabot ./cmd/bot/main.go

# Этап 2: Финальный минималистичный образ
FROM alpine:latest

WORKDIR /root/

# Копируем скомпилированный бинарник из первого этапа
COPY --from=builder /app/gachabot .
# КОПИРУЕМ ПАПКУ СО СЛОВАРЯМИ ИЗ ПЕРВОГО ЭТАПА
COPY --from=builder /app/locales ./locales

# Запускаем бота
CMD ["./gachabot"]