package jsonstringify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(Handler{})
}

// Handler JSON-encodes native Caddy replacer values and stores the encoded
// JSON text as a string in another replacer value.
//
// A single source uses source/get/src. Multiple sources use
// sources/gets/srcs and are merged into one flat JSON array.
//
// Destination aliases:
//   - destination
//   - set
//   - dest
//
// Source expressions in multi-source mode support:
//
//   {placeholder}   fetch the native placeholder value
//   [{placeholder}] wrap the native placeholder value in an array
//
// Arrays contributed by sources are flattened one level into the merged array.
type Handler struct {
	Source      string   `json:"source,omitempty"`
	Get         string   `json:"get,omitempty"`
	Src         string   `json:"src,omitempty"`
	Sources     []string `json:"sources,omitempty"`
	Gets        []string `json:"gets,omitempty"`
	Srcs        []string `json:"srcs,omitempty"`
	Destination string   `json:"destination,omitempty"`
	Set         string   `json:"set,omitempty"`
	Dest        string   `json:"dest,omitempty"`
}

// CaddyModule returns the Caddy module information.
func (Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.json_stringify",
		New: func() caddy.Module { return new(Handler) },
	}
}

// Validate validates the handler configuration.
func (h Handler) Validate() error {
	_, singleSet, err := resolveOptionalAlias("source", h.Source, h.Get, h.Src)
	if err != nil {
		return err
	}
	_, multiSet, err := resolveSliceAlias("sources", h.Sources, h.Gets, h.Srcs)
	if err != nil {
		return err
	}

	if singleSet == multiSet {
		if singleSet {
			return fmt.Errorf("configure either source or sources, not both")
		}
		return fmt.Errorf("source or sources is required")
	}

	if _, err := resolveAlias("destination", h.Destination, h.Set, h.Dest); err != nil {
		return err
	}
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	repl, ok := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
	if !ok || repl == nil {
		return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("request replacer is unavailable"))
	}

	destination, err := resolveAlias("destination", h.Destination, h.Set, h.Dest)
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	source, singleSet, err := resolveOptionalAlias("source", h.Source, h.Get, h.Src)
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}
	sources, multiSet, err := resolveSliceAlias("sources", h.Sources, h.Gets, h.Srcs)
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	if singleSet == multiSet {
		if singleSet {
			return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("configure either source or sources, not both"))
		}
		return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("source or sources is required"))
	}

	var value any
	if singleSet {
		var found bool
		value, found = repl.Get(source)
		if !found {
			return caddyhttp.Error(
				http.StatusInternalServerError,
				fmt.Errorf("source placeholder {%s} is unknown", source),
			)
		}
	} else {
		merged := make([]any, 0)
		for i, expression := range sources {
			part, err := evaluateSourceExpression(repl, expression)
			if err != nil {
				return caddyhttp.Error(
					http.StatusInternalServerError,
					fmt.Errorf("evaluating sources[%d]: %w", i, err),
				)
			}

			if array, ok := part.([]any); ok {
				merged = append(merged, array...)
			} else {
				merged = append(merged, part)
			}
		}
		value = merged
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("JSON-encoding value: %w", err))
	}

	repl.Set(destination, string(encoded))
	return next.ServeHTTP(w, r)
}

// evaluateSourceExpression evaluates one entry from sources.
//
// An exact {placeholder} returns the placeholder's native value.
// An exact [{placeholder}] returns a one-element array containing that native
// value. Other expressions are placeholder-expanded and then parsed as JSON;
// if the expanded value is not valid JSON, it is treated as a plain string.
func evaluateSourceExpression(repl *caddy.Replacer, expression string) (any, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, fmt.Errorf("source expression is empty")
	}

	if key, ok := exactWrappedPlaceholder(expression); ok {
		value, found := repl.Get(key)
		if !found {
			return nil, fmt.Errorf("source placeholder {%s} is unknown", key)
		}
		return []any{value}, nil
	}

	if key, ok := exactPlaceholder(expression); ok {
		value, found := repl.Get(key)
		if !found {
			return nil, fmt.Errorf("source placeholder {%s} is unknown", key)
		}
		return normalizeJSONValue(value), nil
	}

	expanded := repl.ReplaceKnown(expression, "")
	var value any
	if err := json.Unmarshal([]byte(expanded), &value); err == nil {
		return value, nil
	}
	return expanded, nil
}

// normalizeJSONValue round-trips a native Go value through encoding/json so
// slices/maps with concrete Go types become []any/map[string]any. This lets
// multi-source merging recognize arrays consistently.
func normalizeJSONValue(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return value
	}
	return normalized
}

func exactPlaceholder(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 3 || value[0] != '{' || value[len(value)-1] != '}' {
		return "", false
	}
	key := strings.TrimSpace(value[1 : len(value)-1])
	if key == "" || strings.ContainsAny(key, "{}") {
		return "", false
	}
	return key, true
}

func exactWrappedPlaceholder(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 5 || value[0] != '[' || value[len(value)-1] != ']' {
		return "", false
	}
	return exactPlaceholder(strings.TrimSpace(value[1 : len(value)-1]))
}

func resolveAlias(kind string, values ...string) (string, error) {
	resolved, set, err := resolveOptionalAlias(kind, values...)
	if err != nil {
		return "", err
	}
	if !set {
		return "", fmt.Errorf("%s is required", kind)
	}
	return resolved, nil
}

func resolveOptionalAlias(kind string, values ...string) (string, bool, error) {
	var resolved string

	for _, value := range values {
		value = normalizePlaceholder(value)
		if value == "" {
			continue
		}
		if resolved == "" {
			resolved = value
			continue
		}
		if value != resolved {
			return "", false, fmt.Errorf("conflicting %s aliases configured", kind)
		}
	}

	return resolved, resolved != "", nil
}

func resolveSliceAlias(kind string, values ...[]string) ([]string, bool, error) {
	var resolved []string

	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		if resolved == nil {
			resolved = append([]string(nil), value...)
			continue
		}
		if !equalStrings(resolved, value) {
			return nil, false, fmt.Errorf("conflicting %s aliases configured", kind)
		}
	}

	if len(resolved) == 0 {
		return nil, false, nil
	}
	for i, value := range resolved {
		if strings.TrimSpace(value) == "" {
			return nil, false, fmt.Errorf("%s[%d] is empty", kind, i)
		}
	}
	return resolved, true, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizePlaceholder(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '{' && value[len(value)-1] == '}' {
		value = value[1 : len(value)-1]
	}
	return strings.TrimSpace(value)
}

// Interface guards.
var (
	_ caddyhttp.MiddlewareHandler = (*Handler)(nil)
	_ caddy.Validator              = (*Handler)(nil)
)
