package git

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
)

// CommitOptions holds options for a git commit operation
type CommitOptions struct {
	// CommitMessageText holds a short commit message (-m option)
	CommitMessageText string
	// CommitMessagePath holds the path to a file to be used for the commit message (-F option)
	CommitMessagePath string
	// SigningKey holds a GnuPG key ID or path to a Private SSH Key used to sign the commit with (-S option)
	SigningKey string
	// SigningMethod holds the signing method used to sign commits. (git -c gpg.format=ssh option)
	SigningMethod string
	// SignOff specifies whether to sign-off a commit (-s option)
	SignOff bool
}

// Commit perfoms a git commit for the given pathSpec to the currently checked
// out branch. If pathSpec is empty, or the special value "*", all pending
// changes will be commited. If message is not the empty string, it will be
// used as the commit message, otherwise a default commit message will be used.
// If signingKey is not the empty string, commit will be signed with the given
// GPG or SSH key.
func (m *nativeGitClient) Commit(pathSpec string, opts *CommitOptions) error {
	defaultCommitMsg := "Update parameters"
	// Git configuration
	config := "gpg.format=" + opts.SigningMethod
	args := []string{}
	// -c is a global option and needs to be passed before the actual git sub
	// command (commit).
	if opts.SigningMethod != "" {
		args = append(args, "-c", config)
	}
	args = append(args, "commit")
	if pathSpec == "" || pathSpec == "*" {
		args = append(args, "-a")
	}
	// Commit fails with a space between -S flag and path to SSH key
	// -S/user/test/.ssh/signingKey or -SAAAAAAAA...
	if opts.SigningKey != "" {
		args = append(args, fmt.Sprintf("-S%s", opts.SigningKey))
	}
	if opts.SignOff {
		args = append(args, "-s")
	}
	if opts.CommitMessageText != "" {
		args = append(args, "-m", opts.CommitMessageText)
	} else if opts.CommitMessagePath != "" {
		args = append(args, "-F", opts.CommitMessagePath)
	} else {
		args = append(args, "-m", defaultCommitMsg)
	}

	out, err := m.runCmd(args...)
	if err != nil {
		log.Errorf(out)
		return err
	}

	return nil
}

// Branch creates a new target branch from a given source branch
func (m *nativeGitClient) Branch(sourceBranch string, targetBranch string) error {
	if sourceBranch != "" {
		_, err := m.runCmd("checkout", sourceBranch)
		if err != nil {
			return fmt.Errorf("could not checkout source branch: %v", err)
		}
	}

	_, err := m.runCmd("branch", targetBranch)
	if err != nil {
		return fmt.Errorf("could not create new branch: %v", err)
	}

	return nil
}

// Push pushes local changes to the remote branch. If force is true, will force
// the remote to accept the push.
func (m *nativeGitClient) Push(remote string, branch string, force bool) error {
	args := []string{"push"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, remote, branch)
	err := m.runCredentialedCmd(args...)
	if err != nil {
		return fmt.Errorf("could not push %s to %s: %v", branch, remote, err)
	}
	return nil
}

// Add adds a path spec to the repository
func (m *nativeGitClient) Add(path string) error {
	return m.runCredentialedCmd("add", path)
}

// SymRefToBranch retrieves the branch name a symbolic ref points to
func (m *nativeGitClient) SymRefToBranch(symRef string) (string, error) {
	output, err := m.runCredentialedCmdWithOutput("remote", "show", "origin")
	if err != nil {
		return "", fmt.Errorf("error running git: %v", err)
	}
	for _, l := range strings.Split(output, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "HEAD branch:") {
			b := strings.SplitN(l, ":", 2)
			if len(b) == 2 {
				return strings.TrimSpace(b[1]), nil
			}
		}
	}
	return "", fmt.Errorf("no default branch found in remote")
}

// Config configures username and email address for the repository
func (m *nativeGitClient) Config(username string, email string) error {
	_, err := m.runCmd("config", "user.name", username)
	if err != nil {
		return fmt.Errorf("could not set git username: %v", err)
	}
	_, err = m.runCmd("config", "user.email", email)
	if err != nil {
		return fmt.Errorf("could not set git email: %v", err)
	}

	return nil
}

// runCredentialedCmdWithOutput is a convenience function to run a git command
// with username/password credentials while supplying command output to the
// caller.
// nolint:unparam
func (m *nativeGitClient) runCredentialedCmdWithOutput(args ...string) (string, error) {
	closer, environ, err := m.creds.Environ()
	if err != nil {
		return "", err
	}
	defer func() { _ = closer.Close() }()

	// If a basic auth header is explicitly set, tell Git to send it to the
	// server to force use of basic auth instead of negotiating the auth scheme
	for _, e := range environ {
		if strings.HasPrefix(e, fmt.Sprintf("%s=", forceBasicAuthHeaderEnv)) {
			args = append([]string{"--config-env", fmt.Sprintf("http.extraHeader=%s", forceBasicAuthHeaderEnv)}, args...)
		}
	}

	cmd := exec.Command("git", args...)
	cmd.Env = append(cmd.Env, environ...)
	return m.runCmdOutput(cmd, runOpts{})
}
