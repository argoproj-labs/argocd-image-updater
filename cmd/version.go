package main

import (
	"fmt"

	"github.com/argoproj-labs/argocd-image-updater/pkg/version"

	"github.com/spf13/cobra"
)

// newVersionCommand implements "version" command
func newVersionCommand() *cobra.Command {
	var short bool
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !short {
				fmt.Printf("%s\n", version.Useragent())
				fmt.Printf("  BuildDate: %s\n", version.BuildDate())
				fmt.Printf("  GitCommit: %s\n", version.GitCommit())
				fmt.Printf("  GoVersion: %s\n", version.GoVersion())
				fmt.Printf("  GoCompiler: %s\n", version.GoCompiler())
				fmt.Printf("  Platform: %s\n", version.GoPlatform())
			} else {
				fmt.Printf("%s\n", version.Version())
			}
			return nil
		},
	}
	versionCmd.Flags().BoolVar(&short, "short", false, "show only the version number")
	return versionCmd
}
