package debris

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type stubRunner struct {
	formats []FormatSpec
}

func (s *stubRunner) Formats() []FormatSpec { return s.formats }

func (s *stubRunner) Probe(_ context.Context, req ProbeRequest) (ProbeResponse, error) {
	return ProbeResponse{Matched: true, Format: "eagle", RootPath: req.Path}, nil
}

func (s *stubRunner) Inspect(_ context.Context, req InspectRequest) (InspectResponse, error) {
	return InspectResponse{Format: "eagle", RootPath: req.Path, AssetCount: 3}, nil
}

func (s *stubRunner) Plan(_ context.Context, req PlanRequest) (PlanResponse, error) {
	return PlanResponse{
		SourceFormat: req.From,
		TargetFormat: req.To,
		InputPath:    req.InputPath,
		OutputPath:   req.OutputPath,
		CopyMode:     req.CopyMode,
	}, nil
}

func (s *stubRunner) Convert(_ context.Context, req ConvertRequest, emit EventCallback) (ConvertResponse, error) {
	if emit != nil {
		emit(Event{Kind: "progress", Current: 1, Total: 1})
	}
	return ConvertResponse{
		SourceFormat: req.From,
		TargetFormat: req.To,
		InputPath:    req.InputPath,
		OutputPath:   req.OutputPath,
		CopyMode:     req.CopyMode,
	}, nil
}

func TestEngine_ValidationAndDelegation(t *testing.T) {
	eng := NewEngine(WithFormats(FormatSpec{ID: "billfish", DisplayName: "Billfish"}))
	if got := eng.Formats(); len(got) != 1 || got[0].ID != "billfish" {
		t.Fatalf("unexpected formats: %#v", got)
	}

	if _, err := eng.Probe(context.Background(), ProbeRequest{}); AsError(err).Code != CodeInvalidRequest {
		t.Fatalf("expected CodeInvalidRequest, got %v", err)
	}
	if _, err := eng.Probe(context.Background(), ProbeRequest{Path: "/tmp/lib"}); AsError(err).Code != CodeNotImplemented {
		t.Fatalf("expected CodeNotImplemented, got %v", err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := eng.Inspect(canceledCtx, InspectRequest{Path: "/tmp/lib"}); AsError(err).Code != CodeCanceled {
		t.Fatalf("expected CodeCanceled, got %v", err)
	}

	if _, err := eng.Convert(context.Background(), ConvertRequest{
		To:         "pixcall",
		InputPath:  "/in",
		OutputPath: "/out",
		CopyMode:   "invalid",
	}, nil); AsError(err).Code != CodeInvalidRequest {
		t.Fatalf("expected CodeInvalidRequest on invalid copyMode, got %v", err)
	}

	wired := NewEngine(WithRunner(&stubRunner{
		formats: []FormatSpec{{ID: "eagle", DisplayName: "Eagle"}},
	}))
	var seenEvents int
	resp, err := wired.Convert(context.Background(), ConvertRequest{
		From:       "billfish",
		To:         "eagle",
		InputPath:  "/in",
		OutputPath: "/out",
	}, func(ev Event) {
		seenEvents++
	})
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if resp.CopyMode != CopyModeAuto || seenEvents != 1 {
		t.Fatalf("unexpected Convert response=%#v seenEvents=%d", resp, seenEvents)
	}

	// Ensure Error JSON serialization works cleanly.
	wrapped := WrapError(CodeIO, "disk full", errors.New("ENOSPC"))
	raw, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("json.Marshal(Error) failed: %v", err)
	}
	var decoded Error
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(Error) failed: %v", err)
	}
	if decoded.Code != CodeIO || decoded.Detail != "ENOSPC" {
		t.Fatalf("unexpected decoded Error: %#v", decoded)
	}
}
