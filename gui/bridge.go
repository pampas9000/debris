package main

import (
	"context"
	"sync"

	"debris/pkg/debris"

	"github.com/egoist/mygo"
)

// ConvertEvent streams conversion progress, stage changes, and warnings to the frontend.
var ConvertEvent = mygo.NewEvent[debris.Event]("convert-event")

// DebrisService exposes Debris Engine operations and native desktop helpers to MyGo.
type DebrisService struct {
	eng *debris.Engine

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewDebrisService constructs a DebrisService backed by eng.
func NewDebrisService(eng *debris.Engine) *DebrisService {
	return &DebrisService{eng: eng}
}

// Formats returns all registered library formats and their capabilities.
func (s *DebrisService) Formats() []debris.FormatSpec {
	return s.eng.Formats()
}

// PickDirectory opens a native OS directory picker attached to the caller window.
func (s *DebrisService) PickDirectory(ctx context.Context, title string) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:            mygo.CallerWindow(ctx),
		Title:             title,
		Directory:         true,
		CreateDirectories: true,
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

// Probe detects the library format at req.Path.
func (s *DebrisService) Probe(ctx context.Context, req debris.ProbeRequest) (debris.ProbeResponse, error) {
	return s.eng.Probe(ctx, req)
}

// Inspect returns a lightweight summary of the library at req.Path.
func (s *DebrisService) Inspect(ctx context.Context, req debris.InspectRequest) (debris.InspectResponse, error) {
	return s.eng.Inspect(ctx, req)
}

// Plan previews a conversion and reports projected counts, warnings, and compatibility losses.
func (s *DebrisService) Plan(ctx context.Context, req debris.PlanRequest) (debris.PlanResponse, error) {
	return s.eng.Plan(ctx, req)
}

// Convert runs a library conversion and broadcasts progress events to the frontend.
func (s *DebrisService) Convert(ctx context.Context, req debris.ConvertRequest) (debris.ConvertResponse, error) {
	runCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = cancel
	s.mu.Unlock()

	defer func() {
		cancel()
		s.mu.Lock()
		s.cancel = nil
		s.mu.Unlock()
	}()

	return s.eng.Convert(runCtx, req, func(ev debris.Event) {
		ConvertEvent.Broadcast(ev)
	})
}

// CancelConvert cancels the currently running conversion, if any.
func (s *DebrisService) CancelConvert() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}
