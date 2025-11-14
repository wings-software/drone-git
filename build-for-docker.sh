#!/bin/bash

# Build drone-git binaries for Docker deployment
set -e

echo "🚀 Building drone-git binaries for Docker integration..."

# Create output directory
mkdir -p docker-binaries

# Linux AMD64 (most common)
echo "Building for Linux AMD64..."
GOOS=linux GOARCH=amd64 go build -o docker-binaries/drone-git main.go

# Linux ARM64 (for ARM Docker hosts)
echo "Building for Linux ARM64..."
GOOS=linux GOARCH=arm64 go build -o docker-binaries/drone-git-arm64 main.go

# Windows AMD64
echo "Building for Windows AMD64..."
GOOS=windows GOARCH=amd64 go build -o docker-binaries/drone-git.exe main.go

echo "✅ Docker binaries built successfully:"
ls -lh docker-binaries/

echo ""
echo "📦 Binary mapping for Docker files:"
echo "  Linux AMD64/ARM variants: drone-git (11MB)"
echo "  Linux ARM64 variant: drone-git-arm64 (11MB)"
echo "  Windows variants: drone-git.exe (11MB)"

echo ""
echo "🐳 Ready to build Docker images!"
echo "Example: docker build -f docker/Dockerfile.linux.amd64 -t drone-git:linux-amd64 ."

