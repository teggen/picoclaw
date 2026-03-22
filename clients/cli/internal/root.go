package internal

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/clients/cli/internal/chat"
	"github.com/sipeed/picoclaw/clients/cli/internal/configcmd"
	"github.com/sipeed/picoclaw/clients/cli/internal/events"
	"github.com/sipeed/picoclaw/clients/cli/internal/list"
	"github.com/sipeed/picoclaw/clients/cli/internal/metrics"
	"github.com/sipeed/picoclaw/clients/cli/internal/session"
	"github.com/sipeed/picoclaw/clients/cli/internal/status"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "picoclaw-cli",
		Short:        "CLI client for a running PicoClaw gateway",
		SilenceUsage: true,
	}

	cmd.PersistentFlags().StringP("gateway-url", "g", "", "Gateway base URL (env: PICOCLAW_GATEWAY_URL)")
	cmd.PersistentFlags().BoolP("json", "j", false, "Raw JSON output for piping/scripting")
	cmd.PersistentFlags().DurationP("timeout", "t", 30*time.Second, "HTTP request timeout")

	cmd.AddCommand(
		NewVersionCommand(),
		status.NewCommand(),
		list.NewCommand(),
		session.NewCommand(),
		configcmd.NewCommand(),
		chat.NewCommand(),
		events.NewCommand(),
		metrics.NewCommand(),
	)

	return cmd
}
