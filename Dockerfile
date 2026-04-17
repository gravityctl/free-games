# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -buildvcs=false -o free-games .

# Final stage
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/free-games .
COPY .env.example ./

RUN mkdir -p /data && chown -R nobody:nobody /data

USER nobody

EXPOSE 8080

ENTRYPOINT ["./free-games"]
