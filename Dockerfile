# ---- Frontend: build the Vite SPA (gacha-nova, mounted as the `web` submodule) ----
FROM node:22-alpine AS frontend
WORKDIR /web
COPY web/package.json ./
RUN npm install
COPY web/ ./
# VITE_API_BASE_URL is left unset on purpose → the client defaults to "/api"
# (same-origin, since Go serves both). Pass the bot username for the browser
# Telegram Login Widget via build arg if needed.
ARG VITE_TG_BOT_USERNAME=""
ARG VITE_DISCORD_CLIENT_ID=""
ENV VITE_TG_BOT_USERNAME=$VITE_TG_BOT_USERNAME
ENV VITE_DISCORD_CLIENT_ID=$VITE_DISCORD_CLIENT_ID
RUN npm run build

# ---- Backend: build the Go binaries ----
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache tzdata
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o gachabot ./cmd/bot/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o seed ./cmd/seed/main.go

# ---- Final image ----
FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/gachabot .
COPY --from=builder /app/seed .
COPY --from=builder /app/locales ./locales
COPY --from=builder /app/assets ./assets
COPY --from=frontend /web/dist ./web

# Go serves the built SPA from here (same origin as /api).
ENV HTTP_STATIC_DIR=/root/web

CMD ["./gachabot"]
