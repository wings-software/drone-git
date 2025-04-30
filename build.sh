#!/bin/bash

# Create dist directory
mkdir -p dist

# Build binaries
echo "Building Linux AMD64..."
GOOS=linux GOARCH=amd64 go build -o dist/drone-git-linux-amd64 

echo "Building Linux ARM64..."
GOOS=linux GOARCH=arm64 go build -o dist/drone-git-linux-arm64 

echo "Building Linux ARM7..."
GOOS=linux GOARCH=arm GOARM=7 go build -o dist/drone-git-linux-arm7 

echo "Building Windows AMD64..."
GOOS=windows GOARCH=amd64 go build -o dist/drone-git-windows-amd64.exe 