$ErrorActionPreference = 'Stop';

Set-Variable -Name "FLAGS" -Value ""
if ($Env:PLUGIN_DEPTH) {
    Set-Variable -Name "FLAGS" -Value "--depth=$Env:PLUGIN_DEPTH" 
}

if (!(Test-Path .git)) {
	Write-Host '+ git init';
	git init
	Write-Host "+ git remote add origin $Env:DRONE_REMOTE_URL"
	git remote add origin $Env:DRONE_REMOTE_URL
}

# the branch may be empty for certain event types,
# such as github deployment events. If the branch
# is empty we checkout the sha directly. Note that
# we intentially omit depth flags to avoid failed
# clones due to lack of history.
if (-not (Test-Path env:DRONE_COMMIT_BRANCH)) {
	Write-Host "+ git fetch origin";
	git fetch origin
	Write-Host "+ git checkout -qf ${Env:DRONE_COMMIT_SHA}";
	git checkout -qf ${Env:DRONE_COMMIT_SHA}
	exit 0
}

# the commit sha may be empty for builds that are
# manually triggered in Harness CI Enterprise. If
# the commit is empty we clone the branch.
if (-not (Test-Path env:DRONE_COMMIT_SHA)) {
	Write-Host "+ git fetch ${FLAGS} origin +refs/heads/${Env:DRONE_COMMIT_BRANCH}:";
	git fetch ${FLAGS} origin "+refs/heads/${Env:DRONE_COMMIT_BRANCH}:"
	Write-Host "+ git checkout -b ${Env:DRONE_COMMIT_BRANCH} origin/${Env:DRONE_COMMIT_BRANCH}";
	git checkout -b ${Env:DRONE_COMMIT_BRANCH} origin/${Env:DRONE_COMMIT_BRANCH}
	exit 0
}

Write-Host "+ git fetch ${FLAGS} origin +refs/heads/${Env:DRONE_COMMIT_BRANCH}:"
git fetch ${FLAGS} origin "+refs/heads/${Env:DRONE_COMMIT_BRANCH}:"
Write-Host "+ git checkout ${Env:DRONE_COMMIT_SHA} -b ${Env:DRONE_COMMIT_BRANCH}"
git checkout ${Env:DRONE_COMMIT_SHA} -b ${Env:DRONE_COMMIT_BRANCH}