package protocol

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"debris/pkg/debris"
)

// JSONRPCVersion is the supported JSON-RPC protocol version.
const JSONRPCVersion = "2.0"

// Standard JSON-RPC 2.0 and Debris server error codes.
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603

	ErrCodeUnsupportedFormat = -32001
	ErrCodeNotFound          = -32002
	ErrCodeProbeFailed       = -32003
	ErrCodeNotImplemented    = -32004
	ErrCodeCanceled          = -32005
	ErrCodeIO                = -32006
)

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// CancelParams holds parameters for the JSON-RPC "cancel" method.
type CancelParams struct {
	RequestID json.RawMessage `json:"requestId"`
}

// CancelResult is returned when "cancel" is invoked as a JSON-RPC request.
type CancelResult struct {
	Canceled bool `json:"canceled"`
}

// ProgressParams is the params payload emitted on "progress" notifications.
type ProgressParams struct {
	RequestID json.RawMessage `json:"requestId"`
	Event     debris.Event    `json:"event"`
}

type requestMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type successResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
}

type errorResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   *RPCError       `json:"error"`
}

type notificationMessage struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Server implements a stateless JSON-RPC 2.0 NDJSON server over arbitrary streams.
type Server struct {
	engine *debris.Engine

	mu     sync.Mutex
	active map[string]context.CancelFunc

	wg sync.WaitGroup
}

// NewServer constructs a JSON-RPC 2.0 Server backed by engine.
func NewServer(engine *debris.Engine) *Server {
	return &Server{
		engine: engine,
		active: make(map[string]context.CancelFunc),
	}
}

// ServeStdio runs the JSON-RPC 2.0 server over os.Stdin and os.Stdout.
func ServeStdio(ctx context.Context, engine *debris.Engine) error {
	return NewServer(engine).Serve(ctx, os.Stdin, os.Stdout)
}

// Serve reads NDJSON JSON-RPC 2.0 messages from r and writes responses and
// progress notifications thread-safely to w until r reaches EOF or ctx is canceled.
// On shutdown, all in-flight requests are canceled and drained before returning.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	if s == nil || s.engine == nil {
		return errors.New("protocol server requires a non-nil debris.Engine")
	}
	if r == nil || w == nil {
		return errors.New("protocol server requires non-nil reader and writer")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	serverCtx, cancelServer := context.WithCancel(ctx)
	defer cancelServer()

	writer := newSafeWriter(w)

	type scanItem struct {
		line []byte
		err  error
	}
	lines := make(chan scanItem)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			select {
			case lines <- scanItem{line: line}:
			case <-serverCtx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case lines <- scanItem{err: err}:
			case <-serverCtx.Done():
			}
		}
	}()

	var exitErr error
readLoop:
	for {
		select {
		case <-ctx.Done():
			exitErr = ctx.Err()
			break readLoop
		case item, ok := <-lines:
			if !ok {
				break readLoop
			}
			if item.err != nil {
				exitErr = item.err
				break readLoop
			}
			s.handleLine(serverCtx, writer, item.line)
		}
	}

	cancelServer()
	s.wg.Wait()
	return exitErr
}

func (s *Server) handleLine(serverCtx context.Context, writer *safeWriter, line []byte) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return
	}

	if !json.Valid(trimmed) {
		_ = writer.writeError(json.RawMessage("null"), &RPCError{
			Code:    ErrCodeParse,
			Message: "parse error",
		})
		return
	}
	if trimmed[0] != '{' {
		_ = writer.writeError(json.RawMessage("null"), &RPCError{
			Code:    ErrCodeInvalidRequest,
			Message: "invalid request",
		})
		return
	}

	var req requestMessage
	if err := json.Unmarshal(trimmed, &req); err != nil {
		_ = writer.writeError(json.RawMessage("null"), &RPCError{
			Code:    ErrCodeInvalidRequest,
			Message: "invalid request",
			Data:    err.Error(),
		})
		return
	}

	id, idKey, isNotification, validID := parseMessageID(req.ID)
	if !validID {
		_ = writer.writeError(json.RawMessage("null"), &RPCError{
			Code:    ErrCodeInvalidRequest,
			Message: "invalid request id",
		})
		return
	}

	if req.JSONRPC != JSONRPCVersion || req.Method == "" {
		if !isNotification {
			_ = writer.writeError(id, &RPCError{
				Code:    ErrCodeInvalidRequest,
				Message: "invalid request",
			})
		}
		return
	}

	switch req.Method {
	case "formats":
		s.handleFormats(writer, id, isNotification, req.Params)
	case "cancel":
		s.handleCancel(writer, id, isNotification, req.Params)
	case "probe":
		var payload debris.ProbeRequest
		if !s.decodeAndLaunch(serverCtx, writer, id, idKey, isNotification, req.Params, &payload, func(ctx context.Context) (any, error) {
			return s.engine.Probe(ctx, payload)
		}) {
			return
		}
	case "inspect":
		var payload debris.InspectRequest
		if !s.decodeAndLaunch(serverCtx, writer, id, idKey, isNotification, req.Params, &payload, func(ctx context.Context) (any, error) {
			return s.engine.Inspect(ctx, payload)
		}) {
			return
		}
	case "plan":
		var payload debris.PlanRequest
		if !s.decodeAndLaunch(serverCtx, writer, id, idKey, isNotification, req.Params, &payload, func(ctx context.Context) (any, error) {
			return s.engine.Plan(ctx, payload)
		}) {
			return
		}
	case "convert":
		var payload debris.ConvertRequest
		if !s.decodeAndLaunch(serverCtx, writer, id, idKey, isNotification, req.Params, &payload, func(ctx context.Context) (any, error) {
			var finished atomic.Bool
			emit := func(ev debris.Event) {
				if finished.Load() || isNotification {
					return
				}
				_ = writer.writeNotification("progress", ProgressParams{
					RequestID: id,
					Event:     ev,
				})
			}
			resp, err := s.engine.Convert(ctx, payload, emit)
			finished.Store(true)
			return resp, err
		}) {
			return
		}
	default:
		if !isNotification {
			_ = writer.writeError(id, &RPCError{
				Code:    ErrCodeMethodNotFound,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			})
		}
	}
}

func (s *Server) handleFormats(writer *safeWriter, id json.RawMessage, isNotification bool, rawParams json.RawMessage) {
	if err := validateOptionalEmptyParams(rawParams); err != nil {
		if !isNotification {
			_ = writer.writeError(id, &RPCError{
				Code:    ErrCodeInvalidParams,
				Message: "invalid params",
				Data:    err.Error(),
			})
		}
		return
	}
	if isNotification {
		return
	}
	formats := s.engine.Formats()
	if formats == nil {
		formats = []debris.FormatSpec{}
	}
	_ = writer.writeResult(id, formats)
}

func (s *Server) handleCancel(writer *safeWriter, id json.RawMessage, isNotification bool, rawParams json.RawMessage) {
	var params CancelParams
	if err := unmarshalObjectParams(rawParams, &params); err != nil {
		if !isNotification {
			_ = writer.writeError(id, &RPCError{
				Code:    ErrCodeInvalidParams,
				Message: "invalid params",
				Data:    err.Error(),
			})
		}
		return
	}
	targetID, targetKey, targetMissing, targetValid := parseMessageID(params.RequestID)
	if !targetValid || targetMissing || bytes.Equal(targetID, []byte("null")) {
		if !isNotification {
			_ = writer.writeError(id, &RPCError{
				Code:    ErrCodeInvalidParams,
				Message: "requestId is required",
			})
		}
		return
	}

	s.mu.Lock()
	cancelFn, found := s.active[targetKey]
	s.mu.Unlock()

	if found && cancelFn != nil {
		cancelFn()
	}

	if !isNotification {
		_ = writer.writeResult(id, CancelResult{Canceled: found})
	}
}

func (s *Server) decodeAndLaunch(
	serverCtx context.Context,
	writer *safeWriter,
	id json.RawMessage,
	idKey string,
	isNotification bool,
	rawParams json.RawMessage,
	dst any,
	fn func(ctx context.Context) (any, error),
) bool {
	if err := unmarshalObjectParams(rawParams, dst); err != nil {
		if !isNotification {
			_ = writer.writeError(id, &RPCError{
				Code:    ErrCodeInvalidParams,
				Message: "invalid params",
				Data:    err.Error(),
			})
		}
		return false
	}

	reqCtx, cancelFn := context.WithCancel(serverCtx)
	if !isNotification {
		s.mu.Lock()
		if _, exists := s.active[idKey]; exists {
			s.mu.Unlock()
			cancelFn()
			_ = writer.writeError(id, &RPCError{
				Code:    ErrCodeInvalidRequest,
				Message: "duplicate in-flight request id",
			})
			return false
		}
		s.active[idKey] = cancelFn
		s.mu.Unlock()
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancelFn()
		if !isNotification {
			defer s.removeActive(idKey)
		}

		res, err := fn(reqCtx)
		if isNotification {
			return
		}
		if err != nil {
			_ = writer.writeError(id, mapError(err))
			return
		}
		_ = writer.writeResult(id, res)
	}()

	return true
}

func (s *Server) removeActive(idKey string) {
	if idKey == "" {
		return
	}
	s.mu.Lock()
	delete(s.active, idKey)
	s.mu.Unlock()
}

func (s *Server) activeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.active)
}

func parseMessageID(raw json.RawMessage) (compact json.RawMessage, key string, missing bool, valid bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, "", true, true
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err != nil {
		return nil, "", false, false
	}
	b := buf.Bytes()
	switch {
	case b[0] == '"':
		return json.RawMessage(b), string(b), false, true
	case bytes.Equal(b, []byte("null")):
		return json.RawMessage(b), "null", false, true
	case b[0] == '-' || (b[0] >= '0' && b[0] <= '9'):
		var num json.Number
		if err := json.Unmarshal(b, &num); err != nil {
			return nil, "", false, false
		}
		return json.RawMessage(b), string(b), false, true
	default:
		return nil, "", false, false
	}
}

func validateOptionalEmptyParams(raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	var obj map[string]any
	if trimmed[0] != '{' {
		return errors.New("params must be an object or null")
	}
	return json.Unmarshal(trimmed, &obj)
}

func unmarshalObjectParams(raw json.RawMessage, dst any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("params must be a JSON object")
	}
	return json.Unmarshal(trimmed, dst)
}

func mapError(err error) *RPCError {
	var de *debris.Error
	if !errors.As(err, &de) {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			de = debris.WrapError(debris.CodeCanceled, "operation canceled", err)
		} else {
			de = debris.AsError(err)
		}
	}
	return &RPCError{
		Code:    jsonRPCCodeFor(de.Code),
		Message: de.Message,
		Data:    de,
	}
}

func jsonRPCCodeFor(code string) int {
	switch code {
	case debris.CodeInvalidRequest:
		return ErrCodeInvalidParams
	case debris.CodeUnsupportedFormat:
		return ErrCodeUnsupportedFormat
	case debris.CodeNotFound:
		return ErrCodeNotFound
	case debris.CodeProbeFailed:
		return ErrCodeProbeFailed
	case debris.CodeNotImplemented:
		return ErrCodeNotImplemented
	case debris.CodeCanceled:
		return ErrCodeCanceled
	case debris.CodeIO:
		return ErrCodeIO
	default:
		return ErrCodeInternal
	}
}

type safeWriter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func newSafeWriter(w io.Writer) *safeWriter {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &safeWriter{enc: enc}
}

func (sw *safeWriter) writeResult(id json.RawMessage, result any) error {
	return sw.encode(successResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	})
}

func (sw *safeWriter) writeError(id json.RawMessage, rpcErr *RPCError) error {
	return sw.encode(errorResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   rpcErr,
	})
}

func (sw *safeWriter) writeNotification(method string, params any) error {
	return sw.encode(notificationMessage{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  params,
	})
}

func (sw *safeWriter) encode(v any) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.enc.Encode(v)
}
