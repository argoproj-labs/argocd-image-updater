package argocd

// UpdaterConfig holds configuration for image updating
type UpdaterConfig struct {
	// DryRun if true, do not modify anything
	DryRun bool
	// MaxConcurrency is the maximum number of concurrent update operations
	MaxConcurrency int
	// GitCommitUser is the user name to use for Git commits
	GitCommitUser string
	// GitCommitEmail is the email to use for Git commits
	GitCommitEmail string
	// GitCommitMessage is the template for Git commit messages
	GitCommitMessage string
	// GitCommitSigningKey is the key to use for signing Git commits
	GitCommitSigningKey string
	// GitCommitSigningMethod is the method to use for signing Git commits
	GitCommitSigningMethod string
	// GitCommitSignOff if true, add sign-off line to Git commits
	GitCommitSignOff bool
}

// NewUpdaterConfig creates a new UpdaterConfig with default values
func NewUpdaterConfig() *UpdaterConfig {
	return &UpdaterConfig{
		DryRun:                 false,
		MaxConcurrency:         10,
		GitCommitUser:          "argocd-image-updater",
		GitCommitEmail:         "noreply@argoproj.io",
		GitCommitMessage:       "Update image version",
		GitCommitSigningKey:    "",
		GitCommitSigningMethod: "openpgp",
		GitCommitSignOff:       false,
	}
}
