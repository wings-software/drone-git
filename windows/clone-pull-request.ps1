. "${PSScriptRoot}\utility.ps1"

Set-Alias iu Invoke-Utility

Set-Variable -Name "FLAGS" -Value ""
if ($Env:PLUGIN_DEPTH) {
    Set-Variable -Name "FLAGS" -Value "--depth=$Env:PLUGIN_DEPTH"
}

if (!(Test-Path .git)) {
	iu git init
	iu git remote add origin $Env:DRONE_REMOTE_URL
}

if ($Env:PLUGIN_PR_CLONE_STRATEGY -eq "SourceBranch") {
	Write-Host "+ git fetch ${FLAGS} origin ${Env:DRONE_COMMIT_REF}:"
	iu git fetch ${FLAGS} origin "${Env:DRONE_COMMIT_REF}:"
	Write-Host "+ git checkout ${Env:DRONE_COMMIT_SHA} -b ${Env:DRONE_SOURCE_BRANCH}"
	iu git checkout ${Env:DRONE_COMMIT_SHA} -b ${Env:DRONE_SOURCE_BRANCH}
	exit 0
}

Write-Host "+ git fetch ${FLAGS} origin +refs/heads/${Env:DRONE_COMMIT_BRANCH}:"
iu git fetch ${FLAGS} origin "+refs/heads/${Env:DRONE_COMMIT_BRANCH}:"

if (Test-Path env:DRONE_COMMIT_BEFORE) {
	# PR clone strategy is merge commit
	Write-Host "+ git checkout ${Env:DRONE_COMMIT_BEFORE} -b ${Env:DRONE_COMMIT_BRANCH}"
	iu git checkout ${Env:DRONE_COMMIT_BEFORE} -b ${Env:DRONE_COMMIT_BRANCH}
} else {
	Write-Host "+ git checkout $Env:DRONE_COMMIT_BRANCH"
	iu git checkout $Env:DRONE_COMMIT_BRANCH
}

Write-Host "+ git fetch origin ${Env:DRONE_COMMIT_REF}:"
iu git fetch origin "${Env:DRONE_COMMIT_REF}:"
Write-Host "+ git merge $Env:DRONE_COMMIT_SHA"
iu git merge $Env:DRONE_COMMIT_SHA