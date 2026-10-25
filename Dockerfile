FROM golang:1.27-bookworm AS builder

WORKDIR /app

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    gcc \
    zlib1g-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go run github.com/AshokShau/gotdbot/scripts/tools
RUN go run setup_ntgcalls.go

RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o main .

FROM debian:12-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -r app && useradd -r -g app -m -d /home/app app

ENV HOME="/home/app"

COPY --from=builder --chown=app:app /app/main /usr/local/bin/app
COPY --from=builder --chown=app:app /app/libtdjson.so.* /home/app/

USER app

WORKDIR /home/app
ENTRYPOINT ["app"]
