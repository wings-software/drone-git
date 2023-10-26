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

if (![string]::IsNullOrEmpty($env:DRONE_HTTP_PROXY_URL) -and ![string]::IsNullOrEmpty($env:DRONE_HTTP_PROXY_PORT)) {
    Write-Host "+ git config --global --global http.proxy $Env:DRONE_HTTP_PROXY_URL:$Env:DRONE_HTTP_PROXY_PORT "
    iu git config --global http.proxy "$($env:DRONE_HTTP_PROXY_URL):$($env:DRONE_HTTP_PROXY_PORT)"
}

sf -flags ${FLAGS} -ref "+refs/tags/${Env:DRONE_TAG}"
Write-Host "+ git checkout -qf ${Env:FETCH_HEAD}";
iu git checkout -qf FETCH_HEAD
