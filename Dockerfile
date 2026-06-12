# Stage 1: build SvelteKit frontend
FROM --platform=$BUILDPLATFORM node:22-alpine AS node-build
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: build Go binary (with embedded frontend)
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS go-build
WORKDIR /app/backend
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Embed the frontend build
COPY --from=node-build /app/frontend/build ./internal/static/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags="-s -w -X github.com/skua-app/skua/internal/version.Version=${VERSION}" \
    -trimpath \
    -o /server \
    ./cmd/server

# Stage 3: minimal runtime image
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go-build /server /server
EXPOSE 3200
USER nonroot
ENTRYPOINT ["/server"]
