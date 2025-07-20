# Frontend build stage
FROM node:20-bullseye as frontend-builder
WORKDIR /app/front
COPY front/package*.json ./
RUN npm ci
COPY front/ ./
RUN npm run build

# Backend build stage
FROM golang:1.23-bullseye
RUN apt-get update
RUN apt-get upgrade -y
RUN apt-get install -y build-essential git pkg-config libunistring-dev libaom-dev libdav1d-dev bzip2 nasm wget yasm ca-certificates

COPY install-ffmpeg.sh /install-ffmpeg.sh
RUN chmod +x /install-ffmpeg.sh && /install-ffmpeg.sh
ENV PKG_CONFIG_PATH=/ffmpeg_build/lib/pkgconfig:${PKG_CONFIG_PATH}
ENV PATH="/usr/local/go/bin:${PATH}"
WORKDIR /app

# Copy Go modules and download dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy all source code
COPY ./ /app

# Copy built frontend from frontend-builder stage
COPY --from=frontend-builder /app/front/dist ./front/dist

# Create videos directory
RUN mkdir -p /app/bin/videos

# Build Go application
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -o /app/liveflow

WORKDIR /app
ENV GOGC=10
ENTRYPOINT ["/app/liveflow"]
