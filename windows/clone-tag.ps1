$ErrorActionPreference = 'Stop';

Set-Variable -Name "FLAGS" -Value ""
if ($Env:PLUGIN_DEPTH) {
    Set-Variable -Name "FLAGS" -Value "--depth=$Env:PLUGIN_DEPTH" 
}

if (!(Test-Path .git)) {
	Write-Host '+ git init';
	git init
	Write-Host "+ git remote add origin $Env:DRONE_REMOTE_URL"
	git remote add origin "$Env:DRONE_REMOTE_URL"
}

Write-Host "+ git fetch $FLAGS origin +refs/tags/${Env:DRONE_TAG}:";
git fetch $FLAGS origin "+refs/tags/${Env:DRONE_TAG}:"
Write-Host "+ git checkout -qf ${Env:FETCH_HEAD}";
git checkout -qf FETCH_HEAD
