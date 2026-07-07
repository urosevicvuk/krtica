package cli

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/urosevicvuk/krtica/internal/agent"
	"github.com/urosevicvuk/krtica/internal/config"
	"github.com/urosevicvuk/krtica/internal/server"
)

func New() *cobra.Command {
	root := &cobra.Command{
		Use:           "krtica",
		Short:         "krtica — self-hosted reverse tunnel (the mole digs outward)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(serverCmd(), agentCmd())
	return root
}

func serverCmd() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run the public-facing molehill",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.LoadServer(cfgPath)
			if err != nil {
				return err
			}
			return server.New(cfg, logger()).Run(cmd.Context())
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "server.yaml", "path to server config")
	return cmd
}

func agentCmd() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run the behind-NAT mole",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.LoadAgent(cfgPath)
			if err != nil {
				return err
			}
			return agent.New(cfg, logger()).Run(cmd.Context())
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "agent.yaml", "path to agent config")
	return cmd
}

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}
