#!/bin/sh

set -ex

# Leak Sanitizer flags
export SANITIZE_FLAGS="-fsanitize=address -g -O1"

mkdir -p /ffmpeg_build
cd /ffmpeg_build

git config --global http.sslVerify false

git clone --depth 1 https://code.videolan.org/videolan/x264.git
cd x264
./configure --prefix="/ffmpeg_build" --enable-static --disable-opencl LDFLAGS="$SANITIZE_FLAGS"
make
make install
cd ..

wget --no-check-certificate -O ffmpeg-7.0.1.tar.bz2 https://ffmpeg.org/releases/ffmpeg-7.0.1.tar.bz2
tar xjf ffmpeg-7.0.1.tar.bz2
cd ffmpeg-7.0.1

PKG_CONFIG_PATH="/ffmpeg_build/lib/pkgconfig" ./configure \
  --prefix="/ffmpeg_build" \
  --pkg-config-flags="--static" \
  --extra-cflags="-I/ffmpeg_build/include" \
  --extra-ldflags="$SANITIZE_FLAGS -L/ffmpeg_build/lib" \
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

echo "FFmpeg 7.0.1 with x264 (LSan enabled) has been successfully installed to /ffmpeg_build."
