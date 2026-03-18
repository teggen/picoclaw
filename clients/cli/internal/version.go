package internal

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	CLIVersion = "dev"
	CLICommit  = "unknown"
)

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show CLI version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("picoclaw-cli %s (%s)\n", CLIVersion, CLICommit)
		},
	}
}
