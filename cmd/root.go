package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"syscall"

	"debris/internal/engine"
	"debris/internal/format/billfish"
	"debris/internal/format/eagle"
	"debris/internal/format/pixcall"
	"debris/pkg/debris"

	"github.com/spf13/cobra"
)

// defaultEngine constructs the default Debris Engine wired to all format adapters.
var defaultEngine = func() *debris.Engine {
	reg := engine.NewRegistry(
		billfish.New(),
		eagle.New(),
		pixcall.New(),
	)
	return debris.NewEngine(debris.WithRunner(engine.NewRunner(reg)))
}

// NewRootCmd builds the root Debris CLI command wired to eng.
func NewRootCmd(eng *debris.Engine) *cobra.Command {
	if eng == nil {
		eng = defaultEngine()
	}

	var rootConvertOpts convertOptions

	rootCmd := &cobra.Command{
		Use:   "debris",
		Short: "Convert and inspect design asset libraries across formats",
		Long:  "Debris inspects, plans, and converts design asset libraries between formats such as Billfish, Eagle, and Pixcall.",
		Example: `  debris --from billfish --to eagle --input ./lib.billfish --output ./lib.library
  debris convert -f billfish -t pixcall -i ./lib.billfish -o ./pixcall-lib --json
  debris inspect --path ./lib.library --json
  debris plan --to eagle --input ./lib.billfish --output ./lib.library
  debris protocol`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hasAnyConvertFlagChanged(cmd) {
				return cmd.Help()
			}
			cmd.SilenceUsage = true
			return executeConvert(cmd, eng, rootConvertOpts)
		},
	}

	bindConvertFlags(rootCmd, &rootConvertOpts)

	rootCmd.AddCommand(
		newConvertCmd(eng),
		newInspectCmd(eng),
		newPlanCmd(eng),
		newProtocolCmd(eng),
	)

	return rootCmd
}

// Execute runs the Debris CLI with OS signal cancellation handling.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd := NewRootCmd(defaultEngine())
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd != nil && cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

type jsonErrorEnvelope struct {
	Error *debris.Error `json:"error"`
}

func writeJSONError(w io.Writer, err error) {
	_ = writeJSON(w, jsonErrorEnvelope{Error: debris.AsError(err)})
}
