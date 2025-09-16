# Optimize Windows Docker Image Size

## Problem Analysis

The current Windows LTSC2022 image is several GBs in size, causing slow downloads. After examining the Dockerfiles, I've identified the main size contributors:

### Current Architecture Issues:
1. **Base Image**: Uses `mcr.microsoft.com/windows/servercore:ltsc2022` (~4.1GB) + `mcr.microsoft.com/powershell:windowsservercore-ltsc2022` (~5.3GB)
2. **Git Installation**: Downloads full MinGit package (~50MB) 
3. **Git LFS**: Separate download and installation (~30MB)
4. **OpenSSH**: Adds Windows capability increasing size
5. **Multi-stage build inefficiencies**: Not optimally cleaning up intermediate layers

## Optimization Plan

### Phase 1: Base Image Optimization
- **Switch to Windows Nano Server**: Replace `windowsservercore` with `nanoserver` base images
  - `mcr.microsoft.com/windows/nanoserver:ltsc2022` (~300MB vs ~4GB)
  - `mcr.microsoft.com/powershell:nanoserver-ltsc2022` (~400MB vs ~5GB)

### Phase 2: Git Installation Optimization  
- **Use Git Portable**: Switch from MinGit to PortableGit which is more optimized for containers
- **Single download step**: Combine Git and Git LFS installation
- **Clean up artifacts**: Remove zip files and temporary downloads in same RUN layer

### Phase 3: Layer Optimization
- **Combine RUN commands**: Reduce layers by combining download, extract, and cleanup operations
- **Remove unnecessary components**: Strip unused Git components for container use
- **Optimize PATH setup**: Streamline environment variable configuration

### Phase 4: Alternative Approach (if needed)
- **Static binary approach**: Investigate using scratch/distroless base with static git binary
- **Multi-arch considerations**: Ensure ARM64 compatibility maintained

## Expected Size Reduction
- Current: ~5-8GB 
- Target: ~500MB-1GB (80-90% reduction)

## Implementation Tasks
1. ✅ ~~Create optimized Dockerfile.windows.ltsc2022.optimized~~ (Updated existing files instead)
2. ✅ ~~Create rootless variant Dockerfile.windows.ltsc2022.optimized.rootless~~ (Updated existing files instead)
3. ✅ Create build script ./build-optimized-windows.sh with size comparison
4. ✅ Update CLAUDE.md documentation
5. ✅ **CHANGED**: Update existing Dockerfile.windows.ltsc2022 directly with optimizations
6. ✅ **CHANGED**: Update existing Dockerfile.windows.ltsc2022.rootless directly with optimizations
7. ✅ Create backup files (.backup) of original Dockerfiles
8. ✅ Remove temporary optimized files  
9. ✅ Update build script to use existing filenames
10. ⏳ Test functionality with existing PowerShell scripts
11. ⏳ Benchmark image size before/after
12. ⏳ Validate git operations work correctly

## Implementation Details Completed

### Key Changes Made:
1. **Base Image Switch**: 
   - FROM: `mcr.microsoft.com/windows/servercore:ltsc2022` (~4.1GB) + `mcr.microsoft.com/powershell:windowsservercore-ltsc2022` (~5.3GB)
   - TO: `mcr.microsoft.com/windows/nanoserver:ltsc2022` (~300MB) + `mcr.microsoft.com/powershell:nanoserver-ltsc2022` (~400MB)

2. **Git Installation Optimization**:
   - Switched from MinGit to PortableGit (more container-optimized)
   - Combined download, extract, and cleanup in single RUN layer
   - Removed unnecessary components (docs, man pages, locale files)

3. **Layer Optimization**:
   - Single RUN command for all Git setup operations
   - Immediate cleanup of temporary files and downloads
   - Eliminated intermediate layers

4. **Build Infrastructure**:
   - Created `build-optimized-windows.sh` script with size comparison reporting
   - Updated to use existing Dockerfile names instead of creating new ones
   - Added both standard and rootless variants
   - Included verification tests in build script

5. **File Management**:
   - **IMPORTANT CHANGE**: Updated existing files directly instead of creating new ones per user request
   - Created backup files: `Dockerfile.windows.ltsc2022.backup` and `Dockerfile.windows.ltsc2022.rootless.backup`
   - Removed temporary optimized files to avoid confusion
   - Maintained backward compatibility by keeping same filenames

## Risk Assessment
- **Low Risk**: Base image change (Nano Server supports PowerShell) ✅
- **Medium Risk**: Git installation method change (need to verify all git commands work) ⏳
- **Low Risk**: Layer optimization (standard Docker best practices) ✅