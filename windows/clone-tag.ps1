. "${PSScriptRoot}\utility.ps1"
. "${PSScriptRoot}\git-utility.ps1"

Set-Alias iu Invoke-Utility
Set-Alias sf Start-Fetch

Set-Variable -Name "FLAGS" -Value ""
if ($Env:PLUGIN_DEPTH) {
    Set-Variable -Name "FLAGS" -Value "--depth=$Env:PLUGIN_DEPTH" 
}

sf -flags ${FLAGS} -ref "+refs/tags/${Env:DRONE_TAG}"
Write-Host "+ git checkout -qf ${Env:FETCH_HEAD}";
iu git checkout -qf FETCH_HEAD
