. "${PSScriptRoot}\utility.ps1"
. "${PSScriptRoot}\git-utility.ps1"

Set-Alias iu Invoke-Utility
Set-Alias sf Start-Fetch

function Set-OriginUrl {
    param (
        [string]$originUrl
    )
    # Check if the remote 'origin' exists
    $originExists = git remote get-url origin -ErrorAction SilentlyContinue

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

if (!(Test-Path .git)) {
    Write-Host '+ git init';
    iu git init

    Write-Host "+ git config --global --add safe.directory *"
    iu git config --global --add safe.directory '*'

    Write-Host "+ git remote add origin $Env:DRONE_REMOTE_URL"
    iu git remote add origin $Env:DRONE_REMOTE_URL
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
    Write-Host "+ git config --global http.proxy $env:HARNESS_HTTPS_PROXY"
    iu git config --global http.proxy $env:HARNESS_HTTPS_PROXY
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
