FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./

RUN apk add --no-cache tzdata

RUN go mod download

COPY . .

RUN GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-$(go env GOARCH)}" CGO_ENABLED=0 \
    go build -ldflags="-s -w" -o ./cockpit ./cmd/cockpit/

FROM alpine:3.22.0

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=builder ./app/cockpit /cockpit/cockpit

WORKDIR /cockpit

EXPOSE 8080

ENV TZ=Asia/Shanghai \
    NACOS_CACHE_DIR=/tmp/nacos/cache

CMD ["./cockpit", "--host", "0.0.0.0", "--port", "8080"]
