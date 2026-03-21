FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o ./cockpit ./cmd/cockpit/

FROM alpine:3.22.0

RUN apk add --no-cache tzdata

RUN mkdir -p /cockpit /tmp/nacos/cache /tmp/nacos/log

COPY --from=builder ./app/cockpit /cockpit/cockpit

WORKDIR /cockpit

EXPOSE 8317

ENV TZ=Asia/Shanghai \
    NACOS_CACHE_DIR=/tmp/nacos/cache

RUN cp /usr/share/zoneinfo/${TZ} /etc/localtime && echo "${TZ}" > /etc/timezone

CMD ["./cockpit"]
