package tlssansjson

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

// Handler JSON-encodes a native Caddy replacer value and stores the encoded
// JSON text as a string in another replacer value.
//
// Source aliases:
//   - source
//   - get
//   - src
//
// Destination aliases:
//   - destination
//   - set
//   - dest
//
// Values may be written either as bare replacer keys:
//
//   http.request.tls.client.san.dns_names
//
// or as normal Caddy placeholders:
//
//   {http.request.tls.client.san.dns_names}
type Handler struct {
	Source      string `json:"source,omitempty"`
	Get         string `json:"get,omitempty"`
	Src         string `json:"src,omitempty"`
	Destination string `json:"destination,omitempty"`
	Set         string `json:"set,omitempty"`
	Dest        string `json:"dest,omitempty"`
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
	if _, err := resolveAlias("source", h.Source, h.Get, h.Src); err != nil {
		return err
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

	source, err := resolveAlias("source", h.Source, h.Get, h.Src)
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}
	destination, err := resolveAlias("destination", h.Destination, h.Set, h.Dest)
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	value, found := repl.Get(source)
	if !found {
		return caddyhttp.Error(
			http.StatusInternalServerError,
			fmt.Errorf("source placeholder {%s} is unknown", source),
		)
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return caddyhttp.Error(
			http.StatusInternalServerError,
			fmt.Errorf("JSON-encoding source placeholder {%s}: %w", source, err),
		)
	}

	repl.Set(destination, string(encoded))
	return next.ServeHTTP(w, r)
}

func resolveAlias(kind string, values ...string) (string, error) {
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
			return "", fmt.Errorf("conflicting %s aliases configured", kind)
		}
	}

	if resolved == "" {
		return "", fmt.Errorf("%s is required", kind)
	}

	return resolved, nil
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
