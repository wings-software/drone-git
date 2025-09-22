# Test SSH Clone Functionality - Windows LTSC 2022

## Overview
Need to verify that SSH clone functionality works with the optimized Windows LTSC 2022 Docker image `harness/drone-git:CI-18700-windows-ltsc2022-amd64`. Manager reported that SSH clone wasn't working in previous optimization attempts.

## Implementation Plan

### Phase 1: Analyze Current SSH Support
1. **Examine the optimized LTSC 2022 Dockerfile**
   - Check if OpenSSH client is included
   - Verify SSH-related PATH configurations
   - Compare with working baseline (if any)

2. **Review Windows clone scripts**
   - Check `windows/clone.ps1` for SSH handling
   - Verify SSH key management logic
   - Look for any SSH-specific dependencies

### Phase 2: Test SSH Clone Functionality
1. **Basic SSH connectivity test**
   - Test if SSH client is available in container
   - Verify SSH can connect to common Git hosts (GitHub, GitLab)

2. **SSH clone test scenarios**
   - Test with SSH Git URL (git@github.com:user/repo.git format)
   - Test with SSH key authentication
   - Test with known_hosts handling

3. **Compare with HTTPS clone**
   - Verify HTTPS clone still works (regression test)
   - Document any differences in behavior

### Phase 3: Debugging & Fixes (if needed)
1. **If SSH clone fails:**
   - Check if OpenSSH client is missing from optimized image
   - Verify SSH client is in PATH
   - Check if SSH configuration directories exist
   - Look for missing dependencies removed during optimization

2. **Implement fixes:**
   - Add missing SSH components if needed
   - Update PATH configuration
   - Ensure proper permissions for SSH

### Testing Approach
- Use actual container to test SSH clone operations
- Test against real repositories (public GitHub repos)
- Document exact error messages and behaviors
- Compare with known working configurations

## COMPLETED ANALYSIS & FIX

### Root Cause Identified
The optimized LTSC 2022 Dockerfile was missing OpenSSH client support:

1. **Missing OpenSSH Installation**: The original used `Add-WindowsCapability -Online -Name OpenSSH.Client*` but our optimized version switched to nanoserver base which doesn't support this command
2. **Missing OpenSSH Files**: No `COPY --from=git C:\\Windows\\System32\\OpenSSH\\ /openssh` 
3. **Missing PATH Entry**: OpenSSH wasn't included in the PATH environment variable

### Fix Implemented
Updated `docker/Dockerfile.windows.ltsc2022` to:

1. **Use servercore for build stage**: Changed from `powershell:nanoserver-ltsc2022` to `windows/servercore:ltsc2022` for the build stage to support OpenSSH installation
2. **Install OpenSSH**: Added `RUN Add-WindowsCapability -Online -Name OpenSSH.Client*`
3. **Copy OpenSSH to final image**: Added `COPY --from=git C:\\Windows\\System32\\OpenSSH\\ /openssh`
4. **Update PATH**: Added `;C:\openssh` to the PATH environment variable
5. **Verification**: Added SSH client verification in the build process

### Next Steps
- Rebuild the image with the updated Dockerfile
- Test SSH clone functionality with the new image
- Verify both SSH and HTTPS clones work properly

## Expected Deliverables
1. **Fix Implementation** ✅ **COMPLETED**
   - Updated Dockerfile with SSH support
   - Documentation of changes made
   - Ready for rebuild and testing

2. **Test Results Report** - **PENDING**
   - SSH clone functionality status (after rebuild)
   - Verification that both SSH and HTTPS work