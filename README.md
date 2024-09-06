# Liveflow

**Liveflow** is a flexible and modular live streaming solution that supports various input and output formats. It is designed to handle real-time media streams efficiently, providing support for multiple input and output formats.

## Features

- **Modular Design:** Each input and output module is independent, making it easy to extend and add new formats.
- **Real-Time Processing:** Optimized for handling real-time media streams.
- **FFmpeg Dependency:** Leverages the power of the FFmpeg library for media processing.

## Input and Output Formats

|            | **HLS** | **WHEP** | **MKV** | **MP4** |
|------------|---------|----------|---------|---------|
| **RTMP**   |    ✅    |    ✅    |    ✅    |    ✅    |
| **WHIP**   |    ✅    |    ✅    |    ✅    |    ✅    |



## Requirements

- **FFmpeg**: This repository depends on FFmpeg. Please ensure FFmpeg is installed on your system.

## Installation

0. Make sure you have FFmpeg installed on your system. You can install FFmpeg using the following commands:

### MAC
```bash
brew install ffmpeg # version 7
git clone https://github.com/hsnks100/liveflow.git
cd liveflow
go build && ./liveflow 
```
### Docker Compose
```bash
docker-compose up liveflow -d --force-recreate --build
```

## Usage

To start processing a stream:

You can select way to stream from the following options:

### WHIP broadcast
OBS 
- server: http://127.0.0.1:5555/whip
- bearer token: test

### RTMP broadcast
OBS
- server: rtmp://127.0.0.1:1930/live
- stream key: test

So, you can watch the stream from the following options:
### HLS 
- url: http://127.0.0.1:8044/hls/test/master.m3u8

### WHEP
- url: http://127.0.0.1:5555/ 
- bearer token: test
- click subscribe button.

### MKV, MP4
- docker: ~/.store
- local: $(repo)/videos

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.
