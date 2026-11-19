package cmd

import (
	"encoding/json"
	"fmt"
	"sync"

	"debris/pkg/debris"

	"github.com/spf13/cobra"
)

type convertOptions struct {
	From           string
	To             string
	InputPath      string
	OutputPath     string
	CopyMode       string
	DryRun         bool
	Overwrite      bool
	IncludeDeleted bool
	IncludeHidden  bool
	LibraryName    string
	JSON           bool
}

// ConvertNDJSONLine represents a single NDJSON line emitted by `debris convert --json`.
type ConvertNDJSONLine struct {
	Type   string                  `json:"type"`
	Event  *debris.Event           `json:"event,omitempty"`
	Result *debris.ConvertResponse `json:"result,omitempty"`
	Error  *debris.Error           `json:"error,omitempty"`
}

var convertFlagNames = []string{
	"from",
	"to",
	"input",
	"output",
	"copy-mode",
	"dry-run",
	"overwrite",
	"include-deleted",
	"include-hidden",
	"name",
	"json",
}

func newConvertCmd(eng *debris.Engine) *cobra.Command {
	var opts convertOptions

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert a library from one format to another",
		Example: `  debris convert --from billfish --to eagle --input ./lib.billfish --output ./lib.library
  debris convert -t pixcall -i ./lib.billfish -o ./pixcall-lib --copy-mode hardlink --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return executeConvert(cmd, eng, opts)
		},
	}

	bindConvertFlags(cmd, &opts)
	return cmd
}

func bindConvertFlags(cmd *cobra.Command, opts *convertOptions) {
	flags := cmd.Flags()
	flags.StringVarP(&opts.From, "from", "f", "", "Source format (auto-detected if omitted)")
	flags.StringVarP(&opts.To, "to", "t", "", "Destination format (e.g., eagle, pixcall, billfish)")
	flags.StringVarP(&opts.InputPath, "input", "i", "", "Input library path")
	flags.StringVarP(&opts.OutputPath, "output", "o", "", "Output library path")
	flags.StringVar(&opts.CopyMode, "copy-mode", string(debris.CopyModeAuto), "Asset transfer mode: copy, hardlink, or auto")
	flags.BoolVar(&opts.DryRun, "dry-run", false, "Validate and plan without writing target files")
	flags.BoolVar(&opts.Overwrite, "overwrite", false, "Overwrite existing target directory contents")
	flags.BoolVar(&opts.IncludeDeleted, "include-deleted", false, "Include deleted/trashed items")
	flags.BoolVar(&opts.IncludeHidden, "include-hidden", false, "Include hidden items")
	flags.StringVar(&opts.LibraryName, "name", "", "Override target library name")
	flags.BoolVar(&opts.JSON, "json", false, "Emit NDJSON progress events and final result")
}

func hasAnyConvertFlagChanged(cmd *cobra.Command) bool {
	flags := cmd.Flags()
	for _, name := range convertFlagNames {
		if flags.Changed(name) {
			return true
		}
	}
	return false
}

func executeConvert(cmd *cobra.Command, eng *debris.Engine, opts convertOptions) error {
	req := debris.ConvertRequest{
		From:           opts.From,
		To:             opts.To,
		InputPath:      opts.InputPath,
		OutputPath:     opts.OutputPath,
		CopyMode:       debris.CopyMode(opts.CopyMode),
		DryRun:         opts.DryRun,
		IncludeDeleted: opts.IncludeDeleted,
		IncludeHidden:  opts.IncludeHidden,
		Overwrite:      opts.Overwrite,
		LibraryName:    opts.LibraryName,
	}

	out := cmd.OutOrStdout()

	if opts.JSON {
		var mu sync.Mutex
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		writeLine := func(line ConvertNDJSONLine) {
			mu.Lock()
			defer mu.Unlock()
			_ = enc.Encode(line)
		}

		resp, err := eng.Convert(commandContext(cmd), req, func(ev debris.Event) {
			evCopy := ev
			writeLine(ConvertNDJSONLine{
				Type:  "event",
				Event: &evCopy,
			})
		})
		if err != nil {
			writeLine(ConvertNDJSONLine{
				Type:  "error",
				Error: debris.AsError(err),
			})
			return err
		}
		writeLine(ConvertNDJSONLine{
			Type:   "result",
			Result: &resp,
		})
		return nil
	}

	var mu sync.Mutex
	resp, err := eng.Convert(commandContext(cmd), req, func(ev debris.Event) {
		mu.Lock()
		defer mu.Unlock()
		switch ev.Kind {
		case "stage":
			if ev.Message != "" {
				_, _ = fmt.Fprintf(out, "[stage] %s: %s\n", ev.Stage, ev.Message)
			} else if ev.Stage != "" {
				_, _ = fmt.Fprintf(out, "[stage] %s\n", ev.Stage)
			}
		case "warning":
			_, _ = fmt.Fprintf(out, "[warning] %s\n", ev.Message)
		case "progress":
			if ev.Total > 0 {
				_, _ = fmt.Fprintf(out, "[progress] %d/%d %s\n", ev.Current, ev.Total, ev.AssetID)
			}
		}
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "Converted %s -> %s\n", resp.SourceFormat, resp.TargetFormat)
	_, _ = fmt.Fprintf(out, "Input:      %s\n", resp.InputPath)
	_, _ = fmt.Fprintf(out, "Output:     %s\n", resp.OutputPath)
	_, _ = fmt.Fprintf(out, "CopyMode:   %s (dryRun=%t)\n", resp.CopyMode, resp.DryRun)
	_, _ = fmt.Fprintf(out, "Folders:    %d\n", resp.FolderCount)
	_, _ = fmt.Fprintf(out, "TagGroups:  %d\n", resp.TagGroupCount)
	_, _ = fmt.Fprintf(out, "Tags:       %d\n", resp.TagCount)
	_, _ = fmt.Fprintf(out, "Assets:     %d (copied=%d, hardlinked=%d, skipped=%d)\n",
		resp.AssetCount, resp.CopiedAssets, resp.HardlinkedAssets, resp.SkippedAssets)
	_, _ = fmt.Fprintf(out, "TotalBytes: %d\n", resp.TotalBytes)
	_, _ = fmt.Fprintf(out, "DurationMs: %d\n", resp.DurationMs)
	for _, w := range resp.Warnings {
		_, _ = fmt.Fprintf(out, "Warning:    %s\n", w)
	}
	return nil
}
