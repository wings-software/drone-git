. "${PSScriptRoot}\utility.ps1"
. "${PSScriptRoot}\git-utility.ps1"

Set-Alias iu Invoke-Utility
Set-Alias sf Start-Fetch

Set-Variable -Name "FLAGS" -Value ""
if ($Env:PLUGIN_DEPTH) {
	Set-Variable -Name "FLAGS" -Value "--depth=$Env:PLUGIN_DEPTH"
}

if ($Env:PLUGIN_PR_CLONE_STRATEGY -eq "SourceBranch") {
	sf -flags ${FLAGS} -ref "${Env:DRONE_COMMIT_REF}"
	Write-Host "+ git checkout ${Env:DRONE_COMMIT_SHA} -b ${Env:DRONE_SOURCE_BRANCH}"
	iu git checkout ${Env:DRONE_COMMIT_SHA} -b ${Env:DRONE_SOURCE_BRANCH}
	exit 0
}

sf -flags ${FLAGS} -ref "+refs/heads/${Env:DRONE_COMMIT_BRANCH}"

if (Test-Path env:DRONE_COMMIT_BEFORE) {
	if ($env:DRONE_PR_MERGE_STRATEGY_BRANCH -eq "true") {
		Write-Host "+ git checkout $Env:DRONE_COMMIT_BRANCH"
		iu git checkout $Env:DRONE_COMMIT_BRANCH
	} else {
		# PR clone strategy is merge commit
		Write-Host "+ git checkout ${Env:DRONE_COMMIT_BEFORE} -b ${Env:DRONE_COMMIT_BRANCH}"
		iu git checkout ${Env:DRONE_COMMIT_BEFORE} -b ${Env:DRONE_COMMIT_BRANCH}
	}
} else {
	Write-Host "+ git checkout $Env:DRONE_COMMIT_BRANCH"
	iu git checkout $Env:DRONE_COMMIT_BRANCH
}

sf -flags $null -ref "${Env:DRONE_COMMIT_REF}"

Write-Host "+ git merge $Env:DRONE_COMMIT_SHA"
iu git merge $Env:DRONE_COMMIT_SHA