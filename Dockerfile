# syntax=docker/dockerfile:1

# 1. Build the SvelteKit frontend into web/dist (static, precompressed).
FROM node:24-alpine AS web
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts
COPY svelte.config.js vite.config.ts tsconfig.json ./
COPY src ./src
COPY static ./static
RUN npm run build

# 2. Compile a static Go binary with the frontend embedded.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kbo-review ./cmd/kbo-review \
 && mkdir -p /out/data

# 3. Minimal runtime: no shell, no package manager, non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/kbo-review /kbo-review
# Shipped owned by nonroot so a fresh named volume inherits writable permissions.
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV ADDR=0.0.0.0:8787 DATA_DIR=/data
VOLUME /data
EXPOSE 8787
USER nonroot
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD ["/kbo-review", "-check"]
ENTRYPOINT ["/kbo-review"]
