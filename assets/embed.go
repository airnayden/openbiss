// Package assets exposes binary assets (icons, images) bundled into the
// OpenBISS binary at compile time via Go's //go:embed directive.
//
// Lives at the module root (not internal/gui) because Go's embed directive
// forbids ".." in patterns, so the embedding .go file must be co-located
// with the asset files it references.
package assets

import _ "embed"

// IconPNG is the 1254×1254 RGB application icon (assets/icon.png), used as
// the Fyne app icon (taskbar, dock, window decorations).
//
//go:embed icon.png
var IconPNG []byte

// TrayLightPNG is the 22×22 RGBA tray icon for dark menu bars (white
// silhouette on transparent). On macOS, Fyne treats this as the template
// image source and handles dark/light inversion automatically.
//
//go:embed tray-light.png
var TrayLightPNG []byte

// TrayDarkPNG is the 22×22 RGBA tray icon for light menu bars (black
// silhouette on transparent). Used on Linux/Windows where the desktop
// environment does not auto-invert tray icons.
//
//go:embed tray-dark.png
var TrayDarkPNG []byte
