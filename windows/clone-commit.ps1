. "${PSScriptRoot}\utility.ps1"
. "${PSScriptRoot}\git-utility.ps1"

Set-Alias iu Invoke-Utility
Set-Alias sf Start-Fetch

Set-Variable -Name "FLAGS" -Value ""
if ($Env:PLUGIN_DEPTH) {
	Set-Variable -Name "FLAGS" -Value "--depth=$Env:PLUGIN_DEPTH" 
}

# the branch may be empty for certain event types,
# such as github deployment events. If the branch
# is empty we checkout the sha directly. Note that
# we intentially omit depth flags to avoid failed
# clones due to lack of history.
if ([string]::IsNullOrEmpty($env:DRONE_COMMIT_BRANCH)) {
	sf -flags $null -ref $null
	Write-Host "+ git checkout -qf ${Env:DRONE_COMMIT_SHA}";
	iu git checkout -qf ${Env:DRONE_COMMIT_SHA}
	exit 0
}

# the commit sha may be empty for builds that are
# manually triggered in Harness CI Enterprise. If
# the commit is empty we clone the branch.
if ([string]::IsNullOrEmpty($env:DRONE_COMMIT_SHA)) {
	sf -flags ${FLAGS} -ref "+refs/heads/${Env:DRONE_COMMIT_BRANCH}"
	Write-Host "+ git checkout -b ${Env:DRONE_COMMIT_BRANCH} origin/${Env:DRONE_COMMIT_BRANCH}";
	iu git checkout -b ${Env:DRONE_COMMIT_BRANCH} origin/${Env:DRONE_COMMIT_BRANCH}
}else{
	sf -flags ${FLAGS} -ref "+refs/heads/${Env:DRONE_COMMIT_BRANCH}"
	Write-Host "+ git checkout ${Env:DRONE_COMMIT_SHA} -b ${Env:DRONE_COMMIT_BRANCH}"
	iu git checkout ${Env:DRONE_COMMIT_SHA} -b ${Env:DRONE_COMMIT_BRANCH}
}