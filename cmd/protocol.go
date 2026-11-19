package cmd

import (
	"debris/internal/protocol"
	"debris/pkg/debris"

	"github.com/spf13/cobra"
)

func newProtocolCmd(eng *debris.Engine) *cobra.Command {
	return &cobra.Command{
		Use:   "protocol",
		Short: "Run the stateless JSON-RPC 2.0 stdio server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return protocol.NewServer(eng).Serve(commandContext(cmd), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
}
