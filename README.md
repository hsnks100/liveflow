# **Liveflow**

**Liveflow** is a flexible and modular live streaming solution designed for efficient real-time media stream handling. It supports a wide range of input and output formats, making it adaptable to various streaming needs.

## **Features**

- **Modular Architecture:** Each input and output module operates independently, allowing for easy extension and addition of new formats.
- **Real-Time Processing:** Optimized for processing live media streams with minimal latency.
- **Powered by FFmpeg:** Utilizes the robust FFmpeg library for comprehensive media processing capabilities.

## **Input and Output Formats**

|            | **HLS** | **WHEP** | **MKV** | **MP4** |
|------------|---------|----------|---------|---------|
| **RTMP**   |    ✅    |    ✅    |    ✅    |    ✅    |
| **WHIP**   |    ✅    |    ✅    |    ✅    |    ✅    |

The system architecture can be visualized as follows:

```
+-------+         +-------+
|       |         |       |
| RTMP  |         |       |
|Input  +-------> |       |
|       |         |  Hub  |
+-------+         |       +-----> Outputs
                  |       |       (WHEP,
+-------+         |       |       HLS, MP4, MKV)
|       |         |       |
| WHIP  +-------> |       |
|Input  |         |       |
+-------+         +-------+
```

## **Requirements**

- **FFmpeg:** Ensure FFmpeg is installed on your system as Liveflow relies on it for media processing.

## **Installation**

### **macOS**
1. Install FFmpeg:
   ```bash
   brew install ffmpeg # version 7
   ```
2. Clone and build the repository:
   ```bash
   git clone https://github.com/hsnks100/liveflow.git
   cd liveflow
   go build && ./liveflow 
   ```

### **Docker Compose**
1. Run Liveflow using Docker Compose:
   ```bash
   docker-compose up liveflow -d --force-recreate --build
   ```

## Building and Running

There are two ways to run the project: for production and for development.

### For Production
This method builds the frontend assets and embeds them into a single Go binary.

1.  **Build the frontend:**
    ```bash
    cd front
    npm install
    npm run build
    ```

2.  **Build and run the Go application:**
    ```bash
    cd .. 
    go build -o liveflow
    ./liveflow
    ```

3.  Access the application at `http://localhost:8044`.

### For Development
This method runs the Go backend and the Vite frontend development server separately, enabling hot-reloading for frontend changes.

1.  **Run the Go backend server:**
    Open a terminal and run:
    ```bash
    go build -o liveflow
    ./liveflow
    ```

2.  **Run the frontend development server:**
    Open a second terminal and run:
    ```bash
    cd front
    npm install
    npm run dev
    ```

3.  Access the application via the address shown by the Vite server (e.g., `http://localhost:5173`).

## **Usage**

Start streaming by choosing from the following broadcast options:

### **WHIP Broadcast (OBS >= 30.x)**
- **Server:** `http://127.0.0.1:8044/whip`
- **Bearer Token:** `test`

### **RTMP Broadcast**
- **Server:** `rtmp://127.0.0.1:1930/live`
- **Stream Key:** `test`

### **Stream Viewing Options**

- **HLS:**
    - URL: `http://127.0.0.1:8044/hls/test/master.m3u8`
    - Viewer: `http://127.0.0.1:8044/player/test`

- **WHEP:**
    - URL: `http://127.0.0.1:8044/wv`
    - Bearer Token: `test`
    - Click the **Subscribe** button.

- **MKV, MP4:**
    - **Docker:** `~/.store`
    - **Local:** `$(repo)/videos`

## **License**

This project is licensed under the MIT License. For more details, see the [LICENSE](LICENSE) file.

