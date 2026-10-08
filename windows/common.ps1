. "${PSScriptRoot}\utility.ps1"
. "${PSScriptRoot}\git-utility.ps1"

Set-Alias iu Invoke-Utility
Set-Alias sf Start-Fetch

function Set-OriginUrl {
    param (
        [string]$originUrl
    )
    # Check if the remote 'origin' exists
    $originExists = git remote get-url origin

    if ($originExists) {
        # If 'origin' exists, update its URL
        Write-Host "+ git remote set-url origin $originUrl"
        iu git remote set-url origin $originUrl
    } else {
        # If 'origin' doesn't exist, add it
        Write-Host "+ git remote add origin $originUrl"
        iu git remote add origin $originUrl
    }
}

if ($Env:DRONE_NETRC_DEBUG) {
    $Env:GIT_CURL_VERBOSE = 1
    $Env:GIT_TRACE = 1
    $Env:LFS_DEBUG_HTTP = "true"
}

# Set git config scope based on isolation mode
if ($Env:HARNESS_GIT_CONFIG_FOLDER) {
    $GIT_CONFIG_SCOPE = "--local"
    Write-Host "[DEBUG] Using git config --local (isolated mode)"
} else {
    $GIT_CONFIG_SCOPE = "--global"
    Write-Host "[DEBUG] Using git config --global (global mode)"
}

if (!(Test-Path .git)) {
    Write-Host '+ git init';
    iu git init

    Write-Host "+ git config --global --add safe.directory *"
    iu git config --global --add safe.directory '*'

    Write-Host "+ git remote add origin $Env:DRONE_REMOTE_URL"
    iu git remote add origin $Env:DRONE_REMOTE_URL

    # borrow objects from a local reference repository when one is usable,
    # equivalent to clone --reference-if-able. accepts either a working tree
    # or a bare repository, and is skipped when the path is not a repository.
    if ($Env:PLUGIN_REFERENCE_REPO) {
        $alternates = ""
        if (Test-Path (Join-Path $Env:PLUGIN_REFERENCE_REPO '.git\objects')) {
            $alternates = (Resolve-Path (Join-Path $Env:PLUGIN_REFERENCE_REPO '.git\objects')).Path.Replace('\', '/')
        } elseif (Test-Path (Join-Path $Env:PLUGIN_REFERENCE_REPO 'objects')) {
            $alternates = (Resolve-Path (Join-Path $Env:PLUGIN_REFERENCE_REPO 'objects')).Path.Replace('\', '/')
        }
        if ($alternates) {
            Write-Host "[INFO] borrowing objects from $alternates"
            Set-Content -Path .git\objects\info\alternates -Value $alternates -Encoding ascii
        } else {
            Write-Host "[INFO] skipping reference repository ${Env:PLUGIN_REFERENCE_REPO}: not a git repository"
        }
    }
} else {
    Write-Host "+ git config --global --add safe.directory *"
    iu git config --global --add safe.directory '*'

    Set-OriginUrl -originUrl $Env:DRONE_REMOTE_URL
}

if ($env:DRONE_NETRC_LFS_ENABLED -eq "true") {
    Write-Host "+ git lfs install"
    iu git lfs install
}


if ($env:HARNESS_GIT_PROXY -eq "true" -and -not [string]::IsNullOrEmpty($env:HARNESS_HTTPS_PROXY)) {
    Write-Host "+ git config $GIT_CONFIG_SCOPE http.proxy $env:HARNESS_HTTPS_PROXY"
    iu git config $GIT_CONFIG_SCOPE http.proxy $env:HARNESS_HTTPS_PROXY
}

if ($env:DRONE_NETRC_FETCH_TAGS -eq "true") {
    Write-Host "+ git fetch --tags"
    iu git fetch --tags
}

if (-not [string]::IsNullOrEmpty($env:DRONE_NETRC_SPARSE_CHECKOUT)) {
    Write-Host '+ git sparse-checkout init'
    iu git sparse-checkout init
    $lines = $env:DRONE_NETRC_SPARSE_CHECKOUT -split "`n"
    foreach ($line in $lines) {
        Write-Host "+ git sparse-checkout add $line"
        iu git sparse-checkout add $line
    }
}

if (-not [string]::IsNullOrEmpty($env:DRONE_NETRC_PRE_FETCH)) {
    $lines = $env:DRONE_NETRC_PRE_FETCH -split "`n"
    foreach ($line in $lines) {
        Write-Host "+ $line"
        Invoke-Expression $line
    }
}
