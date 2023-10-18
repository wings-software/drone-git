$ErrorActionPreference = 'Stop';

# HACK: no clue how to set the PATH inside the Dockerfile,
# so am setting it here instead. This is not idea.
$Env:PATH += ';C:\git\cmd;C:\git\mingw64\bin;C:\git\usr\bin;C:\openssh'

# if the workspace is set we should create it and make sure it is the current working directory.
if ($Env:DRONE_WORKSPACE) {
    md -Force $Env:DRONE_WORKSPACE
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

if ($Env:DRONE_SSH_KEY) {
    mkdir C:\.ssh  -Force
    echo $Env:DRONE_SSH_KEY > C:\.ssh\id_rsa

    # $Env:SSH_KEYSCAN_FLAGS=""
    # if ($Env:DRONE_NETRC_PORT) {
    # 	$Env:SSH_KEYSCAN_FLAGS="-p ${Env:DRONE_NETRC_PORT}"
    # }
    # ssh-keyscan -H $Env:SSH_KEYSCAN_FLAGS $Env:DRONE_NETRC_MACHINE >  C:\\.ssh\\known_hosts

    $Env:GIT_SSH_COMMAND="ssh -i C:\\.ssh\\id_rsa ${Env:SSH_KEYSCAN_FLAGS} -o StrictHostKeyChecking=no"
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

Set-Variable -Name "CLONE_TYPE" -Value "$Env:DRONE_BUILD_EVENT"
switch -regex ($Env:DRONE_COMMIT_REF)
{
    'refs/tags/*' {
        Set-Variable -Name "CLONE_TYPE" -Value "tag"
        break
    }

    'refs/pull/*' {
        Set-Variable -Name "CLONE_TYPE" -Value "pull_request"
        break
    }

    'refs/pull-request/*' {
        Set-Variable -Name "CLONE_TYPE" -Value "pull_request"
        break
    }

    'refs/merge-requests/*' {
        Set-Variable -Name "CLONE_TYPE" -Value "pull_request"
        break
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
