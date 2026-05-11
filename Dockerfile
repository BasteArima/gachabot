FROM golang:1.25-alpine AS builder

RUN apk add --no-cache tzdata
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o gachabot ./cmd/bot/main.go

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/gachabot .
COPY --from=builder /app/locales ./locales

CMD ["./gachabot"]