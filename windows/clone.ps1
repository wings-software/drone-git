$ErrorActionPreference = 'Stop';

# HACK: no clue how to set the PATH inside the Dockerfile,
# so am setting it here instead. This is not idea.
$Env:PATH += ';C:\git\cmd;C:\git\mingw64\bin;C:\git\usr\bin'

# if the workspace is set we should make sure
# it is the current working directory.

if ($Env:DRONE_WORKSPACE) {
    cd $Env:DRONE_WORKSPACE
}

# if the netrc enviornment variables exist, write
# the netrc file.

if ($Env:DRONE_NETRC_MACHINE) {
@"
machine $Env:DRONE_NETRC_MACHINE
login $Env:DRONE_NETRC_USERNAME
password $Env:DRONE_NETRC_PASSWORD
"@ > (Join-Path $Env:USERPROFILE '_netrc');
}

# AWS codecommit support using AWS access key & secret key
# Refer: https://docs.aws.amazon.com/codecommit/latest/userguide/setting-up-https-unixes.html

if ($Env:DRONE_AWS_ACCESS_KEY) {
	aws configure set aws_access_key_id $DRONE_AWS_ACCESS_KEY
	aws configure set aws_secret_access_key $DRONE_AWS_SECRET_KEY
	aws configure set default.region $DRONE_AWS_REGION

	git config --global credential.helper '!aws codecommit credential-helper $@'
	git config --global credential.UseHttpPath true
}

# configure git global behavior and parameters via the
# following environment variables:

if ($Env:PLUGIN_SKIP_VERIFY) {
    $Env:GIT_SSL_NO_VERIFY = "true"
}

if ($Env:DRONE_COMMIT_AUTHOR_NAME -eq '' -or $Env:DRONE_COMMIT_AUTHOR_NAME -eq $null) {
    $Env:GIT_AUTHOR_NAME = "drone"
} else {
    $Env:GIT_AUTHOR_NAME = $Env:DRONE_COMMIT_AUTHOR_NAME
}

if ($Env:DRONE_COMMIT_AUTHOR_EMAIL -eq '' -or $Env:DRONE_COMMIT_AUTHOR_EMAIL -eq $null) {
    $Env:GIT_AUTHOR_EMAIL = 'drone@localhost'
} else {
    $Env:GIT_AUTHOR_EMAIL = $Env:DRONE_COMMIT_AUTHOR_EMAIL
}

$Env:GIT_COMMITTER_NAME  = $Env:GIT_AUTHOR_NAME
$Env:GIT_COMMITTER_EMAIL = $Env:GIT_AUTHOR_EMAIL

# invoke the sub-script based on the drone event type.
# TODO we should ultimately look at the ref, since
# we need something compatible with deployment events.

$CLONE_TYPE=$Env:DRONE_BUILD_EVENT
switch -regex ($Env:DRONE_COMMIT_REF) { 
    'refs/tags/*' {
        $CLONE_TYPE="tag"
    }
    'refs/pull/*' {
        $CLONE_TYPE="pull_request"
    }
    'refs/pull-request/*' {
        $CLONE_TYPE="pull_request"
    }
    'refs/merge-requests/*' {
        $CLONE_TYPE="pull_request"
    }
}

switch ($CLONE_TYPE) {
    "pull_request" {
        Invoke-Expression "${PSScriptRoot}\clone-pull-request.ps1"
        break
    }
    "tag" {
        Invoke-Expression "${PSScriptRoot}\clone-tag.ps1"
        break
    }
    default {
        Invoke-Expression "${PSScriptRoot}\clone-commit.ps1"
        break
    }
}
