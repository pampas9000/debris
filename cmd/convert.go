package cmd

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"sync"

	"debris/pkg/debris"

	"github.com/spf13/cobra"
)

// ConvertNDJSONLine represents a single NDJSON line emitted by `debris convert --json`.
type ConvertNDJSONLine struct {
	Type   string                  `json:"type"`
	Event  *debris.Event           `json:"event,omitempty"`
	Result *debris.ConvertResponse `json:"result,omitempty"`
	Error  *debris.Error           `json:"error,omitempty"`
}

func newConvertCmd(eng *debris.Engine) *cobra.Command {
	var opts transferOptions

	cmd := &cobra.Command{
		Use:   "convert [input] [output]",
		Short: "Convert a library from one format to another",
		Example: `  debris convert ./lib.billfish ./lib.library -t eagle
  debris convert ./lib.billfish -t pixcall
  debris convert -t pixcall -i ./lib.billfish -o ./pixcall-lib --copy-mode hardlink --json`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			applyPositionalInputOutput(&opts, args)
			return executeConvert(cmd, eng, opts)
		},
	}

	bindTransferFlags(cmd, &opts, eng, true)
	return cmd
}

func executeConvert(cmd *cobra.Command, eng *debris.Engine, opts transferOptions) error {
	req := debris.ConvertRequest{
		From:           opts.From,
		To:             opts.To,
		InputPath:      opts.InputPath,
		OutputPath:     opts.OutputPath,
		CopyMode:       debris.CopyMode(opts.CopyMode),
		IncludeDeleted: opts.IncludeDeleted,
		IncludeHidden:  opts.IncludeHidden,
		Overwrite:      opts.Overwrite,
		LibraryName:    opts.LibraryName,
	}

	out := cmd.OutOrStdout()

	if opts.JSON {
		cmd.SilenceErrors = true
		var mu sync.Mutex
		enc := jsontext.NewEncoder(out)
		writeLine := func(line ConvertNDJSONLine) {
			mu.Lock()
			defer mu.Unlock()
			_ = json.MarshalEncode(enc, line)
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

	isTTY := isTerminalWriter(out)
	var mu sync.Mutex
	var inlineProgressActive bool
	var lastPercentLogged int64 = -1

	clearInlineProgress := func() {
		if inlineProgressActive {
			_, _ = fmt.Fprint(out, "\r\x1b[K")
			inlineProgressActive = false
		}
	}

	resp, err := eng.Convert(commandContext(cmd), req, func(ev debris.Event) {
		if opts.Quiet && ev.Kind != "warning" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch ev.Kind {
		case "stage":
			clearInlineProgress()
			if ev.Message != "" {
				_, _ = fmt.Fprintf(out, "[stage] %s: %s\n", ev.Stage, ev.Message)
			} else if ev.Stage != "" {
				_, _ = fmt.Fprintf(out, "[stage] %s\n", ev.Stage)
			}
		case "warning":
			clearInlineProgress()
			_, _ = fmt.Fprintf(out, "[warning] %s\n", ev.Message)
		case "progress":
			if ev.Total <= 0 {
				return
			}
			pct := (ev.Current * 100) / ev.Total
			if isTTY {
				_, _ = fmt.Fprintf(out, "\r\x1b[K[progress] %d/%d (%d%%) %s", ev.Current, ev.Total, pct, ev.AssetID)
				inlineProgressActive = true
				return
			}
			if ev.Total > 50 && ev.Current < ev.Total && pct/5 == lastPercentLogged/5 {
				return
			}
			lastPercentLogged = pct
			_, _ = fmt.Fprintf(out, "[progress] %d/%d %s\n", ev.Current, ev.Total, ev.AssetID)
		}
	})
	mu.Lock()
	clearInlineProgress()
	mu.Unlock()
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "Converted %s -> %s\n", resp.SourceFormat, resp.TargetFormat)
	_, _ = fmt.Fprintf(out, "Input:      %s\n", resp.InputPath)
	_, _ = fmt.Fprintf(out, "Output:     %s\n", resp.OutputPath)
	_, _ = fmt.Fprintf(out, "CopyMode:   %s\n", resp.CopyMode)
	_, _ = fmt.Fprintf(out, "Folders:    %d\n", resp.FolderCount)
	_, _ = fmt.Fprintf(out, "TagGroups:  %d\n", resp.TagGroupCount)
	_, _ = fmt.Fprintf(out, "Tags:       %d\n", resp.TagCount)
	_, _ = fmt.Fprintf(out, "Assets:     %d (copied=%d, hardlinked=%d, skipped=%d)\n",
		resp.AssetCount, resp.CopiedAssets, resp.HardlinkedAssets, resp.SkippedAssets)
	_, _ = fmt.Fprintf(out, "TotalBytes: %s\n", formatBytes(resp.TotalBytes))
	_, _ = fmt.Fprintf(out, "DurationMs: %d\n", resp.DurationMs)
	for _, w := range resp.Warnings {
		_, _ = fmt.Fprintf(out, "Warning:    %s\n", w)
	}
	return nil
}
