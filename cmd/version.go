package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/argoproj-labs/argocd-image-updater/pkg/version"
)

// newVersionCommand implements "version" command
func newVersionCommand() *cobra.Command {
	var short bool
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if !short {
				fmt.Fprintf(out, "%s\n", version.Useragent())
				fmt.Fprintf(out, "  BuildDate: %s\n", version.BuildDate())
				fmt.Fprintf(out, "  GitCommit: %s\n", version.GitCommit())
				fmt.Fprintf(out, "  GoVersion: %s\n", version.GoVersion())
				fmt.Fprintf(out, "  GoCompiler: %s\n", version.GoCompiler())
				fmt.Fprintf(out, "  Platform: %s\n", version.GoPlatform())
			} else {
				fmt.Fprintf(out, "%s\n", version.Version())
			}
			return nil
		},
	}
	versionCmd.Flags().BoolVar(&short, "short", false, "show only the version number")
	return versionCmd
}
