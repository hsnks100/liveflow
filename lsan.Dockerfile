FROM golang:1.21-bullseye

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y build-essential git pkg-config libunistring-dev libaom-dev libdav1d-dev bzip2 nasm wget yasm ca-certificates

COPY install-ffmpeg-lsan.sh /install-ffmpeg-lsan.sh
RUN chmod +x /install-ffmpeg-lsan.sh && /install-ffmpeg-lsan.sh

ENV PKG_CONFIG_PATH=/ffmpeg_build/lib/pkgconfig:${PKG_CONFIG_PATH}
ENV PATH="/usr/local/go/bin:${PATH}"

COPY ./ /app
WORKDIR /app

ENV CGO_LDFLAGS='-fsanitize=address'
ENV CGO_CFLAGS='-fsanitize=address'
RUN go mod download
RUN go build -o /app/bin/liveflow
RUN cp config.toml /app/bin/config.toml
RUN cp -r static /app/bin/static
RUN mkdir -p /app/bin/videos
WORKDIR /app/bin
ENTRYPOINT ["/app/bin/liveflow"]