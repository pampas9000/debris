package cmd

import (
	"fmt"

	"debris/pkg/debris"

	"github.com/spf13/cobra"
)

type planOptions struct {
	From           string
	To             string
	InputPath      string
	OutputPath     string
	CopyMode       string
	Overwrite      bool
	IncludeDeleted bool
	IncludeHidden  bool
	LibraryName    string
	JSON           bool
}

func newPlanCmd(eng *debris.Engine) *cobra.Command {
	var opts planOptions

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview a library conversion and potential compatibility losses",
		Example: `  debris plan --from billfish --to eagle --input ./lib.billfish --output ./lib.library
  debris plan -t pixcall -i ./lib.eagle -o ./pixcall-lib --copy-mode copy --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return executePlan(cmd, eng, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.From, "from", "f", "", "Source format (auto-detected if omitted)")
	flags.StringVarP(&opts.To, "to", "t", "", "Destination format (e.g., eagle, pixcall, billfish)")
	flags.StringVarP(&opts.InputPath, "input", "i", "", "Input library path")
	flags.StringVarP(&opts.OutputPath, "output", "o", "", "Output library path")
	flags.StringVar(&opts.CopyMode, "copy-mode", string(debris.CopyModeAuto), "Asset transfer mode: copy, hardlink, or auto")
	flags.BoolVar(&opts.Overwrite, "overwrite", false, "Allow overwriting existing target directory contents")
	flags.BoolVar(&opts.IncludeDeleted, "include-deleted", false, "Include deleted/trashed items")
	flags.BoolVar(&opts.IncludeHidden, "include-hidden", false, "Include hidden items")
	flags.StringVar(&opts.LibraryName, "name", "", "Override target library name")
	flags.BoolVar(&opts.JSON, "json", false, "Output plan as JSON")

	return cmd
}

func executePlan(cmd *cobra.Command, eng *debris.Engine, opts planOptions) error {
	resp, err := eng.Plan(commandContext(cmd), debris.PlanRequest{
		From:           opts.From,
		To:             opts.To,
		InputPath:      opts.InputPath,
		OutputPath:     opts.OutputPath,
		CopyMode:       debris.CopyMode(opts.CopyMode),
		IncludeDeleted: opts.IncludeDeleted,
		IncludeHidden:  opts.IncludeHidden,
		Overwrite:      opts.Overwrite,
		LibraryName:    opts.LibraryName,
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
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
	_, _ = fmt.Fprintf(out, "TotalBytes: %d\n", resp.TotalBytes)
	for _, l := range resp.Losses {
		_, _ = fmt.Fprintf(out, "Loss:       %s\n", l)
	}
	for _, w := range resp.Warnings {
		_, _ = fmt.Fprintf(out, "Warning:    %s\n", w)
	}
	return nil
}
