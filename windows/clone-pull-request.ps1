Set-PSDebug -Trace 1

Set-Variable -Name "FLAGS" -Value ""
if ($Env:PLUGIN_DEPTH) {
    Set-Variable -Name "FLAGS" -Value "--depth=$Env:PLUGIN_DEPTH"
}

if (!(Test-Path .git)) {
	git init
	git remote add origin $Env:DRONE_REMOTE_URL
}

# If PR clone strategy is cloning only the source branch
if ($Env:PLUGIN_PR_CLONE_STRATEGY -eq "SourceBranch") {
	git fetch $FLAGS origin $Env:DRONE_COMMIT_REF:
	git checkout $Env:DRONE_COMMIT_SHA -b $Env:DRONE_SOURCE_BRANCH
	exit 0
}

# PR clone strategy is merge commit
targetRef=$Env:DRONE_COMMIT_BRANCH
if ($Env:DRONE_COMMIT_BEFORE){
	targetRef="$Env:DRONE_COMMIT_BEFORE -b $Env:DRONE_COMMIT_BRANCH"
}

git fetch $FLAGS origin "+refs/heads/${Env:DRONE_COMMIT_BRANCH}:"
git checkout $targetRef

git fetch origin "${Env:DRONE_COMMIT_REF}:"
git merge $Env:DRONE_COMMIT_SHA