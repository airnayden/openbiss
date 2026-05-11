BINARY    := openbiss
MODULE    := github.com/openbiss/openbiss
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   := -ldflags "-s -w -X $(MODULE)/internal/config.Version=$(VERSION)"
# PKG_LDFLAGS: raw linker flags (no outer `-ldflags "..."` wrapper) for fyne-cross.
# fyne-cross's own -ldflags flag splits this on whitespace and re-wraps each token.
PKG_LDFLAGS := -s -w -X $(MODULE)/internal/config.Version=$(VERSION)

.PHONY: build build-darwin build-darwin-arm build-windows build-linux build-all clean vet \
        package-darwin package-windows package-linux package-all fyne-deps

build:
	go build $(LDFLAGS) -o $(BINARY) .

build-darwin:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-amd64 .

build-darwin-arm:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-arm64 .

build-windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-windows-amd64.exe .

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64 .

build-all: build-darwin build-darwin-arm build-windows build-linux

vet:
	go vet ./...

clean:
	rm -rf dist/ $(BINARY) $(BINARY).exe

# ── Fyne packaging (fyne-cross) ───────────────────────────────────────────
# The package-* targets cross-compile and bundle the GUI app for each platform
# using fyne-cross. fyne-cross runs each build inside a platform-specific
# Docker image, so Docker MUST be installed and running on the host.
#
# First-time setup:   make fyne-deps
# Build everything:   make package-all
#
# Outputs land in ./fyne-cross/dist/{darwin,windows,linux}/

package-darwin:
	fyne-cross darwin -arch amd64,arm64 -icon assets/icon.png -name OpenBISS -app-id com.openbiss.openbiss -ldflags "$(PKG_LDFLAGS)"

package-windows:
	fyne-cross windows -arch amd64 -icon assets/icon.png -name OpenBISS -app-id com.openbiss.openbiss -ldflags "$(PKG_LDFLAGS)"

package-linux:
	fyne-cross linux -arch amd64 -icon assets/icon.png -name OpenBISS -app-id com.openbiss.openbiss -ldflags "$(PKG_LDFLAGS)"

package-all: package-darwin package-windows package-linux

fyne-deps:
	go install fyne.io/tools/cmd/fyne@latest
	go install github.com/fyne-io/fyne-cross@latest
