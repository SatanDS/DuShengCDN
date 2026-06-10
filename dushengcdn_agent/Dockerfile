ARG VERSION=dev

FROM golang:1.26.4-alpine AS builder

ARG VERSION
ARG TARGETOS=linux
ARG TARGETARCH

ENV CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH}

WORKDIR /build
COPY dushengcdn_server ./dushengcdn_server
COPY dushengcdn_agent ./dushengcdn_agent
WORKDIR /build/dushengcdn_agent
RUN go mod download
RUN go build -trimpath -buildvcs=false -ldflags "-s -w -X 'dushengcdn-agent/internal/config.AgentVersion=$VERSION'" -o /build/dushengcdn-agent ./cmd/agent

FROM openresty/openresty:alpine

RUN apk add --no-cache ca-certificates tzdata \
        libmaxminddb \
        libmaxminddb-dev \
        libcap \
        gcc \
        musl-dev \
        make \
        perl \
    && opm get anjia0532/lua-resty-maxminddb \
    && opm get ledgetech/lua-resty-http \
    && apk del gcc musl-dev make libmaxminddb-dev perl \
    && ln -sf /usr/lib/libmaxminddb.so.0 /usr/lib/libmaxminddb.so \
    && addgroup -S dushengcdn \
    && adduser -S -D -H -G dushengcdn -h /data dushengcdn \
    && mkdir -p /etc/dushengcdn /data /usr/local/openresty/nginx/logs /usr/local/openresty/nginx/client_body_temp /usr/local/openresty/nginx/proxy_temp /usr/local/openresty/nginx/fastcgi_temp /usr/local/openresty/nginx/uwsgi_temp /usr/local/openresty/nginx/scgi_temp \
    && chown -R dushengcdn:dushengcdn /etc/dushengcdn /data /usr/local/openresty/nginx

ENV DUSHENGCDN_OPENRESTY_PATH=openresty \
    DUSHENGCDN_DATA_DIR=/data \
    DUSHENGCDN_GEOIP_DATABASE_PATH=/data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb \
    DUSHENGCDN_OPENRESTY_GEOIP_DATABASE_PATH=/data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb

COPY --from=builder /build/dushengcdn-agent /usr/local/bin/dushengcdn-agent
RUN setcap cap_net_bind_service=+ep /usr/local/openresty/nginx/sbin/nginx \
    && setcap cap_net_bind_service=+ep /usr/local/bin/dushengcdn-agent

EXPOSE 80 443
USER dushengcdn
ENTRYPOINT ["/usr/local/bin/dushengcdn-agent"]
CMD ["-config", "/etc/dushengcdn/agent.json"]
