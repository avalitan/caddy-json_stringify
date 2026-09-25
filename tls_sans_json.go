package tlssansjson

import (
	"encoding/json"
	"net/http"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

const placeholder = "http.request.tls.client.san.dns_names_json"

func init() {
	caddy.RegisterModule(Handler{})
}

// Handler adds a JSON-encoded client-certificate name list to Caddy's replacer.
// The first element is the certificate Subject Common Name (when non-empty),
// followed by every DNS SAN from the leaf client certificate.
type Handler struct{}

// CaddyModule returns the Caddy module information.
func (Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.tls_sans_json",
		New: func() caddy.Module { return new(Handler) },
	}
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
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

	repl := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
	repl.Set(placeholder, string(encoded))

	return next.ServeHTTP(w, r)
}

// Interface guard.
var _ caddyhttp.MiddlewareHandler = (*Handler)(nil)
