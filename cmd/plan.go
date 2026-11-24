package cmd

import (
	"fmt"

	"debris/pkg/debris"

	"github.com/spf13/cobra"
)

func newPlanCmd(eng *debris.Engine) *cobra.Command {
	var opts transferOptions

	cmd := &cobra.Command{
		Use:   "plan [input] [output]",
		Short: "Preview a library conversion and potential compatibility losses",
		Example: `  debris plan ./billfish-lib -t eagle
  debris plan --from billfish --to eagle --input ./billfish-lib --output ./eagle-lib
  debris plan -t pixcall -i ./eagle-lib -o ./pixcall-lib --copy-mode copy --json`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			applyPositionalInputOutput(&opts, args)
			return executePlan(cmd, eng, opts)
		},
	}

	bindTransferFlags(cmd, &opts, eng, false)
	return cmd
}

func executePlan(cmd *cobra.Command, eng *debris.Engine, opts transferOptions) error {
	out := cmd.OutOrStdout()
	if opts.JSON {
		cmd.SilenceErrors = true
	}

	resp, err := eng.Plan(commandContext(cmd), debris.PlanRequest{
		From:                 opts.From,
		To:                   opts.To,
		InputPath:            opts.InputPath,
		OutputPath:           opts.OutputPath,
		CopyMode:             debris.CopyMode(opts.CopyMode),
		IncludeDeleted:       opts.IncludeDeleted,
		IncludeHidden:        opts.IncludeHidden,
		IncludeSuggestedTags: opts.IncludeSuggestedTags,
		Overwrite:            opts.Overwrite,
		LibraryName:          opts.LibraryName,
	})
	if err != nil {
		if opts.JSON {
			writeJSONError(out, err)
		}
		return err
	}

	if opts.JSON {
		return writeJSON(out, resp)
	}

	_, _ = fmt.Fprintf(out, "Plan:       %s -> %s\n", resp.SourceFormat, resp.TargetFormat)
	_, _ = fmt.Fprintf(out, "Input:      %s\n", resp.InputPath)
	_, _ = fmt.Fprintf(out, "Output:     %s\n", resp.OutputPath)
	_, _ = fmt.Fprintf(out, "CopyMode:   %s\n", resp.CopyMode)
	_, _ = fmt.Fprintf(out, "Folders:    %d\n", resp.FolderCount)
	_, _ = fmt.Fprintf(out, "TagGroups:  %d\n", resp.TagGroupCount)
	_, _ = fmt.Fprintf(out, "Tags:       %d\n", resp.TagCount)
	_, _ = fmt.Fprintf(out, "Assets:     %d\n", resp.AssetCount)
	_, _ = fmt.Fprintf(out, "TotalBytes: %s\n", formatBytes(resp.TotalBytes))
	for _, l := range resp.Losses {
		_, _ = fmt.Fprintf(out, "Loss:       %s\n", l)
	}
	for _, w := range resp.Warnings {
		_, _ = fmt.Fprintf(out, "Warning:    %s\n", w)
	}
	return nil
}
