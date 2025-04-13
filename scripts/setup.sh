#!/bin/bash

# Setup script for installing cross-compilers for CGO support
# Used in GitHub Actions workflow for building Tide on multiple platforms

set -e

echo "Installing cross compilers for CGO support..."
sudo apt-get update
sudo apt-get install -y --no-install-recommends \
  gcc \
  gcc-aarch64-linux-gnu \
  libc6-dev-arm64-cross \
  gcc-mingw-w64-x86-64

# Create directory to mark that cross-compilers are installed
mkdir -p ~/.apt-cross-toolchains
echo "Installed on: $(date)" > ~/.apt-cross-toolchains/installed.txt

echo "Cross-compiler installation completed successfully."


# move image to hicolor
