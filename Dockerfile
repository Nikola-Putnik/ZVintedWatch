# syntax=docker/dockerfile:1
#
# Multi-stage build: web interface (Node) → static Go binary → minimal image.
#   docker build -t zvintedwatch .
#   docker run -d -p 8080:8080 -v ./data:/data zvintedwatch

FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN mkdir -p ../internal/web/dist && npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/Nikola-Putnik/ZVintedWatch/internal/version.Version=${VERSION}" \
      -o /out/zvintedwatch ./cmd/zvintedwatch \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/zvintedwatch /usr/local/bin/zvintedwatch
COPY --from=build --chown=65532:65532 /out/data /data
# Inside a container the interface must listen on all interfaces; a password
# is generated on first start and printed in the logs (docker logs ...).
ENV ZVW_LISTEN=0.0.0.0:8080 \
    ZVW_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/zvintedwatch"]
CMD ["run", "-open=false"]
