# --- Этап 1: Сборка (Build Stage) ---
# Используем официальный образ Go как 'builder'
FROM golang:1.21-alpine AS builder

# Устанавливаем рабочую директорию внутри контейнера
WORKDIR /app

# Копируем файлы go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./

# Загружаем зависимости
RUN go mod download

# Копируем остальной исходный код
COPY . .

# Собираем приложение
# CGO_ENABLED=0 - отключает Cgo для статической линковки
# GOOS=linux - собираем для Linux (т.к. запускать будем в Alpine)
# -o /app/main - выходной бинарный файл
RUN CGO_ENABLED=0 GOOS=linux go build -v -o /app/main .

# --- Этап 2: Выполнение (Runtime Stage) ---
# Используем минимальный образ Alpine
FROM alpine:latest

# Устанавливаем рабочую директорию
WORKDIR /root/

# Копируем *только* собранный бинарный файл из 'builder'
COPY --from=builder /app/main .

# (Опционально, но рекомендуется) Добавляем ca-certificates для HTTPS запросов
RUN apk --no-cache add ca-certificates

# Команда, которая будет запущена при старте контейнера
CMD ["./main"]