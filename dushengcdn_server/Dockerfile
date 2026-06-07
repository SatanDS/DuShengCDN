ARG VERSION=dev

FROM node:20 AS builder

ARG VERSION

WORKDIR /build
COPY ./web/package.json ./
COPY ./web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY ./web ./
RUN NEXT_PUBLIC_APP_VERSION="$VERSION" pnpm build

FROM golang:1.26.4 AS builder2

ARG VERSION

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=builder /build/build ./web/build
RUN go build -trimpath -buildvcs=false -ldflags "-s -w -X 'dushengcdn/common.Version=$VERSION'" -o dushengcdn

FROM alpine

RUN apk update \
    && apk upgrade \
    && apk add --no-cache ca-certificates tzdata \
    && update-ca-certificates 2>/dev/null || true
ENV PORT=3000
COPY --from=builder2 /build/dushengcdn /
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/dushengcdn"]
