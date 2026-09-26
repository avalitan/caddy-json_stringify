package tlssansjson

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

const (
	legacyDestination = "http.request.tls.client.san.dns_names_json"
)

func init() {
	caddy.RegisterModule(Handler{})
}

// Handler JSON-encodes a native Caddy replacer value and stores the encoded
// string in another replacer value.
//
// Source and Destination may be written either as bare replacer keys:
//
//   http.request.tls.client.san.dns_names
//
// or as normal Caddy placeholders:
//
//   {http.request.tls.client.san.dns_names}
//
// For backwards compatibility, when both fields are omitted the handler keeps
// its original behavior: the client certificate Common Name is prepended to
// the DNS SAN list and the JSON string is stored in
// http.request.tls.client.san.dns_names_json.
type Handler struct {
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
}

// CaddyModule returns the Caddy module information.
func (Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.tls_sans_json",
		New: func() caddy.Module { return new(Handler) },
	}
}

// Validate validates the handler configuration.
func (h Handler) Validate() error {
	// Both omitted means legacy mode.
	if h.Source == "" && h.Destination == "" {
		return nil
	}

	if normalizePlaceholder(h.Source) == "" {
		return fmt.Errorf("source must be set when destination is set")
	}
	if normalizePlaceholder(h.Destination) == "" {
		return fmt.Errorf("destination must be set when source is set")
	}

	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	repl, ok := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
	if !ok || repl == nil {
		return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("request replacer is unavailable"))
	}

	// Backwards-compatible behavior for existing configurations which have no
	// source/destination fields.
	if h.Source == "" && h.Destination == "" {
		names := make([]string, 0)

		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			cert := r.TLS.PeerCertificates[0]

			if cert.Subject.CommonName != "" {
				names = append(names, cert.Subject.CommonName)
			}
			names = append(names, cert.DNSNames...)
		}

		encoded, err := json.Marshal(names)
		if err != nil {
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}

		repl.Set(legacyDestination, string(encoded))
		return next.ServeHTTP(w, r)
	}

	source := normalizePlaceholder(h.Source)
	destination := normalizePlaceholder(h.Destination)

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
