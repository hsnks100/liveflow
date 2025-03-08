FROM golang:1.23-bullseye

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y build-essential git pkg-config libunistring-dev libaom-dev libdav1d-dev bzip2 nasm wget yasm ca-certificates

COPY install-ffmpeg-lsan.sh /install-ffmpeg-lsan.sh
RUN chmod +x /install-ffmpeg-lsan.sh && /install-ffmpeg-lsan.sh

ENV PKG_CONFIG_PATH=/ffmpeg_build/lib/pkgconfig:${PKG_CONFIG_PATH}
ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /app
COPY go.mod go.sum ./
ENV CGO_LDFLAGS='-fsanitize=address'
ENV CGO_CFLAGS='-fsanitize=address'
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY ./ /app
RUN mkdir -p /app/bin/videos
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -o /app
WORKDIR /app
ENV GOGC 10
ENV ASAN_OPTIONS quarantine_size_mb=32
ENTRYPOINT ["/app/liveflow"]