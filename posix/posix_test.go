package posix

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommits(t *testing.T) {
	remote := fixtureRemote(t)

	base, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.Remove(base)

	for i, test := range tests {
		local := filepath.Join(base, fmt.Sprint(i))
		err = os.MkdirAll(local, 0777)
		if err != nil {
			t.Error(err)
			return
		}
		initGit(t, local, remote)

		bin, err := filepath.Abs("clone-commit")
		if err != nil {
			t.Error(err)
			return
		}

		cmd := exec.Command(bin)
		cmd.Dir = local
		cmd.Env = scriptEnv(
			fmt.Sprintf("DRONE_COMMIT_BRANCH=%s", test.branch),
			fmt.Sprintf("DRONE_COMMIT_SHA=%s", test.commit),
			fmt.Sprintf("DRONE_WORKSPACE=%s", local),
			fmt.Sprintf("DRONE_REMOTE_URL=%s", remote),
		)

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Error(err)
			t.Log(string(out))
			return
		}

		commit, err := getCommit(local)
		if err != nil {
			t.Error(err)
			return
		}

		branch, err := getBranch(local)
		if err != nil {
			t.Error(err)
			return
		}

		if want, got := test.commit, commit; got != want {
			t.Errorf("Want commit %s, got %s", want, got)
		}

		if want, got := test.branch, branch; got != want {
			t.Errorf("Want branch %s, got %s", want, got)
		}

		file := filepath.Join(local, test.file)
		out, err = ioutil.ReadFile(file)
		if err != nil {
			t.Error(err)
			return
		}

		if want, got := test.text, string(out); want != got {
			t.Errorf("Want file content %q, got %q", want, got)
		}
	}
}

func TestTags(t *testing.T) {
	remote := fixtureRemote(t)

	base, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.Remove(base)

	for i, test := range tests {
		local := filepath.Join(base, fmt.Sprint(i))
		err = os.MkdirAll(local, 0777)
		if err != nil {
			t.Error(err)
			return
		}
		initGit(t, local, remote)

		bin, err := filepath.Abs("clone-tag")
		if err != nil {
			t.Error(err)
			return
		}

		cmd := exec.Command(bin)
		cmd.Dir = local
		cmd.Env = scriptEnv(
			fmt.Sprintf("DRONE_TAG=%s", test.tag),
			fmt.Sprintf("DRONE_COMMIT_SHA=%s", test.commit),
			fmt.Sprintf("DRONE_WORKSPACE=%s", local),
			fmt.Sprintf("DRONE_REMOTE_URL=%s", remote),
		)

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Error(err)
			t.Log(string(out))
			return
		}

		commit, err := getCommit(local)
		if err != nil {
			t.Error(err)
			return
		}

		if want, got := test.commit, commit; got != want {
			t.Errorf("Want commit %s, got %s", want, got)
		}

		file := filepath.Join(local, test.file)
		out, err = ioutil.ReadFile(file)
		if err != nil {
			t.Error(err)
			return
		}

		if want, got := test.text, string(out); want != got {
			t.Errorf("Want file content %q, got %q", want, got)
		}
	}
}

func TestPullRequest(t *testing.T) {
	remote := "https://github.com/octocat/Spoon-Knife.git"

	local, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.Remove(local)
	initGit(t, local, remote)

	bin, err := filepath.Abs("clone-pull-request")
	if err != nil {
		t.Error(err)
		return
	}

	cmd := exec.Command(bin)
	cmd.Dir = local
	cmd.Env = scriptEnv(
		fmt.Sprintf("DRONE_COMMIT_REF=%s", "refs/pull/14596/head"),
		fmt.Sprintf("DRONE_COMMIT_BRANCH=%s", "main"),
		fmt.Sprintf("DRONE_COMMIT_SHA=%s", "26923a8f37933ccc23943de0d4ebd53908268582"),
		fmt.Sprintf("DRONE_WORKSPACE=%s", local),
		fmt.Sprintf("DRONE_REMOTE_URL=%s", remote),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Error(err)
		t.Log(string(out))
		return
	}

	commit, err := getCommit(local)
	if err != nil {
		t.Error(err)
		return
	}

	branch, err := getBranch(local)
	if err != nil {
		t.Error(err)
		return
	}

	if want, got := "26923a8f37933ccc23943de0d4ebd53908268582", commit; got != want {
		t.Errorf("Want commit %s, got %s", want, got)
	}

	if want, got := "main", branch; got != want {
		t.Errorf("Want branch %s, got %s", want, got)
	}

	file := filepath.Join(local, "directory/file.txt")
	out, err = ioutil.ReadFile(file)
	if err != nil {
		t.Error(err)
		return
	}
}

// fixtureRemote unpacks posix/fixtures.tar into a temp directory and returns the greeting repo.
func fixtureRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("tar", "-xf", "fixtures.tar", "-C", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("extract fixtures.tar: %v\n%s", err, out)
	}
	return filepath.Join(dir, "tmp", "remote", "greeting")
}

// initGit prepares the workspace the way posix/common does before clone-commit,
// clone-tag, and clone-pull-request run.
func initGit(t *testing.T, dir, remote string) {
	t.Helper()
	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "drone@localhost"},
		{"git", "config", "user.name", "drone"},
		{"git", "remote", "add", "origin", remote},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// scriptEnv keeps PATH and HOME so git can run, and does not inherit CI DRONE_* or PLUGIN_* values.
func scriptEnv(extra ...string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=*",
		"GIT_AUTHOR_NAME=drone",
		"GIT_AUTHOR_EMAIL=drone@localhost",
		"GIT_COMMITTER_NAME=drone",
		"GIT_COMMITTER_EMAIL=drone@localhost",
	}
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		env = append(env, "TMPDIR="+tmp)
	}
	return append(env, extra...)
}

func getBranch(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func getCommit(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

var tests = []struct {
	branch string
	commit string
	tag    string
	file   string
	text   string
}{
	{
		commit: "9cd29dca0a98f76df94d66493ee54788a18190a0",
		branch: "master",
		tag:    "v1.0.0",
		file:   "hello.txt",
		text:   "hi world\n",
	},
	{
		commit: "bbdf5d4028a6066431f59fcd8d83afff610a55ae",
		branch: "master",
		tag:    "v1.1.0",
		file:   "hello.txt",
		text:   "hello world\n",
	},
	{
		commit: "553af1ca53c9ad54b096d7ff1416f6c4d1e5049f",
		branch: "fr",
		tag:    "v2.0.0",
		file:   "hello.txt",
		text:   "salut monde\n",
	},
	{
		commit: "94b4a1710d1581b8b00c5f7b077026eae3c07646",
		branch: "fr",
		tag:    "v2.1.0",
		file:   "hello.txt",
		text:   "bonjour monde\n",
	},
}
