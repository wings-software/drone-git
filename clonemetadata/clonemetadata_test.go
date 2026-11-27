package clonemetadata

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRepoIdentity(t *testing.T) {
	for _, test := range []struct {
		raw       string
		scmType   string
		org, repo string
	}{
		// Legacy rules (empty scmType): the pre-CI-25945 behavior, unchanged.
		{raw: "https://user:token@github.com/acme/app.git", org: "acme", repo: "app"},
		{raw: "https://github.com/acme/app", org: "acme", repo: "app"},
		{raw: "git@github.com:acme/app.git", org: "acme", repo: "app"},
		{raw: "ssh://git@github.com/acme/app.git", org: "acme", repo: "app"},
		{raw: "https://gitlab.com/group/subgroup/repo.git", org: "group/subgroup", repo: "repo"},
		{raw: "https://dev.azure.com/myorg/myproject/_git/myrepo", org: "myorg", repo: "myrepo"},
		{raw: "https://bitbucket.example.com/scm/PROJ/repo.git", org: "PROJ", repo: "repo"},
		{raw: "", org: "", repo: ""},

		// Provider-aware rules (scmType from DRONE_REPO_SCM).
		{raw: "https://github.com/acme/app.git", scmType: scmGithub, org: "acme", repo: "app"},
		{raw: "https://gitlab.com/group/subgroup/repo.git", scmType: scmGitlab, org: "group/subgroup", repo: "repo"},
		// A GitLab namespace genuinely named "scm" is identity, not furniture.
		{raw: "https://gitlab.com/scm/tools/app.git", scmType: scmGitlab, org: "scm/tools", repo: "app"},
		{raw: "https://dev.azure.com/myorg/myproject/_git/myrepo", scmType: scmAzureRepo, org: "myorg", repo: "myrepo"},
		// Azure SSH: scp-style carries a v3/ path prefix; the ssh:// form puts :v3
		// in the port position, which url.Parse rejects (repoPath cuts it manually).
		{raw: "myorg@vs-ssh.visualstudio.com:v3/myorg/myproject/_git/myrepo", scmType: scmAzureRepo, org: "myorg", repo: "myrepo"},
		{raw: "ssh://myorg@vs-ssh.visualstudio.com:v3/myorg/myproject/_git/myrepo", scmType: scmAzureRepo, org: "myorg", repo: "myrepo"},
		// Azure's current SSH form (ssh.dev.azure.com) drops the _git segment; the
		// v3/ prefix still marks the fixed org/project/repo shape.
		{raw: "git@ssh.dev.azure.com:v3/myorg/myproject/myrepo", scmType: scmAzureRepo, org: "myorg", repo: "myrepo"},
		{raw: "https://bitbucket.org/acme/app.git", scmType: scmBitbucket, org: "acme", repo: "app"},
		{raw: "https://bitbucket.example.com/scm/PROJ/repo.git", scmType: scmBitbucket, org: "PROJ", repo: "repo"},
		// Server mounted under a context path: everything through "scm" is prefix.
		{raw: "https://example.com/bitbucket/scm/PROJ/repo.git", scmType: scmBitbucket, org: "PROJ", repo: "repo"},
		{raw: "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/app", scmType: scmCodeCommit, org: "", repo: "app"},
		{raw: "https://git.harness.io/account/org/proj/app.git", scmType: scmHarness, org: "account/org/proj", repo: "app"},
		{raw: "https://vcs.example.com/acme/app.git", scmType: scmGit, org: "acme", repo: "app"},
	} {
		t.Run(test.scmType+"/"+test.raw, func(t *testing.T) {
			org, repo := parseRepoIdentity(sanitizeRepoURL(test.raw), test.scmType)
			assert.Equal(t, test.org, org)
			assert.Equal(t, test.repo, repo)
		})
	}
}

func TestSanitizeRepoURL(t *testing.T) {
	assert.Equal(t, "https://github.com/acme/app.git", sanitizeRepoURL("https://user:token@github.com/acme/app.git"))
	assert.Equal(t, "https://github.com/acme/app.git", sanitizeRepoURL("https://ghs_token@github.com/acme/app.git"))
	assert.Equal(t, "http://github.com/acme/app.git", sanitizeRepoURL("http://ghs_token@github.com/acme/app.git"))
	assert.Equal(t, "git@github.com:acme/app.git", sanitizeRepoURL("git@github.com:acme/app.git"))
	assert.Equal(t, "ssh://git@github.com/acme/app.git", sanitizeRepoURL("ssh://git@github.com/acme/app.git"))
	assert.Equal(t, "ssh://github.com/acme/app.git", sanitizeRepoURL("ssh://git:pass@github.com/acme/app.git"))
	assert.Equal(t, "", sanitizeRepoURL(""))

	// Query/fragment can carry tokens (?access_token=...); strip on every
	// parseable scheme. The URL otherwise round-trips unchanged.
	assert.Equal(t, "https://github.com/acme/app.git", sanitizeRepoURL("https://github.com/acme/app.git?access_token=secret"))
	assert.Equal(t, "https://github.com/acme/app.git", sanitizeRepoURL("https://user:token@github.com/acme/app.git?access_token=secret"))
	assert.Equal(t, "https://github.com/acme/app.git", sanitizeRepoURL("https://github.com/acme/app.git?access_token=secret#frag"))
	assert.Equal(t, "https://github.com/acme/app.git", sanitizeRepoURL("https://github.com/acme/app.git"))
	assert.Equal(t, "ssh://host/repo.git", sanitizeRepoURL("ssh://host/repo.git?token=x"))
	assert.Equal(t, "ssh://git@host/repo.git", sanitizeRepoURL("ssh://git@host/repo.git?token=x"))
	assert.Equal(t, "ssh://git@host/repo.git", sanitizeRepoURL("ssh://git@host/repo.git"))
	assert.Equal(t, "ssh://host/repo.git", sanitizeRepoURL("ssh://git:pass@host/repo.git?token=x"))
	assert.Equal(t, "ssh://host/repo.git", sanitizeRepoURL("ssh://host/repo.git?"))
	assert.Equal(t, "ssh://host/repo.git", sanitizeRepoURL("ssh://host/repo.git#frag"))
}

// A query-token URL must not reach the receipt, and identity parsing must
// still see the repo path (queries are not part of u.Path).
func TestTryWriteStripsQueryCredentials(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git?access_token=secret")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "https://github.com/acme/app", got.RepoURL)
	assert.Equal(t, "acme", got.OrgName)
	assert.Equal(t, "app", got.RepoName)
	assert.NotContains(t, string(mustRead(t, output)), "secret")
}

func TestCloneTypeFromEnv(t *testing.T) {
	for _, test := range []struct {
		name, event, ref, want string
	}{
		{name: "event tag", event: "tag", ref: "", want: "tag"},
		{name: "event pull_request", event: "pull_request", ref: "", want: "pull_request"},
		{name: "event push", event: "push", ref: "", want: "push"},
		{name: "empty", event: "", ref: "", want: ""},
		{name: "tag ref overrides push", event: "push", ref: "refs/tags/v1", want: "tag"},
		{name: "pull ref overrides push", event: "push", ref: "refs/pull/9/head", want: "pull_request"},
		{name: "pull-request ref", event: "push", ref: "refs/pull-request/1/from", want: "pull_request"},
		{name: "merge-requests ref", event: "push", ref: "refs/merge-requests/1/head", want: "pull_request"},
		{name: "tag ref wins over pull_request event", event: "pull_request", ref: "refs/tags/v1", want: "tag"},
		{name: "heads ref does not override", event: "push", ref: "refs/heads/main", want: "push"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DRONE_BUILD_EVENT", test.event)
			t.Setenv("DRONE_COMMIT_REF", test.ref)
			assert.Equal(t, test.want, cloneTypeFromEnv())
		})
	}
}

// refType maps cloneTypeFromEnv (posix/clone CLONE_TYPE) onto the receipt wire values.
func TestRefTypeForEvent(t *testing.T) {
	for _, test := range []struct {
		event, want string
	}{
		{event: "tag", want: "tag"},
		{event: "pull_request", want: "pr"},
		{event: "branch", want: "branch"},
		{event: "push", want: "branch"},
		{event: "commit", want: "branch"},
		{event: "", want: "branch"},
	} {
		t.Run(test.event, func(t *testing.T) {
			assert.Equal(t, test.want, refTypeForEvent(test.event))
		})
	}
}

func TestTryWriteRefTypePRFromBuildEvent(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_BUILD_EVENT", "pull_request")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "pr", got.RefType)
}

// posix/clone: DRONE_COMMIT_REF=refs/tags/* forces CLONE_TYPE=tag even when
// DRONE_BUILD_EVENT is push. getBuildEventInfo would still say branch.
func TestTryWriteTagRefOverridesPushEvent(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	git(t, workdir, "tag", "v1.2.3")
	git(t, workdir, "checkout", "--detach", "v1.2.3")

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_BUILD_EVENT", "push")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_TAG", "v1.2.3")
	t.Setenv("DRONE_COMMIT_REF", "refs/tags/v1.2.3")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "tag", got.RefType)
	assert.Equal(t, "v1.2.3", got.Tag)
	assert.Empty(t, got.Branch)
	assert.Empty(t, got.TargetBranch)
}

// posix/clone: empty event and DRONE_TAG without refs/tags/* is clone-commit.
// getBuildEventInfo would still classify that as tag.
func TestTryWriteEmptyEventWithDroneTagIsBranch(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_BUILD_EVENT", "")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_TAG", "v1.2.3")
	t.Setenv("DRONE_COMMIT_REF", "")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "branch", got.RefType)
	assert.Equal(t, "main", got.Branch)
	assert.Empty(t, got.Tag)
	assert.Empty(t, got.TargetBranch)
}

// posix/clone: PR refs override a non-PR DRONE_BUILD_EVENT.
func TestTryWritePullRequestRefOverridesPushEvent(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_BUILD_EVENT", "push")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_SOURCE_BRANCH", "feature-x")
	t.Setenv("DRONE_TARGET_BRANCH", "main")
	for _, ref := range []string{
		"refs/pull/9/head",
		"refs/pull-request/9/from",
		"refs/merge-requests/9/head",
	} {
		t.Run(ref, func(t *testing.T) {
			t.Setenv("DRONE_COMMIT_REF", ref)
			t.Setenv("DRONE_TAG", "")

			require.NoError(t, TryWrite(workdir, Inputs{}))
			got := readMetadata(t, output)
			assert.Equal(t, "pr", got.RefType)
			assert.Equal(t, "feature-x", got.Branch)
			assert.Equal(t, "main", got.TargetBranch)
			assert.Empty(t, got.Tag)
		})
	}
}

// A plain branch push resolves to refType branch.
func TestTryWriteRefTypeBranch(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "branch", got.RefType)
}

func TestTryWriteBranchClone(t *testing.T) {
	workdir := initRepo(t, map[string]string{
		"f.txt": "a\nb\nc\n",
		"g.txt": "one\n",
	})
	sha := git(t, workdir, "rev-parse", "HEAD")

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_CI_EXPLICIT_GIT_CLONE_STEP", "true")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://user:token@github.com/acme/app.git")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_COMMIT_SHA", "")

	before := time.Now().UnixMilli()
	require.NoError(t, TryWrite(workdir, Inputs{LinesOfCode: int64Ptr(48210), FileCount: int64Ptr(12)}))
	after := time.Now().UnixMilli()

	got := readMetadata(t, output)
	assert.Equal(t, "https://github.com/acme/app", got.RepoURL)
	assert.Equal(t, sha, got.CommitSHA)
	assert.Equal(t, "branch", got.RefType)
	// LinesOfCode is a pass-through of the binary's scc metrics, not counted here.
	require.NotNil(t, got.LinesOfCode)
	assert.Equal(t, int64(48210), *got.LinesOfCode)
	require.NotNil(t, got.FileCount)
	assert.Equal(t, int64(12), *got.FileCount)
	assert.Equal(t, "main", got.Branch)
	assert.Empty(t, got.Tag)
	assert.Equal(t, "app", got.RepoName)
	assert.Equal(t, "acme", got.OrgName)
	assert.GreaterOrEqual(t, got.Timestamp, before)
	assert.LessOrEqual(t, got.Timestamp, after)
	assert.NotContains(t, string(mustRead(t, output)), "token")
}

// When the shared scc pass did not run (telemetry disabled or failed), the
// receipt omits linesOfCode instead of recording a misleading zero.
func TestTryWriteOmitsLinesOfCodeWhenNotProvided(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Nil(t, got.LinesOfCode)
	assert.NotContains(t, string(mustRead(t, output)), "linesOfCode")
	assert.NotContains(t, string(mustRead(t, output)), "fileCount")
}

func TestTryWriteTagClone(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	git(t, workdir, "tag", "v1.2.3")
	git(t, workdir, "checkout", "--detach", "v1.2.3")

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_CI_EXPLICIT_GIT_CLONE_STEP", "true")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "git@github.com:acme/app.git")
	t.Setenv("DRONE_COMMIT_BRANCH", "")
	t.Setenv("DRONE_TAG", "v1.2.3")
	t.Setenv("DRONE_COMMIT_REF", "refs/tags/v1.2.3")

	require.NoError(t, TryWrite(workdir, Inputs{LinesOfCode: int64Ptr(1)}))
	got := readMetadata(t, output)
	assert.Equal(t, "git@github.com:acme/app", got.RepoURL)
	assert.Equal(t, "tag", got.RefType)
	assert.Empty(t, got.Branch)
	assert.Equal(t, "v1.2.3", got.Tag)
	assert.Equal(t, "acme", got.OrgName)
	assert.Equal(t, "app", got.RepoName)
	require.NotNil(t, got.LinesOfCode)
	assert.Equal(t, int64(1), *got.LinesOfCode)
}

// clone-pull-request checks out DRONE_COMMIT_BRANCH (the target) then merges
// the PR, so gitAbbrevRef returns "main". The receipt still records the PR's
// source branch.
func TestTryWritePullRequestUsesSourceBranchOverCheckedOutTarget(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_CI_EXPLICIT_GIT_CLONE_STEP", "true")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_BUILD_EVENT", "pull_request")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_SOURCE_BRANCH", "feature-x")
	t.Setenv("DRONE_TAG", "stale")
	t.Setenv("DRONE_COMMIT_REF", "refs/merge-requests/1/head")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "pr", got.RefType)
	assert.Equal(t, "feature-x", got.Branch)
	assert.Equal(t, "main", got.TargetBranch)
	assert.Empty(t, got.Tag)
}

func TestTryWriteTagFromRefWhenEnvTagEmpty(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	git(t, workdir, "checkout", "--detach", "HEAD")

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_CI_EXPLICIT_GIT_CLONE_STEP", "true")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_TAG", "")
	t.Setenv("DRONE_COMMIT_REF", "refs/tags/v9")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "tag", got.RefType)
	assert.Empty(t, got.Branch)
	assert.Equal(t, "v9", got.Tag)
}

// The metadata file path is the only gate: implicit codebase clones must write a
// receipt too (CI-25945), so the explicit-clone marker does not gate writing.
func TestTryWriteWritesRegardlessOfExplicitMarker(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_CI_EXPLICIT_GIT_CLONE_STEP", "")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_TAG", "")
	t.Setenv("DRONE_COMMIT_REF", "")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	assert.FileExists(t, output)
}

func TestTryWriteSkipsWithoutMetadataFilePath(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	t.Setenv("HARNESS_CI_EXPLICIT_GIT_CLONE_STEP", "true")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", "")
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")

	require.NoError(t, TryWrite(workdir, Inputs{}))
}

func TestTryWriteSkipsWhenNothingToRecord(t *testing.T) {
	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_CI_EXPLICIT_GIT_CLONE_STEP", "true")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "")
	t.Setenv("DRONE_COMMIT_SHA", "")
	require.NoError(t, TryWrite(t.TempDir(), Inputs{}))
	assert.NoFileExists(t, output)
}

// A receipt with an empty commitSha must never be written: the manager persists
// it as a billing row with a hole. Temp dir is not a git repo and DRONE_COMMIT_SHA
// is unset, so no SHA can be resolved.
func TestTryWriteSkipsWhenCommitShaMissing(t *testing.T) {
	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_CI_EXPLICIT_GIT_CLONE_STEP", "true")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_COMMIT_SHA", "")
	require.NoError(t, TryWrite(t.TempDir(), Inputs{}))
	assert.NoFileExists(t, output)
}

// Same for repoUrl: the manager drops repoUrl-less receipts, so writing one is noise.
func TestTryWriteSkipsWhenRepoUrlMissing(t *testing.T) {
	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_CI_EXPLICIT_GIT_CLONE_STEP", "true")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "")
	t.Setenv("DRONE_COMMIT_SHA", "abc123")
	require.NoError(t, TryWrite(t.TempDir(), Inputs{}))
	assert.NoFileExists(t, output)
}

// scmType is copied verbatim from DRONE_REPO_SCM into the receipt, and it steers
// identity parsing: the Azure v3/ SSH prefix is stripped only because the manager
// told us the provider.
func TestTryWriteScmTypeFromEnv(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REPO_SCM", "AzureRepo")
	t.Setenv("DRONE_REMOTE_URL", "myorg@vs-ssh.visualstudio.com:v3/myorg/myproject/_git/myrepo")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_TAG", "")
	t.Setenv("DRONE_COMMIT_REF", "")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "AzureRepo", got.SCMType)
	assert.Equal(t, "myorg", got.OrgName)
	assert.Equal(t, "myrepo", got.RepoName)
}

func TestTryWriteReportsFileError(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	t.Setenv("HARNESS_CI_EXPLICIT_GIT_CLONE_STEP", "true")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", workdir)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_BUILD_EVENT", "push")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_TAG", "")
	err := TryWrite(workdir, Inputs{})
	require.ErrorContains(t, err, "failed to write explicit git clone metadata file")
}

func TestCommitSHAFallsBackToEnv(t *testing.T) {
	t.Setenv("DRONE_COMMIT_SHA", "abc123")
	assert.Equal(t, "abc123", resolveCommitSHA(t.TempDir()))
}

// The checked-out commit wins over DRONE_COMMIT_SHA: for a PR merge checkout the
// env SHA is the source head, not what git actually put in workdir.
func TestCommitSHAPrefersCheckedOutCommit(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	sha := git(t, workdir, "rev-parse", "HEAD")
	t.Setenv("DRONE_COMMIT_SHA", "deadbeef")
	assert.Equal(t, sha, resolveCommitSHA(workdir))
}

// DRONE_PULL_REQUEST is the manager's own number; it takes priority over the ref.
func TestResolvePRNumberFromEnv(t *testing.T) {
	t.Setenv("DRONE_PULL_REQUEST", "42")
	t.Setenv("DRONE_COMMIT_REF", "refs/pull/9/head")
	n, ok := resolvePRNumber()
	assert.True(t, ok)
	assert.Equal(t, int64(42), n)
}

func TestResolvePRNumberFromCommitRef(t *testing.T) {
	for _, ref := range []string{
		"refs/pull/17/head",
		"refs/pull-requests/17/from",
		"refs/merge-requests/17/head",
	} {
		t.Run(ref, func(t *testing.T) {
			t.Setenv("DRONE_PULL_REQUEST", "")
			t.Setenv("DRONE_COMMIT_REF", ref)
			n, ok := resolvePRNumber()
			assert.True(t, ok)
			assert.Equal(t, int64(17), n)
		})
	}
}

func TestResolvePRNumberAbsent(t *testing.T) {
	for _, test := range []struct {
		pr  string
		ref string
	}{
		{pr: "", ref: ""},
		{pr: "", ref: "refs/heads/main"},
		{pr: "", ref: "refs/tags/v1"},
		{pr: "abc", ref: ""}, // not a number
		{pr: "0", ref: ""},   // zero is not a PR number
		{pr: "", ref: "refs/pull/x/head"},
	} {
		t.Run(test.pr+"/"+test.ref, func(t *testing.T) {
			t.Setenv("DRONE_PULL_REQUEST", test.pr)
			t.Setenv("DRONE_COMMIT_REF", test.ref)
			_, ok := resolvePRNumber()
			assert.False(t, ok)
		})
	}
}

// A PR clone: prNumber lands in the receipt and refType resolves to pr.
// A real PR clone is detached, so the branch comes from the env; on
// pull_request events that is DRONE_SOURCE_BRANCH (the PR's own branch), not
// DRONE_COMMIT_BRANCH (the target branch).
func TestTryWritePullRequestReceipt(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\nb\n"})
	sha := git(t, workdir, "rev-parse", "HEAD")
	git(t, workdir, "checkout", "--detach", "HEAD")

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_BUILD_EVENT", "pull_request")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_SOURCE_BRANCH", "feature-17")
	t.Setenv("DRONE_PULL_REQUEST", "17")
	t.Setenv("DRONE_COMMIT_REF", "refs/pull/17/head")
	t.Setenv("DRONE_TAG", "")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "pr", got.RefType)
	assert.Equal(t, "feature-17", got.Branch)
	assert.Equal(t, "main", got.TargetBranch)
	require.NotNil(t, got.PRNumber)
	assert.Equal(t, int64(17), *got.PRNumber)
	assert.Equal(t, sha, got.CommitSHA)
	assert.Equal(t, "acme", got.OrgName)
	assert.Equal(t, "app", got.RepoName)
	assert.Greater(t, got.Timestamp, int64(0))
}

// Without DRONE_SOURCE_BRANCH a detached PR checkout still falls back to
// DRONE_COMMIT_BRANCH (the target branch).
func TestTryWritePullRequestDetachedNoSourceBranch(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	git(t, workdir, "checkout", "--detach", "HEAD")

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_BUILD_EVENT", "pull_request")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_COMMIT_REF", "refs/pull/9/head")
	t.Setenv("DRONE_TAG", "")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "pr", got.RefType)
	assert.Equal(t, "main", got.Branch)
	assert.Equal(t, "main", got.TargetBranch)
}

// DRONE_TARGET_BRANCH (the manager's explicit field) wins; DRONE_COMMIT_BRANCH
// is only the fallback when the explicit one is absent.
func TestTryWriteTargetBranchPrefersExplicitEnv(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_BUILD_EVENT", "pull_request")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_TARGET_BRANCH", "release-1.x")
	t.Setenv("DRONE_SOURCE_BRANCH", "feature-x")
	t.Setenv("DRONE_COMMIT_REF", "refs/pull/3/head")
	t.Setenv("DRONE_TAG", "")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, "feature-x", got.Branch)
	assert.Equal(t, "release-1.x", got.TargetBranch)
}

// targetBranch is a PR-only field: branch/tag checkouts never carry it, even
// though DRONE_COMMIT_BRANCH is set on those builds too.
func TestTryWriteOmitsTargetBranchForNonPR(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_BUILD_EVENT", "push")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_TAG", "")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	raw := string(mustRead(t, output))
	assert.NotContains(t, raw, "targetBranch")
	got := readMetadata(t, output)
	assert.Equal(t, "main", got.Branch)
	assert.Empty(t, got.TargetBranch)
}

func TestTryWriteSparseCheckoutAndSubmoduleStrategyFromEnv(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_NETRC_SPARSE_CHECKOUT", "src\n  docs/README.md\n\n")
	t.Setenv("DRONE_NETRC_SUBMODULE_STRATEGY", "recursive")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, []string{"src", "docs/README.md"}, got.SparseCheckout)
	assert.Equal(t, "recursive", got.SubmoduleStrategy)
}

func TestTryWriteOmitsSparseCheckoutAndSubmoduleStrategyWhenUnset(t *testing.T) {
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_NETRC_SPARSE_CHECKOUT", "")
	t.Setenv("DRONE_NETRC_SUBMODULE_STRATEGY", "")

	require.NoError(t, TryWrite(workdir, Inputs{}))
	raw := string(mustRead(t, output))
	assert.NotContains(t, raw, "sparseCheckout")
	assert.NotContains(t, raw, "submoduleStrategy")
	got := readMetadata(t, output)
	assert.Empty(t, got.SparseCheckout)
	assert.Empty(t, got.SubmoduleStrategy)
}

func TestParseSparseCheckout(t *testing.T) {
	t.Setenv("DRONE_NETRC_SPARSE_CHECKOUT", "src\nfolder with space\n")
	assert.Equal(t, []string{"src", "folder with space"}, parseSparseCheckout())

	t.Setenv("DRONE_NETRC_SPARSE_CHECKOUT", "   \n\n")
	assert.Nil(t, parseSparseCheckout())
}

// The clone scripts export HOME and GIT_CONFIG_GLOBAL only for themselves;
// without the fix, gitOutput's child git never sees the isolated .gitconfig
// carrying safe.directory '*' and fails on other-owned workspaces.
func TestGitEnv(t *testing.T) {
	t.Run("no isolation passes env through", func(t *testing.T) {
		t.Setenv("HARNESS_GIT_CONFIG_FOLDER", "")
		assert.Equal(t, os.Environ(), gitEnv())
	})

	t.Run("isolation appends and overrides inherited values", func(t *testing.T) {
		folder := t.TempDir()
		t.Setenv("HARNESS_GIT_CONFIG_FOLDER", folder)
		t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
		t.Setenv("HOME", "/inherited-home")

		env := gitEnv()
		assert.Equal(t, filepath.Join(folder, ".gitconfig"), lastEnvValue(env, "GIT_CONFIG_GLOBAL"))
		assert.Equal(t, folder, lastEnvValue(env, "HOME"))
		assert.Equal(t, folder, lastEnvValue(env, "USERPROFILE"))
	})
}

func TestGitOutputUsesIsolatedGitConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake git is a shell script")
	}
	folder := t.TempDir()
	t.Setenv("HARNESS_GIT_CONFIG_FOLDER", folder)
	// Inherited values must lose to the isolated ones.
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("HOME", "/inherited-home")

	fakeGit(t)
	out := gitOutput(t.TempDir(), "rev-parse", "HEAD")
	assert.Contains(t, out, "GLOBAL="+filepath.Join(folder, ".gitconfig"))
	assert.Contains(t, out, "HOME="+folder)
}

func TestGitOutputWithoutIsolationLeavesEnvUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake git is a shell script")
	}
	t.Setenv("HARNESS_GIT_CONFIG_FOLDER", "")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("HOME", "/inherited-home")

	fakeGit(t)
	out := gitOutput(t.TempDir(), "rev-parse", "HEAD")
	assert.Contains(t, out, "GLOBAL=/dev/null")
	assert.Contains(t, out, "HOME=/inherited-home")
}

func TestResolveCommitSHAHonoursIsolatedSafeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ownership simulation is unix-oriented")
	}
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	want := git(t, workdir, "rev-parse", "HEAD") // compute BEFORE the hook below

	folder := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(folder, ".gitconfig"),
		[]byte("[safe]\n\tdirectory = *\n"), 0o600))

	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	t.Setenv("DRONE_COMMIT_SHA", "")

	// Without isolation: ownership check blocks git.
	t.Setenv("HARNESS_GIT_CONFIG_FOLDER", "")
	require.Empty(t, resolveCommitSHA(workdir),
		"precondition: git must refuse without the isolated config (needs git >= 2.35.2)")

	// With isolation: safe.directory applies.
	t.Setenv("HARNESS_GIT_CONFIG_FOLDER", folder)
	assert.Equal(t, want, resolveCommitSHA(workdir))
}

func TestResolveCommitSHAPrefersRealHeadUnderOwnershipCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ownership simulation is unix-oriented")
	}
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	want := git(t, workdir, "rev-parse", "HEAD")

	folder := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(folder, ".gitconfig"),
		[]byte("[safe]\n\tdirectory = *\n"), 0o600))

	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	t.Setenv("HARNESS_GIT_CONFIG_FOLDER", folder)
	t.Setenv("DRONE_COMMIT_SHA", "deadbeef") // requested/source SHA, not the merge SHA

	assert.Equal(t, want, resolveCommitSHA(workdir))
}

func TestTryWriteWritesReceiptUnderOwnershipCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ownership simulation is unix-oriented")
	}
	workdir := initRepo(t, map[string]string{"f.txt": "a\n"})
	want := git(t, workdir, "rev-parse", "HEAD")

	folder := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(folder, ".gitconfig"),
		[]byte("[safe]\n\tdirectory = *\n"), 0o600))

	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	t.Setenv("HARNESS_GIT_CONFIG_FOLDER", folder)
	t.Setenv("HARNESS_GIT_CLONE_METADATA_FILE", output)
	t.Setenv("DRONE_REMOTE_URL", "https://github.com/acme/app.git")
	t.Setenv("DRONE_COMMIT_BRANCH", "main")
	t.Setenv("DRONE_COMMIT_SHA", "") // no requested SHA

	require.NoError(t, TryWrite(workdir, Inputs{}))
	got := readMetadata(t, output)
	assert.Equal(t, want, got.CommitSHA)
	assert.Equal(t, "main", got.Branch) // gitAbbrevRef also works under isolation
}

// fakeGit puts a script named git first on PATH that reports the environment
// its caller handed it.
func fakeGit(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf 'GLOBAL=%s\\nHOME=%s\\n' \"$GIT_CONFIG_GLOBAL\" \"$HOME\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func lastEnvValue(env []string, key string) string {
	value := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			value = strings.TrimPrefix(kv, key+"=")
		}
	}
	return value
}

func int64Ptr(v int64) *int64 { return &v }

func initRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_OPTIONAL_LOCKS", "0")

	for _, key := range []string{
		"DRONE_BUILD_EVENT",
		"DRONE_COMMIT_BRANCH",
		"DRONE_COMMIT_REF",
		"DRONE_COMMIT_SHA",
		"DRONE_PULL_REQUEST",
		"DRONE_REMOTE_URL",
		"DRONE_REPO_SCM",
		"DRONE_SOURCE_BRANCH",
		"DRONE_TAG",
		"DRONE_TARGET_BRANCH",
		"DRONE_NETRC_SPARSE_CHECKOUT",
		"DRONE_NETRC_SUBMODULE_STRATEGY",
	} {
		t.Setenv(key, "")
	}

	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "gc.auto", "0")
	git(t, dir, "config", "maintenance.auto", "false")
	git(t, dir, "config", "core.fsmonitor", "false")
	git(t, dir, "config", "core.untrackedCache", "false")
	git(t, dir, "config", "commit.gpgsign", "false")
	for name, contents := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600))
	}
	git(t, dir, "add", ".")
	git(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fixture")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return strings.TrimSpace(string(out))
}

func readMetadata(t *testing.T, path string) Metadata {
	t.Helper()
	var got Metadata
	require.NoError(t, json.Unmarshal(mustRead(t, path), &got))
	return got
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
