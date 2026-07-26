//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/airnayden/openbiss/internal/server/openapi"
)

func main() {
	spec, err := openapi.SpecJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: SpecJSON: %v\n", err)
		os.Exit(1)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(spec, &doc); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: JSON unmarshal: %v\n", err)
		os.Exit(1)
	}
	info, _ := doc["info"].(map[string]interface{})
	title, _ := info["title"].(string)
	if title != "OpenBISS API" {
		fmt.Fprintf(os.Stderr, "FAIL: info.title = %q, want \"OpenBISS API\"\n", title)
		os.Exit(1)
	}
	fmt.Printf("PASS: SpecJSON size=%d bytes, info.title=%q\n", len(spec), title)

	fsys := openapi.SwaggerUIFS()
	f, err := fsys.Open("index.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: open index.html: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: read index.html: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("PASS: swagger-ui/index.html opened, size=%d bytes\n", len(b))
}
