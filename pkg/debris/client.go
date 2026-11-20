package debris

import (
	"context"
	"strings"
)

// Runner is the backend execution interface plugged into Engine (for example by
// internal/engine). Keeping Runner as a small interface decouples pkg/debris
// from internal packages while preserving a clean Go SDK surface.
type Runner interface {
	Formats() []FormatSpec
	Probe(ctx context.Context, req ProbeRequest) (ProbeResponse, error)
	Inspect(ctx context.Context, req InspectRequest) (InspectResponse, error)
	Plan(ctx context.Context, req PlanRequest) (PlanResponse, error)
	Convert(ctx context.Context, req ConvertRequest, emit EventCallback) (ConvertResponse, error)
}

// Option configures a Debris Engine instance.
type Option func(*Engine)

// WithRunner attaches a Runner implementation to the Engine.
func WithRunner(r Runner) Option {
	return func(e *Engine) {
		e.runner = r
	}
}

// WithFormats configures fallback FormatSpecs returned when no Runner is attached.
func WithFormats(specs ...FormatSpec) Option {
	return func(e *Engine) {
		e.formats = append([]FormatSpec(nil), specs...)
	}
}

// Engine is the stateless facade for Debris library probing, inspection,
// planning, and conversion.
type Engine struct {
	runner  Runner
	formats []FormatSpec
}

// NewEngine creates an Engine configured with the provided options.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	return e
}

// Formats returns the list of registered library formats and their capabilities.
func (e *Engine) Formats() []FormatSpec {
	if e != nil && e.runner != nil {
		return e.runner.Formats()
	}
	if e == nil || len(e.formats) == 0 {
		return nil
	}
	out := make([]FormatSpec, len(e.formats))
	copy(out, e.formats)
	return out
}

// Probe detects the format of the library at req.Path.
func (e *Engine) Probe(ctx context.Context, req ProbeRequest) (ProbeResponse, error) {
	if err := checkContext(ctx); err != nil {
		return ProbeResponse{}, err
	}
	if strings.TrimSpace(req.Path) == "" {
		return ProbeResponse{}, NewError(CodeInvalidRequest, "probe path is required", "")
	}
	if e == nil || e.runner == nil {
		return ProbeResponse{}, NewError(CodeNotImplemented, "engine runner is not configured", "")
	}
	return e.runner.Probe(ctx, req)
}

// Inspect returns a lightweight summary of the library at req.Path.
func (e *Engine) Inspect(ctx context.Context, req InspectRequest) (InspectResponse, error) {
	if err := checkContext(ctx); err != nil {
		return InspectResponse{}, err
	}
	if strings.TrimSpace(req.Path) == "" {
		return InspectResponse{}, NewError(CodeInvalidRequest, "inspect path is required", "")
	}
	if e == nil || e.runner == nil {
		return InspectResponse{}, NewError(CodeNotImplemented, "engine runner is not configured", "")
	}
	return e.runner.Inspect(ctx, req)
}

// Plan validates conversion parameters and computes projected counts and warnings.
func (e *Engine) Plan(ctx context.Context, req PlanRequest) (PlanResponse, error) {
	if err := checkContext(ctx); err != nil {
		return PlanResponse{}, err
	}
	mode, err := normalizeCopyMode(req.CopyMode)
	if err != nil {
		return PlanResponse{}, err
	}
	req.CopyMode = mode
	if err := validateConversionPaths(req.To, req.InputPath, req.OutputPath); err != nil {
		return PlanResponse{}, err
	}
	if e == nil || e.runner == nil {
		return PlanResponse{}, NewError(CodeNotImplemented, "engine runner is not configured", "")
	}
	return e.runner.Plan(ctx, req)
}

// Convert runs a library conversion and streams Events to emit.
func (e *Engine) Convert(ctx context.Context, req ConvertRequest, emit EventCallback) (ConvertResponse, error) {
	if err := checkContext(ctx); err != nil {
		return ConvertResponse{}, err
	}
	mode, err := normalizeCopyMode(req.CopyMode)
	if err != nil {
		return ConvertResponse{}, err
	}
	req.CopyMode = mode
	if err := validateConversionPaths(req.To, req.InputPath, req.OutputPath); err != nil {
		return ConvertResponse{}, err
	}
	if e == nil || e.runner == nil {
		return ConvertResponse{}, NewError(CodeNotImplemented, "engine runner is not configured", "")
	}
	return e.runner.Convert(ctx, req, emit)
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return NewError(CodeInvalidRequest, "context must not be nil", "")
	}
	if err := ctx.Err(); err != nil {
		return WrapError(CodeCanceled, "operation canceled", err)
	}
	return nil
}

func normalizeCopyMode(mode CopyMode) (CopyMode, error) {
	normalized := CopyMode(strings.ToLower(strings.TrimSpace(string(mode))))
	if normalized == "" {
		return CopyModeAuto, nil
	}
	switch normalized {
	case CopyModeCopy, CopyModeHardlink, CopyModeAuto:
		return normalized, nil
	default:
		return "", NewError(CodeInvalidRequest, "invalid copyMode", "expected copy, hardlink, or auto")
	}
}

func validateConversionPaths(to, inputPath, outputPath string) error {
	if strings.TrimSpace(to) == "" {
		return NewError(CodeInvalidRequest, "target format (to) is required", "")
	}
	if strings.TrimSpace(inputPath) == "" {
		return NewError(CodeInvalidRequest, "inputPath is required", "")
	}
	if strings.TrimSpace(outputPath) == "" {
		return NewError(CodeInvalidRequest, "outputPath is required", "")
	}
	return nil
}
