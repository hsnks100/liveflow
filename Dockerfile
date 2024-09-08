# ubuntu
#FROM ubuntu:latest
FROM golang:1.21-bullseye
RUN apt-get update
RUN apt-get upgrade -y
RUN apt-get install -y build-essential git pkg-config libunistring-dev libaom-dev libdav1d-dev bzip2 nasm wget yasm ca-certificates
COPY install-ffmpeg.sh /install-ffmpeg.sh
RUN chmod +x /install-ffmpeg.sh && /install-ffmpeg.sh
ENV PKG_CONFIG_PATH=/ffmpeg_build/lib/pkgconfig:${PKG_CONFIG_PATH}
ENV PATH="/usr/local/go/bin:${PATH}"
COPY ./ /app
WORKDIR /app
RUN ls .
RUN go mod download
RUN go build -o /app/bin/liveflow
RUN cp config.toml /app/bin/config.toml
RUN cp index.html /app/bin/index.html

RUN mkdir /app/bin/videos
WORKDIR /app/bin
ENTRYPOINT ["/app/bin/liveflow"]
