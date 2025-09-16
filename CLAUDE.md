# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Plan & Review
### Before starting work
- Write a plan to .claude/tasks/TASK_NAME. md.
- The plan should be a detailed implementation plan and the reasoning behind them, as well as tasks broken down.
- Don't over plan it, always think MVP.
- Once you write the plan, firstly ask me to review it. Do not continue until I approve the plan.
### While implementing
- You should update the plan as you work.
- After you complete tasks in the plan, you should update and append detailed descriptions of the changes you made, so following tasks can be easily hand over to other


## Project Overview

drone-git is a Drone CI plugin for cloning Git repositories. It's written in Go and supports cross-platform execution on Linux, macOS, and Windows through embedded shell scripts.

## Build Commands

- **Build all binaries**: `./build.sh` - Creates binaries for Linux (amd64, arm64, arm7) and Windows (amd64) in `dist/` directory
- **Build single binary**: `go build -o drone-git`
- **Build Docker image**: `docker build --rm -f docker/Dockerfile.linux.amd64 -t harness/drone-git .`
- **Build Windows images**: `./build-optimized-windows.sh` - Builds optimized Windows LTSC2022 images (~80-90% smaller than original)

## Test Commands

- **Run all tests**: `go test ./...`
- **Run specific package tests**: `go test ./posix` or `go test ./windows`
- **Run with verbose output**: `go test -v ./...`

## Development Commands

- **Generate embedded scripts**: 
  - For POSIX: `cd posix && go generate`
  - For Windows: `cd windows && go generate`
- **Install git-leaks hooks**: `chmod +x ./git-hooks/install.sh && ./git-hooks/install.sh`
- **Format code**: `go fmt ./...`
- **Vet code**: `go vet ./...`

## Architecture

The project follows a cross-platform design pattern:

### Core Components

- **`main.go`**: Entry point that determines OS and executes appropriate scripts from embedded filesystem
- **`posix/`**: Contains shell scripts and Go code for Unix-like systems (Linux/macOS)
- **`windows/`**: Contains PowerShell scripts and Go code for Windows
- **`docker/`**: Multi-architecture Dockerfiles for containerized deployment
  - Windows LTSC2022 images now use Nano Server base for 80-90% size reduction (~500MB vs ~5GB)
  - Backup files: `*.backup` contain original versions before optimization
- **`scripts/`**: Build-time utilities for embedding script content into Go binaries

### Execution Flow

1. `main.go` creates temporary directory and extracts embedded scripts based on runtime OS
2. For POSIX systems: executes `posix/script` via bash/sh
3. For Windows: executes `windows/clone.ps1` via PowerShell
4. Scripts handle various git operations (clone, clone-commit, clone-pull-request, clone-tag)

### Code Generation

The project uses `go:generate` directives to embed shell/PowerShell scripts into Go source:
- `posix/posix.go` embeds POSIX shell scripts into `posix_gen.go`
- `windows/windows.go` embeds PowerShell scripts into `windows_gen.go`
- `scripts/includetext.go` handles the embedding process

### Environment Variables

The plugin reads Drone CI environment variables like:
- `DRONE_WORKSPACE`: Working directory
- `DRONE_REMOTE_URL`: Git repository URL
- `DRONE_BUILD_EVENT`: Build trigger event
- `DRONE_COMMIT_SHA`: Commit hash to clone
- `DRONE_COMMIT_BRANCH`: Branch name

Run `go generate` in the respective `posix/` or `windows/` directories after modifying scripts to regenerate embedded content.