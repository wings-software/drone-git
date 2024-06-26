. "${PSScriptRoot}\utility.ps1"

Set-Alias iu Invoke-Utility

if ($env:DRONE_NETRC_SUBMODULE_STRATEGY -eq "true") {
    Write-Host '+ git submodule update --init'
    iu git submodule update --init
} elseif ($env:DRONE_NETRC_SUBMODULE_STRATEGY -eq "recursive") {
    Write-Host '+ git submodule update --init --recursive'
    iu git submodule update --init --recursive
}