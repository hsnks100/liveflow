# Liveflow

**Liveflow** is a flexible and modular live streaming solution that supports various input and output formats. It is designed to handle real-time media streams efficiently, providing support for multiple input and output formats.

## Features

- **Modular Design:** Each input and output module is independent, making it easy to extend and add new formats.
- **Real-Time Processing:** Optimized for handling real-time media streams.
- **FFmpeg Dependency:** Leverages the power of the FFmpeg library for media processing.

## Input and Output Formats

|            | **HLS** | **WHEP** | **MKV** | **MP4** |
|------------|---------|----------|---------|---------|
| **RTMP**   |    ✔    |    ✔    |    ✔    |    ✔    |
| **WHIP**   |    ✔    |    ✔    |    ✔    |    ✔    |

## Requirements

- **FFmpeg**: This repository depends on FFmpeg. Please ensure FFmpeg is installed on your system.

## Installation

1. Clone the repository:
    ```bash
    git clone https://github.com/yourusername/liveflow.git
    cd liveflow
    ```

2. Install the necessary dependencies:
    ```bash
    make install-dependencies
    ```

3. Configure and build the project:
    ```bash
    make build
    ```

## Usage

To start processing a stream:

```bash
./liveflow --input rtmp://your.input.url --output hls://your.output.url
```


## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.
