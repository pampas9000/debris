package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"debris/internal/format"
	"debris/pkg/debris"
)

// Registry stores registered library format adapters in registration order
// and provides format lookup and auto-detection.
type Registry struct {
	order    []string
	adapters map[string]format.Adapter
}

// NewRegistry constructs a Registry from the provided format adapters.
// Nil adapters or adapters with empty IDs are ignored. If multiple adapters
// share the same case-insensitive ID, the later one replaces the earlier one.
func NewRegistry(adapters ...format.Adapter) *Registry {
	reg := &Registry{
		adapters: make(map[string]format.Adapter, len(adapters)),
	}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		id := normalizeFormatID(adapter.ID())
		if id == "" {
			continue
		}
		if _, exists := reg.adapters[id]; !exists {
			reg.order = append(reg.order, id)
		}
		reg.adapters[id] = adapter
	}
	return reg
}

// List returns the FormatSpec for every registered adapter in registration order.
func (r *Registry) List() []debris.FormatSpec {
	if r == nil || len(r.order) == 0 {
		return nil
	}
	specs := make([]debris.FormatSpec, 0, len(r.order))
	for _, id := range r.order {
		adapter := r.adapters[id]
		if adapter == nil {
			continue
		}
		specs = append(specs, debris.FormatSpec{
			ID:           adapter.ID(),
			DisplayName:  adapter.DisplayName(),
			Capabilities: toDebrisCapabilities(adapter.Capabilities()),
		})
	}
	return specs
}

// Get returns the adapter registered for id (case-insensitive).
func (r *Registry) Get(id string) (format.Adapter, bool) {
	if r == nil {
		return nil, false
	}
	adapter, ok := r.adapters[normalizeFormatID(id)]
	return adapter, ok
}

// Detect probes all registered adapters against path and returns the
// highest-confidence match. If path does not exist it returns CodeNotFound;
// if no adapter matches it returns CodeProbeFailed.
func (r *Registry) Detect(ctx context.Context, path string) (format.Adapter, format.ProbeResult, error) {
	if err := checkContext(ctx); err != nil {
		return nil, format.ProbeResult{}, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, format.ProbeResult{}, debris.NewError(debris.CodeInvalidRequest, "library path is required", "")
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, format.ProbeResult{}, debris.WrapError(debris.CodeNotFound, "library path does not exist", err)
		}
		return nil, format.ProbeResult{}, debris.WrapError(debris.CodeIO, "failed to access library path", err)
	}
	if r == nil || len(r.order) == 0 {
		return nil, format.ProbeResult{}, debris.NewError(debris.CodeProbeFailed, "no format adapters registered", path)
	}

	var (
		bestAdapter format.Adapter
		bestResult  format.ProbeResult
		matched     bool
		probeErrs   []string
	)

	for _, id := range r.order {
		if err := checkContext(ctx); err != nil {
			return nil, format.ProbeResult{}, err
		}
		adapter := r.adapters[id]
		if adapter == nil {
			continue
		}
		res, err := adapter.Probe(ctx, path)
		if err != nil {
			if ctxErr := checkContext(ctx); ctxErr != nil {
				return nil, format.ProbeResult{}, ctxErr
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, format.ProbeResult{}, debris.WrapError(debris.CodeCanceled, "probe canceled", err)
			}
			probeErrs = append(probeErrs, fmt.Sprintf("%s: %v", adapter.ID(), err))
			continue
		}
		if !res.Matched {
			continue
		}
		if res.Format == "" {
			res.Format = adapter.ID()
		}
		if res.RootPath == "" {
			res.RootPath = path
		}
		if !matched || res.Confidence > bestResult.Confidence {
			bestAdapter = adapter
			bestResult = res
			matched = true
		}
	}

	if !matched {
		detail := fmt.Sprintf("path=%s", path)
		if len(probeErrs) > 0 {
			detail = fmt.Sprintf("%s; probe errors: %s", detail, strings.Join(probeErrs, "; "))
		}
		return nil, format.ProbeResult{}, debris.NewError(debris.CodeProbeFailed, "no matching library format found", detail)
	}

	return bestAdapter, bestResult, nil
}

func normalizeFormatID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func toDebrisCapabilities(c format.Capabilities) debris.Capabilities {
	return debris.Capabilities{
		SupportsFolders:          c.SupportsFolders,
		SupportsNestedFolders:    c.SupportsNestedFolders,
		SupportsMultiFolderAsset: c.SupportsMultiFolderAsset,
		SupportsTagGroups:        c.SupportsTagGroups,
		SupportsNestedTags:       c.SupportsNestedTags,
		SupportsColors:           c.SupportsColors,
		SupportsRecycleBin:       c.SupportsRecycleBin,
		SupportsThumbnails:       c.SupportsThumbnails,
		SupportsRating:           c.SupportsRating,
		SupportsOriginURL:        c.SupportsOriginURL,
		SupportsAnnotation:       c.SupportsAnnotation,
	}
}
