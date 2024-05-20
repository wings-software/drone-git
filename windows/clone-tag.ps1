. "${PSScriptRoot}\utility.ps1"
. "${PSScriptRoot}\git-utility.ps1"

Set-Alias iu Invoke-Utility
Set-Alias sf Start-Fetch

Set-Variable -Name "FLAGS" -Value ""
if ($Env:PLUGIN_DEPTH) {
    Set-Variable -Name "FLAGS" -Value "--depth=$Env:PLUGIN_DEPTH" 
}

if (!(Test-Path .git)) {
	Write-Host '+ git init';
	iu git init

	Write-Host "+ git config --global --add safe.directory *"
	iu git config --global --add safe.directory '*'

	Write-Host "+ git remote add origin $Env:DRONE_REMOTE_URL"
	iu git remote add origin "$Env:DRONE_REMOTE_URL"
}

if ($env:HARNESS_GIT_PROXY -eq "true" -and -not [string]::IsNullOrEmpty($env:HARNESS_HTTPS_PROXY)) {
    Write-Host "+ git config --global http.proxy $env:HARNESS_HTTPS_PROXY"
    iu git config --global http.proxy $env:HARNESS_HTTPS_PROXY
}

sf -flags ${FLAGS} -ref "+refs/tags/${Env:DRONE_TAG}"
Write-Host "+ git checkout -qf ${Env:FETCH_HEAD}";
iu git checkout -qf FETCH_HEAD
