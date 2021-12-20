. "${PSScriptRoot}\utility.ps1"

Set-Alias iu Invoke-Utility

Set-Variable -Name "FLAGS" -Value ""
if ($Env:PLUGIN_DEPTH) {
    Set-Variable -Name "FLAGS" -Value "--depth=$Env:PLUGIN_DEPTH" 
}

if (!(Test-Path .git)) {
	Write-Host '+ git init';
	iu git init
	Write-Host "+ git remote add origin $Env:DRONE_REMOTE_URL"
	iu git remote add origin "$Env:DRONE_REMOTE_URL"
}

Write-Host "+ git fetch $FLAGS origin +refs/tags/${Env:DRONE_TAG}:";
iu git fetch $FLAGS origin "+refs/tags/${Env:DRONE_TAG}:"
Write-Host "+ git checkout -qf ${Env:FETCH_HEAD}";
iu git checkout -qf FETCH_HEAD
