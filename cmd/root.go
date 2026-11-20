package cmd

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
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

// Build metadata injected via GoReleaser / -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func versionString() string {
	if Commit == "" || Commit == "none" {
		return Version
	}
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}

// defaultEngine constructs the default Debris Engine wired to all format adapters.
var defaultEngine = func() *debris.Engine {
	reg := engine.NewRegistry(
		billfish.New(),
		eagle.New(),
		pixcall.New(),
	)
	return debris.NewEngine(debris.WithRunner(engine.NewRunner(reg)))
}

// NewDefaultEngine constructs the default Debris Engine wired to all format adapters.
func NewDefaultEngine() *debris.Engine {
	return defaultEngine()
}

// NewRootCmd builds the root Debris CLI command wired to eng.
func NewRootCmd(eng *debris.Engine) *cobra.Command {
	if eng == nil {
		eng = defaultEngine()
	}

	rootCmd := &cobra.Command{
		Use:     "debris",
		Short:   "Convert and inspect design asset libraries across formats",
		Long:    "Debris inspects, plans, and converts design asset libraries between formats such as Billfish, Eagle, and Pixcall.",
		Version: versionString(),
		Example: `  debris convert ./lib.billfish ./lib.library -t eagle
  debris convert ./lib.billfish -t pixcall --json
  debris inspect ./lib.library --json
  debris plan ./lib.billfish -t eagle`,
	}

	rootCmd.AddCommand(
		newConvertCmd(eng),
		newInspectCmd(eng),
		newPlanCmd(eng),
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
	enc := jsontext.NewEncoder(w, jsontext.WithIndent("  "))
	return json.MarshalEncode(enc, v)
}

type jsonErrorEnvelope struct {
	Error *debris.Error `json:"error"`
}

func writeJSONError(w io.Writer, err error) {
	_ = writeJSON(w, jsonErrorEnvelope{Error: debris.AsError(err)})
}
