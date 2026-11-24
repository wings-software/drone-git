// Package clonemetadata writes the Git Clone receipt (CI-25945).
//
// drone-git calls TryWrite immediately after a successful clone. The CI addon
// forwards the file unparsed, so the JSON names are the contract with the manager.
package clonemetadata

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/slog"
)

// Metadata is the JSON receipt written to HARNESS_GIT_CLONE_METADATA_FILE.
type Metadata struct {
	RepoURL     string `json:"repoUrl"`
	CommitSHA   string `json:"commitSha"`
	RefType     string `json:"refType,omitempty"`
	PRNumber    *int64 `json:"prNumber,omitempty"`
	LinesOfCode *int64 `json:"linesOfCode,omitempty"`
	FileCount   *int64 `json:"fileCount,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Tag         string `json:"tag,omitempty"`
	RepoName    string `json:"repoName,omitempty"`
	OrgName     string `json:"orgName,omitempty"`
	Timestamp   int64  `json:"timestamp"`
	SCMType     string `json:"scmType,omitempty"`
}

// Inputs carries values the drone-git binary already computes for its own
// telemetry pass; reusing them keeps one definition of lines-of-code and ref
// type per clone instead of the receipt counting and classifying again.
type Inputs struct {
	// LinesOfCode and FileCount are scc totals from the binary's metrics pass
	// (metrics.Lines / metrics.Files). Nil when the pass was disabled
	// (DISABLE_SCC_METRICS / CI_DISABLE_TELEMETRY) or failed, which omits the
	// fields from the receipt. FileCount is files scc analysed, not git ls-files:
	// vendor, node_modules, .git and oversized files are excluded, same as loc.
	LinesOfCode *int64
	FileCount   *int64
	// BuildEvent is the binary's getBuildEventInfo classification of
	// DRONE_BUILD_EVENT: tag, pull_request, branch, push or commit.
	BuildEvent string
}

// TryWrite writes the clone receipt for a Git Clone step (explicit or implicit).
//
// It is a no-op unless HARNESS_GIT_CLONE_METADATA_FILE is set; the addon
// injects that path on every step container, and only drone-git writes the file.
// refType comes from the caller's build-event classification (the same source
// the clone scripts' CLONE_TYPE uses). The caller logs errors and does not fail
// the step.
func TryWrite(workdir string, in Inputs) error {
	metadataFile := os.Getenv("HARNESS_GIT_CLONE_METADATA_FILE")
	if metadataFile == "" {
		slog.Debug("No HARNESS_GIT_CLONE_METADATA_FILE specified, skipping clone metadata")
		return nil
	}

	// Stamp before the git lookups so ref resolution cannot move the timestamp
	// past the moment the clone itself finished.
	timestamp := time.Now().UnixMilli()

	scmType := strings.TrimSpace(os.Getenv("DRONE_REPO_SCM"))

	repoURL := trimGitSuffix(sanitizeRepoURL(os.Getenv("DRONE_REMOTE_URL")))
	commitSHA := resolveCommitSHA(workdir)
	if repoURL == "" || commitSHA == "" {
		slog.Warn("repo URL or commit SHA missing, skipping clone metadata",
			"hasRepoUrl", repoURL != "", "hasCommitSha", commitSHA != "")
		return nil
	}

	branch, tag := resolveBranchAndTag(workdir)
	var prNumberPtr *int64
	if n, ok := resolvePRNumber(); ok {
		prNumberPtr = &n
	}
	orgName, repoName := parseRepoIdentity(repoURL, scmType)

	metadata := Metadata{
		RepoURL:     repoURL,
		CommitSHA:   commitSHA,
		RefType:     refTypeForEvent(in.BuildEvent),
		PRNumber:    prNumberPtr,
		LinesOfCode: in.LinesOfCode,
		FileCount:   in.FileCount,
		Branch:      branch,
		Tag:         tag,
		RepoName:    repoName,
		OrgName:     orgName,
		Timestamp:   timestamp,
		SCMType:     scmType,
	}

	jsonData, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal explicit git clone metadata: %v", err)
	}
	if err := os.WriteFile(metadataFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write explicit git clone metadata file %s: %v", metadataFile, err)
	}

	slog.Info("Wrote git clone metadata",
		"repoUrl", repoURL,
		"commitSha", commitSHA,
		"refType", metadata.RefType,
		"PRNumber", metadata.PRNumber,
		"SCMType", metadata.SCMType,
		"linesOfCode", metadata.LinesOfCode,
		"fileCount", metadata.FileCount,
		"branch", branch,
		"tag", tag,
		"repoName", repoName,
		"orgName", orgName,
		"timestamp", timestamp,
	)
	return nil
}

// resolveBranchAndTag reports the ref this clone actually checked out.
// A tag checkout (DRONE_BUILD_EVENT=tag, or refs/tags/...) leaves HEAD detached,
// so the branch is empty and the tag is the one that was fetched. A PR checkout
// is detached too; there the branch is the PR's source branch from the env.
func resolveBranchAndTag(workdir string) (string, string) {
	envBranch := strings.TrimSpace(os.Getenv("DRONE_COMMIT_BRANCH"))
	envTag := strings.TrimSpace(os.Getenv("DRONE_TAG"))
	ref := strings.TrimSpace(os.Getenv("DRONE_COMMIT_REF"))
	checkedOut := gitAbbrevRef(workdir)

	tagCheckout := os.Getenv("DRONE_BUILD_EVENT") == "tag" || strings.HasPrefix(ref, "refs/tags/")
	if !tagCheckout && envTag != "" && envBranch == "" && checkedOut == "" {
		tagCheckout = true
	}
	if tagCheckout {
		tag := envTag
		if tag == "" && strings.HasPrefix(ref, "refs/tags/") {
			tag = strings.TrimPrefix(ref, "refs/tags/")
		}
		return "", tag
	}

	if checkedOut == "" {
		if os.Getenv("DRONE_BUILD_EVENT") == "pull_request" {
			if src := strings.TrimSpace(os.Getenv("DRONE_SOURCE_BRANCH")); src != "" {
				return src, ""
			}
		}
		return envBranch, ""
	}
	return checkedOut, ""
}

// resolveCommitSHA returns the commit the clone scripts actually checked out.
// DRONE_COMMIT_SHA is only the requested commit: it is empty for branch builds
// and differs for pull-request merge checkouts, so ask git first.
func resolveCommitSHA(workdir string) string {
	if sha := gitOutput(workdir, "rev-parse", "HEAD"); sha != "" {
		return sha
	}
	return strings.TrimSpace(os.Getenv("DRONE_COMMIT_SHA"))
}

func gitAbbrevRef(workdir string) string {
	ref := gitOutput(workdir, "rev-parse", "--abbrev-ref", "HEAD")
	if ref == "" || ref == "HEAD" {
		return ""
	}
	return ref
}

func gitOutput(workdir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// refTypeForEvent maps the binary's build-event classification (getBuildEventInfo,
// the same source the clone scripts' CLONE_TYPE uses) onto the receipt's wire values.
func refTypeForEvent(buildEvent string) string {
	switch buildEvent {
	case "tag":
		return "tag"
	case "pull_request":
		return "pr"
	default: // branch, push, commit, empty
		return "branch"
	}
}

func sanitizeRepoURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u == nil || u.User == nil {
		return rawURL
	}
	if _, hasPassword := u.User.Password(); !hasPassword {
		return rawURL
	}
	u.User = nil
	return u.String()
}

// scmType values sent by the CI manager via DRONE_REPO_SCM (CiCodebaseUtils).
const (
	scmGithub     = "Github"
	scmGitlab     = "Gitlab"
	scmBitbucket  = "Bitbucket"
	scmAzureRepo  = "AzureRepo"
	scmCodeCommit = "AWSCodeCommit"
	scmHarness    = "Harness"
	scmGit        = "Git"
)

// parseRepoIdentity splits a clone URL into the VCS owner and repository name.
// scmType (DRONE_REPO_SCM) identifies which path segments are provider furniture
// (Azure's "_git"/"v3", Bitbucket Server's "scm", CodeCommit's "v1/repos") so they
// are not mistaken for owners. An empty or unrecognized scmType uses the legacy
// host-blind rules, so older managers see no behavior change.
func parseRepoIdentity(rawURL, scmType string) (orgName, repoName string) {
	path := strings.Trim(repoPath(rawURL), "/")
	if path == "" {
		return "", ""
	}
	switch scmType {
	case scmAzureRepo:
		return parseAzureIdentity(path)
	case scmBitbucket:
		return parseBitbucketIdentity(path)
	case scmCodeCommit:
		return parseCodeCommitIdentity(path)
	case scmGithub, scmGitlab, scmHarness, scmGit:
		return parseGenericIdentity(path)
	default:
		return parseLegacyIdentity(path)
	}
}

// parseGenericIdentity: the last segment is the repo, everything before it is the
// owner (gitlab.com/group/sub/app -> group/sub). No provider furniture.
func parseGenericIdentity(path string) (orgName, repoName string) {
	parts := strings.Split(path, "/")
	repoName = trimGitSuffix(parts[len(parts)-1])
	if len(parts) > 1 {
		orgName = strings.Join(parts[:len(parts)-1], "/")
	}
	return orgName, repoName
}

// parseAzureIdentity handles dev.azure.com/<org>/<project>/_git/<repo> and the
// scp-style SSH form v3/<org>/<project>/_git/<repo>. The Azure project is dropped;
// the org is the first segment after the optional v3/ prefix.
func parseAzureIdentity(path string) (orgName, repoName string) {
	i := strings.Index(strings.ToLower(path), "/_git/")
	if i < 0 {
		// Azure DevOps SSH clone URLs (git@ssh.dev.azure.com:v3/<org>/<project>/<repo>)
		// carry no _git landmark; after the v3/ prefix the shape is fixed, with the
		// project in the middle dropped as above.
		if strings.HasPrefix(path, "v3/") {
			parts := strings.Split(strings.TrimPrefix(path, "v3/"), "/")
			if len(parts) >= 3 {
				return parts[0], trimGitSuffix(parts[len(parts)-1])
			}
		}
		return parseGenericIdentity(path)
	}
	repoName = firstSegment(trimGitSuffix(path[i+len("/_git/"):]))
	ownerPath := strings.TrimPrefix(strings.Trim(path[:i], "/"), "v3/")
	orgName = firstSegment(ownerPath)
	return orgName, repoName
}

// parseBitbucketIdentity handles bitbucket.org/<workspace>/<repo> and Server/DC
// URLs [context/]scm/<PROJECT>/<repo>: "scm" is a marker segment, so everything up
// to and including it is deployment prefix, not identity.
func parseBitbucketIdentity(path string) (orgName, repoName string) {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "scm" && len(parts)-i >= 3 { // marker needs PROJECT + repo after it
			return parseGenericIdentity(strings.Join(parts[i+1:], "/"))
		}
	}
	return parseGenericIdentity(path)
}

// parseCodeCommitIdentity handles .../v1/repos/<repo>. CodeCommit has no owner
// concept, so the org stays empty.
func parseCodeCommitIdentity(path string) (orgName, repoName string) {
	return "", trimGitSuffix(strings.TrimPrefix(path, "v1/repos/"))
}

// parseLegacyIdentity is the pre-scmType behavior, kept for receipts where
// DRONE_REPO_SCM is absent or unrecognized.
func parseLegacyIdentity(path string) (orgName, repoName string) {
	if i := strings.Index(strings.ToLower(path), "/_git/"); i >= 0 {
		repoName = firstSegment(trimGitSuffix(path[i+len("/_git/"):]))
		orgName = firstSegment(path[:i])
		return orgName, repoName
	}
	parts := strings.Split(path, "/")
	if len(parts) > 1 && parts[0] == "scm" {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "", ""
	}
	return parseGenericIdentity(strings.Join(parts, "/"))
}

func repoPath(rawURL string) string {
	if strings.Contains(rawURL, "://") {
		u, err := url.Parse(rawURL)
		if err == nil && u != nil {
			return u.Path
		}
		// url.Parse rejects authorities with a non-numeric "port", e.g. Azure
		// DevOps SSH (ssh://org@vs-ssh.visualstudio.com:v3/org/project/_git/repo).
		// Cut the path off manually: everything from the first "/" after "://".
		rest := rawURL[strings.Index(rawURL, "://")+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			return rest[slash:]
		}
		return ""
	}
	// scp-style: git@host:org/repo.git
	if at := strings.Index(rawURL, "@"); at >= 0 {
		if colon := strings.Index(rawURL[at:], ":"); colon >= 0 {
			return rawURL[at+colon+1:]
		}
	}
	return rawURL
}

func firstSegment(path string) string {
	path = strings.Trim(path, "/")
	if i := strings.Index(path, "/"); i >= 0 {
		return path[:i]
	}
	return path
}

func trimGitSuffix(name string) string {
	return strings.TrimSuffix(strings.Trim(name, "/"), ".git")
}

// resolvePRNumber returns the pull request number for a PR checkout.
// DRONE_PULL_REQUEST wins; otherwise it is parsed from DRONE_COMMIT_REF
// (refs/pull/N/..., refs/pull-requests/N/..., refs/merge-requests/N/...).
func resolvePRNumber() (int64, bool) {
	if s := strings.TrimSpace(os.Getenv("DRONE_PULL_REQUEST")); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			return n, true
		}
	}
	ref := strings.TrimSpace(os.Getenv("DRONE_COMMIT_REF"))
	for _, prefix := range []string{"refs/pull/", "refs/pull-requests/", "refs/merge-requests/"} {
		if strings.HasPrefix(ref, prefix) {
			rest := strings.TrimPrefix(ref, prefix)
			if i := strings.Index(rest, "/"); i >= 0 {
				rest = rest[:i]
			}
			if n, err := strconv.ParseInt(rest, 10, 64); err == nil && n > 0 {
				return n, true
			}
		}
	}
	return 0, false
}
