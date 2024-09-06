#!/bin/sh

set -ex

# Create a directory for the build
mkdir -p /ffmpeg_build
cd /ffmpeg_build

git config --global http.sslVerify false
# Download and compile x264
git clone --depth 1 https://code.videolan.org/videolan/x264.git
cd x264
./configure --prefix="/ffmpeg_build" --enable-static --disable-opencl
make
make install
cd ..

# Download and extract FFmpeg
wget --no-check-certificate -O ffmpeg-7.0.1.tar.bz2 https://ffmpeg.org/releases/ffmpeg-7.0.1.tar.bz2
tar xjf ffmpeg-7.0.1.tar.bz2
cd ffmpeg-7.0.1

# Configure and compile FFmpeg with x264
PKG_CONFIG_PATH="/ffmpeg_build/lib/pkgconfig" ./configure \
  --prefix="/ffmpeg_build" \
  --pkg-config-flags="--static" \
  --extra-cflags="-I/ffmpeg_build/include" \
  --extra-ldflags="-L/ffmpeg_build/lib" \
  --extra-libs="-lpthread -lm" \
  --bindir="/usr/local/bin" \
  --enable-gpl \
  --enable-libx264 \
  --enable-nonfree
make -j8
make install
cd ..

# Clean up
rm -rf /ffmpeg_build/src /ffmpeg_build/*.tar.bz2

echo "FFmpeg 7.0.1 with x264 has been successfully installed to /ffmpeg_build."