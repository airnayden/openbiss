// Package openapi embeds the OpenBISS OpenAPI 3.0 specification (openapi.yaml)
// and the vendored Swagger UI static assets (swagger-ui/*), exposing helpers
// the HTTP server uses to serve /openapi.json and /docs/*.
package openapi

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml
var specYAML []byte

//go:embed swagger-ui
var swaggerUIRoot embed.FS

var (
	specJSONOnce sync.Once
	specJSONData []byte
	specJSONErr  error
)

// SpecJSON returns the OpenAPI 3.0 specification serialised as JSON. The
// embedded YAML is parsed on first call and the resulting JSON is cached;
// subsequent calls return the cached buffer without re-parsing.
//
// The returned slice MUST NOT be mutated by callers.
func SpecJSON() ([]byte, error) {
	specJSONOnce.Do(func() {
		var doc interface{}
		if err := yaml.Unmarshal(specYAML, &doc); err != nil {
			specJSONErr = fmt.Errorf("openapi: unmarshal yaml: %w", err)
			return
		}
		converted, err := stringifyKeys(doc)
		if err != nil {
			specJSONErr = fmt.Errorf("openapi: convert keys: %w", err)
			return
		}
		j, err := json.MarshalIndent(converted, "", "  ")
		if err != nil {
			specJSONErr = fmt.Errorf("openapi: marshal json: %w", err)
			return
		}
		specJSONData = j
	})
	return specJSONData, specJSONErr
}

// SwaggerUIFS returns a filesystem view of the vendored Swagger UI assets
// rooted at the swagger-ui/ directory. Use with http.FileServer to serve
// the files at /docs/*.
//
// If sub-rooting fails (impossible at runtime but defensively handled), the
// returned FS responds with not-found errors on every Open.
func SwaggerUIFS() fs.FS {
	sub, err := fs.Sub(swaggerUIRoot, "swagger-ui")
	if err != nil {
		return errFS{err: err}
	}
	return sub
}

type errFS struct {
	err error
}

func (e errFS) Open(_ string) (fs.File, error) {
	return nil, e.err
}

// Compile-time assertion: errFS satisfies fs.FS.
var _ fs.FS = errFS{}

// stringifyKeys walks a yaml.v3-decoded interface tree and replaces any
// map[interface{}]interface{} keys with string keys so encoding/json can
// marshal the result. yaml.v3 decodes string-keyed maps to
// map[string]interface{} directly, but we defensively handle the legacy
// map[interface{}]interface{} form used by yaml.v2 as well.
func stringifyKeys(v interface{}) (interface{}, error) {
	switch m := v.(type) {
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, val := range m {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("openapi: non-string map key %v (%T)", k, k)
			}
			conv, err := stringifyKeys(val)
			if err != nil {
				return nil, err
			}
			out[ks] = conv
		}
		return out, nil
	case map[string]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, val := range m {
			conv, err := stringifyKeys(val)
			if err != nil {
				return nil, err
			}
			out[k] = conv
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(m))
		for i, val := range m {
			conv, err := stringifyKeys(val)
			if err != nil {
				return nil, err
			}
			out[i] = conv
		}
		return out, nil
	default:
		return v, nil
	}
}
