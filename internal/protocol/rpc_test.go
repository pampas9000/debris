package protocol

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"debris/pkg/debris"
)

type mockRunner struct {
	formats []debris.FormatSpec
	probeFn func(ctx context.Context, req debris.ProbeRequest) (debris.ProbeResponse, error)
	inspFn  func(ctx context.Context, req debris.InspectRequest) (debris.InspectResponse, error)
	planFn  func(ctx context.Context, req debris.PlanRequest) (debris.PlanResponse, error)
	convFn  func(ctx context.Context, req debris.ConvertRequest, emit debris.EventCallback) (debris.ConvertResponse, error)
}

func (m *mockRunner) Formats() []debris.FormatSpec {
	return m.formats
}

func (m *mockRunner) Probe(ctx context.Context, req debris.ProbeRequest) (debris.ProbeResponse, error) {
	if m.probeFn != nil {
		return m.probeFn(ctx, req)
	}
	return debris.ProbeResponse{Matched: true, Format: "billfish", RootPath: req.Path}, nil
}

func (m *mockRunner) Inspect(ctx context.Context, req debris.InspectRequest) (debris.InspectResponse, error) {
	if m.inspFn != nil {
		return m.inspFn(ctx, req)
	}
	return debris.InspectResponse{Format: "billfish", RootPath: req.Path, AssetCount: 10}, nil
}

func (m *mockRunner) Plan(ctx context.Context, req debris.PlanRequest) (debris.PlanResponse, error) {
	if m.planFn != nil {
		return m.planFn(ctx, req)
	}
	return debris.PlanResponse{
		SourceFormat: req.From,
		TargetFormat: req.To,
		InputPath:    req.InputPath,
		OutputPath:   req.OutputPath,
		CopyMode:     req.CopyMode,
		AssetCount:   10,
	}, nil
}

func (m *mockRunner) Convert(ctx context.Context, req debris.ConvertRequest, emit debris.EventCallback) (debris.ConvertResponse, error) {
	if m.convFn != nil {
		return m.convFn(ctx, req, emit)
	}
	return debris.ConvertResponse{
		SourceFormat: req.From,
		TargetFormat: req.To,
		InputPath:    req.InputPath,
		OutputPath:   req.OutputPath,
		CopyMode:     req.CopyMode,
	}, nil
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

func startPipeServer(t *testing.T, ctx context.Context, runner debris.Runner) (
	srv *Server,
	inW *io.PipeWriter,
	msgs <-chan rpcEnvelope,
	done <-chan error,
) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	eng := debris.NewEngine(debris.WithRunner(runner))
	srv = NewServer(eng)

	doneCh := make(chan error, 1)
	go func() {
		err := srv.Serve(ctx, inR, outW)
		_ = outW.Close()
		doneCh <- err
	}()

	msgCh := make(chan rpcEnvelope, 64)
	go func() {
		defer close(msgCh)
		scanner := bufio.NewScanner(outR)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			var env rpcEnvelope
			if err := json.Unmarshal(scanner.Bytes(), &env); err != nil {
				t.Errorf("invalid JSON written by server: %v (%s)", err, scanner.Text())
				continue
			}
			msgCh <- env
		}
	}()

	return srv, inW, msgCh, doneCh
}

func writeLine(t *testing.T, w io.Writer, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

func readMsg(t *testing.T, ch <-chan rpcEnvelope) rpcEnvelope {
	t.Helper()
	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("message channel closed unexpectedly")
		}
		return msg
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server message")
		return rpcEnvelope{}
	}
}

func TestServer_BasicMethodsAndErrors(t *testing.T) {
	runner := &mockRunner{
		formats: []debris.FormatSpec{
			{ID: "billfish", DisplayName: "Billfish"},
			{ID: "eagle", DisplayName: "Eagle"},
		},
		probeFn: func(_ context.Context, req debris.ProbeRequest) (debris.ProbeResponse, error) {
			if req.Path == "/missing" {
				return debris.ProbeResponse{}, debris.NewError(debris.CodeNotFound, "library not found", req.Path)
			}
			return debris.ProbeResponse{Matched: true, Format: "eagle", RootPath: req.Path, Confidence: 100}, nil
		},
		inspFn: func(_ context.Context, req debris.InspectRequest) (debris.InspectResponse, error) {
			return debris.InspectResponse{
				Format:        "eagle",
				RootPath:      req.Path,
				FolderCount:   2,
				TagGroupCount: 1,
				TagCount:      5,
				AssetCount:    42,
				TotalBytes:    1024,
			}, nil
		},
		planFn: func(_ context.Context, req debris.PlanRequest) (debris.PlanResponse, error) {
			return debris.PlanResponse{
				SourceFormat: "billfish",
				TargetFormat: req.To,
				InputPath:    req.InputPath,
				OutputPath:   req.OutputPath,
				CopyMode:     req.CopyMode,
				AssetCount:   42,
				Warnings:     []string{"nested tags flattened"},
			}, nil
		},
	}

	srv, inW, msgs, done := startPipeServer(t, context.Background(), runner)

	// 1. formats
	writeLine(t, inW, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "formats"})
	msg := readMsg(t, msgs)
	if string(msg.ID) != "1" || msg.Error != nil {
		t.Fatalf("formats unexpected response: %+v", msg)
	}
	var fmts []debris.FormatSpec
	if err := json.Unmarshal(msg.Result, &fmts); err != nil || len(fmts) != 2 {
		t.Fatalf("unexpected formats result: %s (%v)", msg.Result, err)
	}

	// 2. probe
	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      "probe-1",
		"method":  "probe",
		"params":  debris.ProbeRequest{Path: "/lib/eagle"},
	})
	msg = readMsg(t, msgs)
	if string(msg.ID) != `"probe-1"` || msg.Error != nil {
		t.Fatalf("probe unexpected response: %+v", msg)
	}
	var probeResp debris.ProbeResponse
	if err := json.Unmarshal(msg.Result, &probeResp); err != nil || !probeResp.Matched || probeResp.Format != "eagle" {
		t.Fatalf("unexpected probe result: %s (%v)", msg.Result, err)
	}

	// 3. inspect
	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "inspect",
		"params":  debris.InspectRequest{Path: "/lib/eagle", IncludeDeleted: true},
	})
	msg = readMsg(t, msgs)
	if string(msg.ID) != "2" || msg.Error != nil {
		t.Fatalf("inspect unexpected response: %+v", msg)
	}
	var inspResp debris.InspectResponse
	if err := json.Unmarshal(msg.Result, &inspResp); err != nil || inspResp.AssetCount != 42 {
		t.Fatalf("unexpected inspect result: %s (%v)", msg.Result, err)
	}

	// 4. plan
	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "plan",
		"params":  debris.PlanRequest{To: "eagle", InputPath: "/in", OutputPath: "/out", CopyMode: debris.CopyModeHardlink},
	})
	msg = readMsg(t, msgs)
	if string(msg.ID) != "3" || msg.Error != nil {
		t.Fatalf("plan unexpected response: %+v", msg)
	}
	var planResp debris.PlanResponse
	if err := json.Unmarshal(msg.Result, &planResp); err != nil || planResp.CopyMode != debris.CopyModeHardlink || len(planResp.Warnings) != 1 {
		t.Fatalf("unexpected plan result: %s (%v)", msg.Result, err)
	}

	// 5. structured debris.Error mapping
	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "probe",
		"params":  debris.ProbeRequest{Path: "/missing"},
	})
	msg = readMsg(t, msgs)
	if msg.Error == nil || msg.Error.Code != ErrCodeNotFound {
		t.Fatalf("expected ErrCodeNotFound, got %+v", msg.Error)
	}
	dataBytes, _ := json.Marshal(msg.Error.Data)
	var de debris.Error
	if err := json.Unmarshal(dataBytes, &de); err != nil || de.Code != debris.CodeNotFound || de.Detail != "/missing" {
		t.Fatalf("unexpected structured error data: %s (%v)", dataBytes, err)
	}

	// 6. unknown method
	writeLine(t, inW, map[string]any{"jsonrpc": "2.0", "id": 5, "method": "nope"})
	msg = readMsg(t, msgs)
	if msg.Error == nil || msg.Error.Code != ErrCodeMethodNotFound {
		t.Fatalf("expected ErrCodeMethodNotFound, got %+v", msg.Error)
	}

	// 7. invalid JSON
	if _, err := inW.Write([]byte("{not valid json\n")); err != nil {
		t.Fatalf("write invalid json: %v", err)
	}
	msg = readMsg(t, msgs)
	if msg.Error == nil || msg.Error.Code != ErrCodeParse || string(msg.ID) != "null" {
		t.Fatalf("expected ErrCodeParse with null id, got %+v (id=%s)", msg.Error, msg.ID)
	}

	// 8. invalid params
	writeLine(t, inW, map[string]any{"jsonrpc": "2.0", "id": 6, "method": "probe", "params": []int{1, 2}})
	msg = readMsg(t, msgs)
	if msg.Error == nil || msg.Error.Code != ErrCodeInvalidParams {
		t.Fatalf("expected ErrCodeInvalidParams, got %+v", msg.Error)
	}

	// Close input -> clean EOF exit
	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("Serve returned unexpected error on EOF: %v", err)
	}
	if srv.activeCount() != 0 {
		t.Fatalf("expected 0 active requests after shutdown, got %d", srv.activeCount())
	}
}

func TestServer_ConvertStreamingAndConcurrentRequests(t *testing.T) {
	started := make(chan struct{})
	proceed := make(chan struct{})

	runner := &mockRunner{
		formats: []debris.FormatSpec{{ID: "eagle", DisplayName: "Eagle"}},
		convFn: func(_ context.Context, req debris.ConvertRequest, emit debris.EventCallback) (debris.ConvertResponse, error) {
			close(started)
			<-proceed
			emit(debris.Event{Kind: "stage", Stage: "read"})
			emit(debris.Event{Kind: "progress", Stage: "copy", Current: 1, Total: 2, AssetID: "a1"})
			emit(debris.Event{Kind: "progress", Stage: "copy", Current: 2, Total: 2, AssetID: "a2"})
			return debris.ConvertResponse{
				SourceFormat: req.From,
				TargetFormat: req.To,
				InputPath:    req.InputPath,
				OutputPath:   req.OutputPath,
				CopyMode:     req.CopyMode,
				AssetCount:   2,
				CopiedAssets: 2,
			}, nil
		},
	}

	srv, inW, msgs, done := startPipeServer(t, context.Background(), runner)

	// Start async convert
	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      "conv-1",
		"method":  "convert",
		"params": debris.ConvertRequest{
			From:       "billfish",
			To:         "eagle",
			InputPath:  "/in",
			OutputPath: "/out",
		},
	})
	<-started

	// While convert is in flight, send formats and probe concurrently
	writeLine(t, inW, map[string]any{"jsonrpc": "2.0", "id": 10, "method": "formats"})
	msg := readMsg(t, msgs)
	if string(msg.ID) != "10" || msg.Error != nil {
		t.Fatalf("concurrent formats failed: %+v", msg)
	}

	// Now unblock convert and collect 3 progress notifications + 1 final response
	close(proceed)

	var progressEvents []ProgressParams
	var finalResp *debris.ConvertResponse
	for i := 0; i < 4; i++ {
		m := readMsg(t, msgs)
		if m.Method == "progress" {
			var p ProgressParams
			if err := json.Unmarshal(m.Params, &p); err != nil {
				t.Fatalf("unmarshal progress params: %v", err)
			}
			if string(p.RequestID) != `"conv-1"` {
				t.Fatalf("unexpected progress requestId: %s", p.RequestID)
			}
			progressEvents = append(progressEvents, p)
			continue
		}
		if string(m.ID) == `"conv-1"` {
			if m.Error != nil {
				t.Fatalf("convert returned unexpected error: %+v", m.Error)
			}
			var r debris.ConvertResponse
			if err := json.Unmarshal(m.Result, &r); err != nil {
				t.Fatalf("unmarshal convert result: %v", err)
			}
			finalResp = &r
		}
	}

	if len(progressEvents) != 3 {
		t.Fatalf("expected 3 progress events, got %d", len(progressEvents))
	}
	if progressEvents[0].Event.Kind != "stage" || progressEvents[2].Event.Current != 2 {
		t.Fatalf("unexpected progress events: %+v", progressEvents)
	}
	if finalResp == nil || finalResp.CopiedAssets != 2 {
		t.Fatalf("unexpected final convert response: %+v", finalResp)
	}

	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("Serve error: %v", err)
	}
	if srv.activeCount() != 0 {
		t.Fatalf("expected active map to be empty, got %d", srv.activeCount())
	}
}

func TestServer_CancelMidFlightAndEOFCleanup(t *testing.T) {
	var wg sync.WaitGroup
	started := make(chan string, 2)

	runner := &mockRunner{
		convFn: func(ctx context.Context, req debris.ConvertRequest, emit debris.EventCallback) (debris.ConvertResponse, error) {
			started <- req.InputPath
			<-ctx.Done()
			return debris.ConvertResponse{}, debris.WrapError(debris.CodeCanceled, "conversion canceled", ctx.Err())
		},
	}

	srv, inW, msgs, done := startPipeServer(t, context.Background(), runner)

	// 1. Launch request id 101 and cancel via cancel request
	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      101,
		"method":  "convert",
		"params":  debris.ConvertRequest{To: "eagle", InputPath: "/job1", OutputPath: "/out1"},
	})
	if p := <-started; p != "/job1" {
		t.Fatalf("expected /job1, got %s", p)
	}

	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      102,
		"method":  "cancel",
		"params":  map[string]any{"requestId": 101},
	})

	// Expect cancel result + canceled convert error (order between goroutine and cancel response may vary)
	seen := map[string]rpcEnvelope{}
	for i := 0; i < 2; i++ {
		m := readMsg(t, msgs)
		seen[string(m.ID)] = m
	}

	cancelMsg, ok := seen["102"]
	if !ok || cancelMsg.Error != nil {
		t.Fatalf("unexpected cancel response: %+v", cancelMsg)
	}
	var cancelRes CancelResult
	if err := json.Unmarshal(cancelMsg.Result, &cancelRes); err != nil || !cancelRes.Canceled {
		t.Fatalf("expected canceled=true, got %s (%v)", cancelMsg.Result, err)
	}

	convMsg, ok := seen["101"]
	if !ok || convMsg.Error == nil || convMsg.Error.Code != ErrCodeCanceled {
		t.Fatalf("expected ErrCodeCanceled for 101, got %+v", convMsg)
	}

	// Ensure active map cleaned up after completion
	deadline := time.Now().Add(2 * time.Second)
	for srv.activeCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.activeCount() != 0 {
		t.Fatalf("expected active map empty after cancel, got %d", srv.activeCount())
	}

	// 2. Launch request id "notif-job" and cancel via cancel notification (no id)
	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      "notif-job",
		"method":  "convert",
		"params":  debris.ConvertRequest{To: "eagle", InputPath: "/job-notif", OutputPath: "/out-notif"},
	})
	if p := <-started; p != "/job-notif" {
		t.Fatalf("expected /job-notif, got %s", p)
	}
	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"method":  "cancel",
		"params":  map[string]any{"requestId": "notif-job"},
	})
	notifConvMsg := readMsg(t, msgs)
	if string(notifConvMsg.ID) != `"notif-job"` || notifConvMsg.Error == nil || notifConvMsg.Error.Code != ErrCodeCanceled {
		t.Fatalf("expected ErrCodeCanceled for notif-job without cancel response, got %+v", notifConvMsg)
	}

	// 3. Launch request id 201 and close stdin (EOF) while in-flight
	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      201,
		"method":  "convert",
		"params":  debris.ConvertRequest{To: "eagle", InputPath: "/job2", OutputPath: "/out2"},
	})
	if p := <-started; p != "/job2" {
		t.Fatalf("expected /job2, got %s", p)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		m := readMsg(t, msgs)
		if string(m.ID) != "201" || m.Error == nil || m.Error.Code != ErrCodeCanceled {
			t.Errorf("expected ErrCodeCanceled on EOF shutdown, got %+v", m)
		}
	}()

	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("Serve returned unexpected error on EOF: %v", err)
	}
	wg.Wait()
	if srv.activeCount() != 0 {
		t.Fatalf("expected active map empty on EOF, got %d", srv.activeCount())
	}
}

func TestServer_ParentContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})

	runner := &mockRunner{
		probeFn: func(ctx context.Context, _ debris.ProbeRequest) (debris.ProbeResponse, error) {
			close(started)
			<-ctx.Done()
			return debris.ProbeResponse{}, ctx.Err() // raw context.Canceled should map to ErrCodeCanceled
		},
	}

	srv, inW, msgs, done := startPipeServer(t, ctx, runner)
	defer func() { _ = inW.Close() }()

	writeLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      "p1",
		"method":  "probe",
		"params":  debris.ProbeRequest{Path: "/slow"},
	})
	<-started

	cancel()
	msg := readMsg(t, msgs)
	if msg.Error == nil || msg.Error.Code != ErrCodeCanceled {
		t.Fatalf("expected ErrCodeCanceled for raw context.Canceled, got %+v", msg.Error)
	}

	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled from Serve, got %v", err)
	}
	if srv.activeCount() != 0 {
		t.Fatalf("expected active map empty after ctx cancel, got %d", srv.activeCount())
	}
}
