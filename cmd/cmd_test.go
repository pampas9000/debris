package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
		DryRun:           req.DryRun,
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
		"--dry-run",
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
		!runner.lastConvert.DryRun ||
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
	if lines[3].Type != "result" || lines[3].Result == nil || !lines[3].Result.DryRun || lines[3].Result.TargetFormat != "pixcall" {
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

func TestBackwardCompatRootFlags(t *testing.T) {
	runner := &mockRunner{}
	stdout, _, err := runCLI(
		runner,
		"-f", "billfish",
		"-t", "eagle",
		"-i", "./old.billfish",
		"-o", "./new.library",
	)
	if err != nil {
		t.Fatalf("root compat convert failed: %v", err)
	}
	if runner.lastConvert.From != "billfish" ||
		runner.lastConvert.To != "eagle" ||
		runner.lastConvert.InputPath != "./old.billfish" ||
		runner.lastConvert.OutputPath != "./new.library" ||
		runner.lastConvert.CopyMode != debris.CopyModeAuto {
		t.Fatalf("unexpected root convert request: %+v", runner.lastConvert)
	}
	if !strings.Contains(stdout, "Converted billfish -> eagle") {
		t.Fatalf("unexpected root convert output:\n%s", stdout)
	}

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

func TestProtocolCmd(t *testing.T) {
	runner := &mockRunner{
		formats: []debris.FormatSpec{{ID: "eagle", DisplayName: "Eagle"}},
	}
	eng := debris.NewEngine(debris.WithRunner(runner))
	root := NewRootCmd(eng)

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"formats"}` + "\n")
	var outBuf, errBuf bytes.Buffer
	root.SetIn(in)
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"protocol"})

	if err := root.Execute(); err != nil {
		t.Fatalf("protocol command failed: %v", err)
	}
	if !strings.Contains(outBuf.String(), `"eagle"`) {
		t.Fatalf("expected formats JSON-RPC response, got: %s", outBuf.String())
	}
}
