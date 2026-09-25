# caddy-tls-sans-json

Tiny Caddy v2 HTTP handler which exposes this placeholder:

```text
{http.request.tls.client.san.dns_names_json}
```

The value is valid JSON. It contains the leaf client certificate's Common Name first (if present), followed by all DNS SANs.

Example certificate:

```text
CN = client01
DNS SANs = foo.example.com, bar.example.com
```

produces:

```json
["client01","foo.example.com","bar.example.com"]
```

## Build

With `xcaddy` installed, from this directory:

```bash
xcaddy build v2.11.4 \
  --with github.com/avalitan/caddy-tls-sans-json=.
```

For an existing custom build, add the same `--with` argument alongside your other modules.

## JSON config

Add the handler before anything which consumes the placeholder:

```json
{
  "handler": "tls_sans_json"
},
{
  "handler": "request_body",
  "set": "{\"host\":{http.request.tls.client.san.dns_names_json}}"
}
```

For the config in the accompanying discussion, the route object can use:

```json
{
  "handler": "request_body",
  "set": "\\{\"@id\":\"site_auth_{client_cert_cn}\",\"match\":[\\{\"host\":{http.request.tls.client.san.dns_names_json}\\}],\"handle\":[\\{\"handler\":\"subroute\",\"@id\":\"site_rules_{client_cert_cn}\",\"routes\":[]\\}]\\}"
}
```

The plugin does not require any configuration fields of its own.
