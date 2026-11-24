package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"strings"
	"testing"

	"debris/pkg/debris"
)

type mockRunner struct {
	formats     []debris.FormatSpec
	lastInspect debris.InspectRequest
	lastPlan    debris.PlanRequest
	lastConvert debris.ConvertRequest

	inspectResp debris.InspectResponse
	inspectErr  error
	planResp    debris.PlanResponse
	planErr     error
	convertResp debris.ConvertResponse
	convertErr  error
	convertEmit []debris.Event
}

func (m *mockRunner) Formats() []debris.FormatSpec {
	return m.formats
}

func (m *mockRunner) Probe(_ context.Context, req debris.ProbeRequest) (debris.ProbeResponse, error) {
	return debris.ProbeResponse{Matched: true, Format: "eagle", RootPath: req.Path}, nil
}

func (m *mockRunner) Inspect(_ context.Context, req debris.InspectRequest) (debris.InspectResponse, error) {
	m.lastInspect = req
	if m.inspectErr != nil {
		return debris.InspectResponse{}, m.inspectErr
	}
	if m.inspectResp.Format != "" {
		return m.inspectResp, nil
	}
	return debris.InspectResponse{
		Format:        "billfish",
		RootPath:      req.Path,
		Name:          "SampleLib",
		FolderCount:   3,
		TagGroupCount: 1,
		TagCount:      8,
		AssetCount:    25,
		TotalBytes:    4096,
	}, nil
}

func (m *mockRunner) Plan(_ context.Context, req debris.PlanRequest) (debris.PlanResponse, error) {
	m.lastPlan = req
	if m.planErr != nil {
		return debris.PlanResponse{}, m.planErr
	}
	if m.planResp.TargetFormat != "" {
		return m.planResp, nil
	}
	return debris.PlanResponse{
		SourceFormat: req.From,
		TargetFormat: req.To,
		InputPath:    req.InputPath,
		OutputPath:   req.OutputPath,
		CopyMode:     req.CopyMode,
		FolderCount:  3,
		TagCount:     8,
		AssetCount:   25,
		TotalBytes:   4096,
		Losses:       []string{"tag groups not supported in eagle"},
	}, nil
}

func (m *mockRunner) Convert(_ context.Context, req debris.ConvertRequest, emit debris.EventCallback) (debris.ConvertResponse, error) {
	m.lastConvert = req
	for _, ev := range m.convertEmit {
		if emit != nil {
			emit(ev)
		}
	}
	if m.convertErr != nil {
		return debris.ConvertResponse{}, m.convertErr
	}
	if m.convertResp.TargetFormat != "" {
		return m.convertResp, nil
	}
	return debris.ConvertResponse{
		SourceFormat:     req.From,
		TargetFormat:     req.To,
		InputPath:        req.InputPath,
		OutputPath:       req.OutputPath,
		CopyMode:         req.CopyMode,
		FolderCount:      3,
		TagCount:         8,
		AssetCount:       25,
		CopiedAssets:     25,
		HardlinkedAssets: 0,
		TotalBytes:       4096,
		DurationMs:       12,
	}, nil
}

func runCLI(runner debris.Runner, args ...string) (stdout, stderr string, err error) {
	eng := debris.NewEngine(debris.WithRunner(runner))
	root := NewRootCmd(eng)
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestInspectCmd(t *testing.T) {
	runner := &mockRunner{}
	stdout, _, err := runCLI(
		runner,
		"inspect",
		"-p", "/libs/my.bf",
		"--format", "billfish",
		"--include-deleted",
		"--include-hidden",
		"--json",
	)
	if err != nil {
		t.Fatalf("inspect --json failed: %v", err)
	}
	if runner.lastInspect.Path != "/libs/my.bf" ||
		runner.lastInspect.Format != "billfish" ||
		!runner.lastInspect.IncludeDeleted ||
		!runner.lastInspect.IncludeHidden {
		t.Fatalf("unexpected inspect request: %+v", runner.lastInspect)
	}

	var resp debris.InspectResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("unmarshal inspect JSON failed: %v\noutput:\n%s", err, stdout)
	}
	if resp.Format != "billfish" || resp.AssetCount != 25 {
		t.Fatalf("unexpected inspect response: %+v", resp)
	}

	// Human-readable output
	stdout, _, err = runCLI(runner, "inspect", "--path", "/libs/my.bf")
	if err != nil {
		t.Fatalf("inspect human output failed: %v", err)
	}
	if !strings.Contains(stdout, "Format:") || !strings.Contains(stdout, "Assets:     25") {
		t.Fatalf("unexpected human inspect output:\n%s", stdout)
	}
}

func TestPlanCmd(t *testing.T) {
	runner := &mockRunner{}
	stdout, _, err := runCLI(
		runner,
		"plan",
		"-f", "billfish",
		"-t", "eagle",
		"-i", "/in/bf",
		"-o", "/out/eagle",
		"--copy-mode", "hardlink",
		"--overwrite",
		"--json",
	)
	if err != nil {
		t.Fatalf("plan --json failed: %v", err)
	}
	if runner.lastPlan.From != "billfish" ||
		runner.lastPlan.To != "eagle" ||
		runner.lastPlan.InputPath != "/in/bf" ||
		runner.lastPlan.OutputPath != "/out/eagle" ||
		runner.lastPlan.CopyMode != debris.CopyModeHardlink ||
		!runner.lastPlan.Overwrite {
		t.Fatalf("unexpected plan request: %+v", runner.lastPlan)
	}

	var resp debris.PlanResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("unmarshal plan JSON failed: %v\noutput:\n%s", err, stdout)
	}
	if resp.CopyMode != debris.CopyModeHardlink || len(resp.Losses) != 1 {
		t.Fatalf("unexpected plan response: %+v", resp)
	}
}

func TestConvertJSONCmd(t *testing.T) {
	runner := &mockRunner{
		convertEmit: []debris.Event{
			{Kind: "stage", Stage: "read"},
			{Kind: "progress", Stage: "copy", Current: 1, Total: 2, AssetID: "asset-1"},
			{Kind: "warning", Message: "skipped empty tag"},
		},
	}

	stdout, _, err := runCLI(
		runner,
		"convert",
		"--from", "billfish",
		"--to", "pixcall",
		"--input", "/in/bf",
		"--output", "/out/pix",
		"--copy-mode", "copy",
		"--overwrite",
		"--include-deleted",
		"--include-hidden",
		"--name", "ConvertedLib",
		"--json",
	)
	if err != nil {
		t.Fatalf("convert --json failed: %v", err)
	}
	if runner.lastConvert.CopyMode != debris.CopyModeCopy ||
		!runner.lastConvert.Overwrite ||
		!runner.lastConvert.IncludeDeleted ||
		!runner.lastConvert.IncludeHidden ||
		runner.lastConvert.LibraryName != "ConvertedLib" {
		t.Fatalf("unexpected convert request: %+v", runner.lastConvert)
	}

	var lines []ConvertNDJSONLine
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	for scanner.Scan() {
		var line ConvertNDJSONLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("unmarshal NDJSON line failed: %v (%s)", err, scanner.Text())
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan NDJSON failed: %v", err)
	}
	if len(lines) != 4 {
		t.Fatalf("expected 4 NDJSON lines (3 events + 1 result), got %d:\n%s", len(lines), stdout)
	}
	if lines[0].Type != "event" || lines[0].Event == nil || lines[0].Event.Kind != "stage" {
		t.Fatalf("unexpected line 0: %+v", lines[0])
	}
	if lines[1].Type != "event" || lines[1].Event == nil || lines[1].Event.AssetID != "asset-1" {
		t.Fatalf("unexpected line 1: %+v", lines[1])
	}
	if lines[3].Type != "result" || lines[3].Result == nil || lines[3].Result.TargetFormat != "pixcall" {
		t.Fatalf("unexpected result line: %+v", lines[3])
	}
}

func TestConvertJSONError(t *testing.T) {
	runner := &mockRunner{
		convertErr: debris.NewError(debris.CodeNotFound, "source library missing", "/missing"),
	}
	stdout, _, err := runCLI(
		runner,
		"convert",
		"-t", "eagle",
		"-i", "/missing",
		"-o", "/out",
		"--json",
	)
	if err == nil {
		t.Fatal("expected error from convert --json")
	}
	var line ConvertNDJSONLine
	if uErr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &line); uErr != nil {
		t.Fatalf("expected NDJSON error line, got %q (%v)", stdout, uErr)
	}
	if line.Type != "error" || line.Error == nil || line.Error.Code != debris.CodeNotFound {
		t.Fatalf("unexpected error NDJSON line: %+v", line)
	}
}

func TestRootHelp(t *testing.T) {
	runner := &mockRunner{}

	// No args/flags prints help without error and does not emit legacy banner.
	helpOut, _, err := runCLI(runner)
	if err != nil {
		t.Fatalf("root help failed: %v", err)
	}
	if strings.Contains(helpOut, "Welcome to lib-bridge") {
		t.Fatalf("legacy banner must be removed, got:\n%s", helpOut)
	}
	if !strings.Contains(helpOut, "Usage:") {
		t.Fatalf("expected usage help output, got:\n%s", helpOut)
	}
}

func TestInspectJSONError(t *testing.T) {
	runner := &mockRunner{
		inspectErr: debris.NewError(debris.CodeNotFound, "source library missing", "/missing"),
	}
	stdout, _, err := runCLI(runner, "inspect", "-p", "/missing", "--json")
	if err == nil {
		t.Fatal("expected error from inspect --json")
	}
	var env jsonErrorEnvelope
	if uErr := json.Unmarshal([]byte(stdout), &env); uErr != nil {
		t.Fatalf("expected JSON error object, got %q (%v)", stdout, uErr)
	}
	if env.Error == nil || env.Error.Code != debris.CodeNotFound {
		t.Fatalf("unexpected JSON error payload: %+v", env)
	}
}

func TestPlanJSONError(t *testing.T) {
	runner := &mockRunner{
		planErr: debris.NewError(debris.CodeNotFound, "source library missing", "/missing"),
	}
	stdout, _, err := runCLI(runner, "plan", "-t", "eagle", "-i", "/missing", "-o", "/out", "--json")
	if err == nil {
		t.Fatal("expected error from plan --json")
	}
	var env jsonErrorEnvelope
	if uErr := json.Unmarshal([]byte(stdout), &env); uErr != nil {
		t.Fatalf("expected JSON error object, got %q (%v)", stdout, uErr)
	}
	if env.Error == nil || env.Error.Code != debris.CodeNotFound {
		t.Fatalf("unexpected JSON error payload: %+v", env)
	}
}

func TestDefaultEngineFormats(t *testing.T) {
	eng := defaultEngine()
	specs := eng.Formats()
	if len(specs) != 4 {
		t.Fatalf("expected 4 wired formats, got %d: %+v", len(specs), specs)
	}
	got := make(map[string]bool, len(specs))
	for _, s := range specs {
		got[s.ID] = true
	}
	for _, want := range []string{"billfish", "eagle", "pixcall", "refto"} {
		if !got[want] {
			t.Fatalf("defaultEngine missing format %q (got %+v)", want, specs)
		}
	}
}

func TestPositionalArgsAndDerivedOutput(t *testing.T) {
	runner := &mockRunner{}

	// convert with 1 positional arg derives output path
	_, _, err := runCLI(runner, "convert", "./libs/sample.billfish", "-t", "eagle")
	if err != nil {
		t.Fatalf("convert positional 1 arg failed: %v", err)
	}
	if runner.lastConvert.InputPath != "./libs/sample.billfish" {
		t.Fatalf("unexpected InputPath: %q", runner.lastConvert.InputPath)
	}
	if runner.lastConvert.OutputPath != "libs/sample.library" {
		t.Fatalf("unexpected derived OutputPath: %q", runner.lastConvert.OutputPath)
	}

	// plan with 2 positional args
	_, _, err = runCLI(runner, "plan", "./in.library", "./out.pixcall", "-t", "pixcall")
	if err != nil {
		t.Fatalf("plan positional 2 args failed: %v", err)
	}
	if runner.lastPlan.InputPath != "./in.library" || runner.lastPlan.OutputPath != "./out.pixcall" {
		t.Fatalf("unexpected plan paths: %+v", runner.lastPlan)
	}

	// same-format derivation avoids colliding with input path
	if got := defaultOutputPath("./my.library", "eagle"); got != "my-eagle.library" {
		t.Fatalf("expected collision-safe derived path, got %q", got)
	}
	// refto derivation
	if got := defaultOutputPath("./My Library.reflib", "eagle"); got != "My Library.library" {
		t.Fatalf("expected .library derived from .reflib, got %q", got)
	}
	if got := defaultOutputPath("./my.library", "refto"); got != "my.reflib" {
		t.Fatalf("expected .reflib derived for refto, got %q", got)
	}
}

func TestIncludeSuggestedTagsFlagForwarding(t *testing.T) {
	runner := &mockRunner{}
	if _, _, err := runCLI(runner, "inspect", "/libs/a.reflib", "--include-suggested-tags", "--json"); err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if !runner.lastInspect.IncludeSuggestedTags {
		t.Fatal("expected IncludeSuggestedTags=true on inspect request")
	}
	if _, _, err := runCLI(runner, "plan", "/libs/a.reflib", "-t", "eagle", "--include-suggested-tags", "--json"); err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if !runner.lastPlan.IncludeSuggestedTags {
		t.Fatal("expected IncludeSuggestedTags=true on plan request")
	}
	if _, _, err := runCLI(runner, "convert", "/libs/a.reflib", "-t", "eagle", "--include-suggested-tags", "--json"); err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	if !runner.lastConvert.IncludeSuggestedTags {
		t.Fatal("expected IncludeSuggestedTags=true on convert request")
	}
}

func TestRootVersionFlag(t *testing.T) {
	runner := &mockRunner{}
	stdout, _, err := runCLI(runner, "--version")
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	if !strings.Contains(stdout, "debris version") {
		t.Fatalf("unexpected --version output: %q", stdout)
	}
}

func TestConvertQuietAndJSONStderrSilence(t *testing.T) {
	runner := &mockRunner{
		convertEmit: []debris.Event{
			{Kind: "stage", Stage: "read"},
			{Kind: "progress", Stage: "copy", Current: 1, Total: 2, AssetID: "asset-1"},
			{Kind: "warning", Message: "keep me"},
		},
	}
	stdout, _, err := runCLI(runner, "convert", "./in.bf", "./out.library", "-t", "eagle", "-q")
	if err != nil {
		t.Fatalf("convert -q failed: %v", err)
	}
	if strings.Contains(stdout, "[stage]") || strings.Contains(stdout, "[progress]") {
		t.Fatalf("expected quiet mode to suppress stage/progress, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "[warning] keep me") || !strings.Contains(stdout, "4.0 KiB (4096 B)") {
		t.Fatalf("expected warning and humanized bytes in output, got:\n%s", stdout)
	}

	errRunner := &mockRunner{
		inspectErr: debris.NewError(debris.CodeNotFound, "missing", "/missing"),
		planErr:    debris.NewError(debris.CodeNotFound, "missing", "/missing"),
		convertErr: debris.NewError(debris.CodeNotFound, "missing", "/missing"),
	}
	for _, args := range [][]string{
		{"inspect", "/missing", "--json"},
		{"plan", "/missing", "/out", "-t", "eagle", "--json"},
		{"convert", "/missing", "/out", "-t", "eagle", "--json"},
	} {
		_, stderr, err := runCLI(errRunner, args...)
		if err == nil {
			t.Fatalf("expected error for %v", args)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr in --json mode for %v, got %q", args, stderr)
		}
	}
}
