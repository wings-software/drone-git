# drone-git

Drone plugin to clone `git` repositories.

Run the following script to install git-leaks support to this repo.
```
chmod +x ./git-hooks/install.sh
./git-hooks/install.sh
```

## Build 

Build the Docker image with the following commands:

```
docker build --rm -f docker/Dockerfile.linux.amd64 -t harness/drone-git .
```

### Windows LTSC 2022 optimized images

Both LTSC 2022 optimized variants use Server Core. The bundled Git/MSYS shell
imports Windows APIs absent from Nano Server 2022; copying the tools from a
Server Core stage into Nano Server does not supply those APIs. This can break
LFS checkout with `fatal: the remote end hung up unexpectedly` even after fetch
succeeds.

On a Windows LTSC 2022 Docker builder, build both variants with the release
version supplied as a build argument:

```powershell
$pluginVersion = 'local-validation' # Use the release version when publishing.
docker build --build-arg "PLUGIN_VERSION=$pluginVersion" -f docker/Dockerfile.windows.ltsc2022.optimized -t drone-git:windows-2022-validation .
docker build --build-arg "PLUGIN_VERSION=$pluginVersion" -f docker/Dockerfile.windows.ltsc2022.rootless.optimized -t drone-git:windows-2022-rootless-validation .
```

These Dockerfiles compile `drone-git.exe` from the current source, including its
embedded clone scripts. No manually created `release/windows/amd64` binary is
required. `C:\bin\clone.ps1` invokes that executable, so `PLUGIN_BUILD_TOOL_FILE`
receives the language, build tool, and code metrics JSON after a successful clone.
The image build checks that `sh.exe --version` succeeds as the final runtime user.

Before publishing, validate both variants on Windows with a real LFS checkout,
recursive submodules, sparse checkout, a multiline pre-fetch command, and paths
containing spaces. Check the requested commit, materialized LFS file, submodule
content, and metadata JSON. Use at least 1 GiB for the advanced LFS/submodule QA
fixture; increasing memory does not repair an incompatible Nano Server runtime.

## Usage

Clone a commit:

```
docker run --rm \
  -e DRONE_WORKSPACE=/drone \
  -e DRONE_REMOTE_URL=https://github.com/drone/envsubst.git \
  -e DRONE_BUILD_EVENT=push \
  -e DRONE_COMMIT_SHA=15e3f9b7e16332eee3bbdff9ef31f95d23c5da2c \
  -e DRONE_COMMIT_BRANCH=master \
  -w "/home/drone"
  harness/drone-git
```
