# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/story-ai .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app

COPY --from=builder --chown=app:app /out/story-ai ./story-ai
COPY --chown=app:app static/ ./static/
COPY --chown=app:app data.db ./data.db

ENV PORT=9779

EXPOSE 9779

USER app

ENTRYPOINT ["/app/story-ai"]
