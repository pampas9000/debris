package cmd

import (
	"fmt"

	"debris/pkg/debris"

	"github.com/spf13/cobra"
)

type inspectOptions struct {
	Path                 string
	Format               string
	IncludeDeleted       bool
	IncludeHidden        bool
	IncludeSuggestedTags bool
	JSON                 bool
}

func newInspectCmd(eng *debris.Engine) *cobra.Command {
	var opts inspectOptions

	cmd := &cobra.Command{
		Use:   "inspect [path]",
		Short: "Inspect a library and summarize its contents",
		Example: `  debris inspect ./eagle-lib
  debris inspect -p ./billfish-lib -f billfish --json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if opts.Path == "" && len(args) > 0 {
				opts.Path = args[0]
			}
			return executeInspect(cmd, eng, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.Path, "path", "p", "", "Library path to inspect")
	flags.StringVarP(&opts.Format, "format", "f", "", "Source format (auto-detected if omitted)")
	flags.BoolVar(&opts.IncludeDeleted, "include-deleted", false, "Include deleted/trashed items")
	flags.BoolVar(&opts.IncludeHidden, "include-hidden", false, "Include hidden items")
	flags.BoolVar(&opts.IncludeSuggestedTags, "include-suggested-tags", false, "Include AI-suggested (unconfirmed) tags when supported by source format")
	flags.BoolVar(&opts.JSON, "json", false, "Output inspection result as JSON")
	registerFormatFlagCompletion(cmd, "format", eng)

	return cmd
}

func executeInspect(cmd *cobra.Command, eng *debris.Engine, opts inspectOptions) error {
	out := cmd.OutOrStdout()
	if opts.JSON {
		cmd.SilenceErrors = true
	}

	resp, err := eng.Inspect(commandContext(cmd), debris.InspectRequest{
		Path:                 opts.Path,
		Format:               opts.Format,
		IncludeDeleted:       opts.IncludeDeleted,
		IncludeHidden:        opts.IncludeHidden,
		IncludeSuggestedTags: opts.IncludeSuggestedTags,
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

	_, _ = fmt.Fprintf(out, "Format:     %s\n", resp.Format)
	if resp.Version != "" {
		_, _ = fmt.Fprintf(out, "Version:    %s\n", resp.Version)
	}
	_, _ = fmt.Fprintf(out, "RootPath:   %s\n", resp.RootPath)
	if resp.Name != "" {
		_, _ = fmt.Fprintf(out, "Name:       %s\n", resp.Name)
	}
	_, _ = fmt.Fprintf(out, "Folders:    %d\n", resp.FolderCount)
	_, _ = fmt.Fprintf(out, "TagGroups:  %d\n", resp.TagGroupCount)
	_, _ = fmt.Fprintf(out, "Tags:       %d\n", resp.TagCount)
	_, _ = fmt.Fprintf(out, "Assets:     %d\n", resp.AssetCount)
	if resp.DeletedCount > 0 {
		_, _ = fmt.Fprintf(out, "Deleted:    %d\n", resp.DeletedCount)
	}
	if resp.HiddenCount > 0 {
		_, _ = fmt.Fprintf(out, "Hidden:     %d\n", resp.HiddenCount)
	}
	_, _ = fmt.Fprintf(out, "TotalBytes: %s\n", formatBytes(resp.TotalBytes))
	for _, w := range resp.Warnings {
		_, _ = fmt.Fprintf(out, "Warning:    %s\n", w)
	}
	return nil
}
