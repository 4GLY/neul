# syntax=docker/dockerfile:1.7

FROM node:24-alpine AS web-build
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@11.5.0 --activate
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web ./
RUN pnpm build

FROM golang:1.25-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
ARG TARGETOS=linux
ARG TARGETARCH=arm64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go test ./... && \
	go build -trimpath -ldflags="-s -w" -o /out/neul-server ./cmd/neul-server

FROM alpine:3.22
RUN addgroup -S -g 65532 neul && \
	adduser -S -D -H -h /home/neul -s /sbin/nologin -G neul -u 65532 neul && \
	mkdir -p /app/web/dist /data /home/neul && \
	chown -R 65532:65532 /app /data /home/neul
COPY --from=go-build /out/neul-server /usr/local/bin/neul-server
COPY --from=web-build /src/web/dist /app/web/dist
USER 65532:65532
ENV NEUL_ADDR=0.0.0.0:8080
ENV NEUL_DB=/data/neul.sqlite
ENV NEUL_STATIC_DIR=/app/web/dist
ENV NEUL_HOME_DIR=/home/neul
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/neul-server"]
