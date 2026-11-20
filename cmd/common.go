package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"debris/pkg/debris"

	"github.com/dustin/go-humanize"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

type transferOptions struct {
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
	Quiet          bool
	JSON           bool
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
	"quiet",
	"json",
}

func bindTransferFlags(cmd *cobra.Command, opts *transferOptions, eng *debris.Engine, includeConvertOnly bool) {
	flags := cmd.Flags()
	flags.StringVarP(&opts.From, "from", "f", "", "Source format (auto-detected if omitted)")
	flags.StringVarP(&opts.To, "to", "t", "", "Destination format (e.g., eagle, pixcall, billfish)")
	flags.StringVarP(&opts.InputPath, "input", "i", "", "Input library path")
	flags.StringVarP(&opts.OutputPath, "output", "o", "", "Output library path (derived from input and --to if omitted)")
	flags.StringVar(&opts.CopyMode, "copy-mode", string(debris.CopyModeAuto), "Asset transfer mode: copy, hardlink, or auto")
	flags.BoolVar(&opts.Overwrite, "overwrite", false, "Overwrite existing target directory contents")
	flags.BoolVar(&opts.IncludeDeleted, "include-deleted", false, "Include deleted/trashed items")
	flags.BoolVar(&opts.IncludeHidden, "include-hidden", false, "Include hidden items")
	flags.StringVar(&opts.LibraryName, "name", "", "Override target library name")

	if includeConvertOnly {
		flags.BoolVar(&opts.DryRun, "dry-run", false, "Validate and plan without writing target files")
		flags.BoolVarP(&opts.Quiet, "quiet", "q", false, "Suppress progress output in non-JSON mode")
		flags.BoolVar(&opts.JSON, "json", false, "Emit NDJSON progress events and final result")
	} else {
		flags.BoolVar(&opts.JSON, "json", false, "Output plan as JSON")
	}

	registerFormatFlagCompletion(cmd, "from", eng)
	registerFormatFlagCompletion(cmd, "to", eng)
	_ = cmd.RegisterFlagCompletionFunc("copy-mode", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		modes := []string{
			string(debris.CopyModeAuto) + "\tPrefer hardlink when possible, fallback to copy",
			string(debris.CopyModeCopy) + "\tCopy asset files",
			string(debris.CopyModeHardlink) + "\tHardlink asset files",
		}
		var matched []string
		for _, m := range modes {
			if strings.HasPrefix(m, toComplete) {
				matched = append(matched, m)
			}
		}
		return matched, cobra.ShellCompDirectiveNoFileComp
	})
}

func registerFormatFlagCompletion(cmd *cobra.Command, flagName string, eng *debris.Engine) {
	_ = cmd.RegisterFlagCompletionFunc(flagName, func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if eng == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var out []string
		for _, spec := range eng.Formats() {
			if strings.HasPrefix(spec.ID, toComplete) {
				out = append(out, fmt.Sprintf("%s\t%s", spec.ID, spec.DisplayName))
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	})
}

func applyPositionalInputOutput(opts *transferOptions, args []string) {
	if opts.InputPath == "" && len(args) >= 1 {
		opts.InputPath = args[0]
	}
	if opts.OutputPath == "" && len(args) >= 2 {
		opts.OutputPath = args[1]
	}
	if opts.OutputPath == "" && strings.TrimSpace(opts.InputPath) != "" && strings.TrimSpace(opts.To) != "" {
		opts.OutputPath = defaultOutputPath(opts.InputPath, opts.To)
	}
}

var knownLibraryExtensions = []string{
	".billfish",
	".bf",
	".library",
	".eagle",
	".pixcall",
}

// defaultOutputPath derives a target library path from inputPath and toFormat
// when --output / positional output is omitted.
func defaultOutputPath(inputPath, toFormat string) string {
	cleanIn := filepath.Clean(strings.TrimSpace(inputPath))
	if cleanIn == "" || cleanIn == "." {
		return ""
	}
	dir := filepath.Dir(cleanIn)
	base := filepath.Base(cleanIn)

	stem := base
	lowerBase := strings.ToLower(base)
	for _, ext := range knownLibraryExtensions {
		if strings.HasSuffix(lowerBase, ext) && len(base) > len(ext) {
			stem = base[:len(base)-len(ext)]
			break
		}
	}

	normTo := strings.ToLower(strings.TrimSpace(toFormat))
	var targetExt string
	switch normTo {
	case "eagle":
		targetExt = ".library"
	case "pixcall":
		targetExt = ".pixcall"
	case "billfish":
		targetExt = ".billfish"
	default:
		if normTo == "" {
			return ""
		}
		targetExt = "." + normTo
	}

	candidate := filepath.Join(dir, stem+targetExt)
	if candidate == cleanIn {
		candidate = filepath.Join(dir, stem+"-"+normTo+targetExt)
	}
	return candidate
}

func formatBytes(b int64) string {
	if b < 0 {
		return fmt.Sprintf("%d B", b)
	}
	return fmt.Sprintf("%s (%d B)", humanize.IBytes(uint64(b)), b)
}

func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
