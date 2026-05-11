# OpenBISS — Brand Assets

This directory contains the source-of-truth icon assets for the OpenBISS desktop application.

## Files

| File | Dimensions | Format | Purpose |
|---|---|---|---|
| `icon.png` | 1024×1024 | PNG / RGBA, 8-bit | Primary application icon. Consumed by `fyne package` → auto-converted to `.icns` (macOS) and `.ico` (Windows) at build time. |
| `tray-light.png` | 22×22 | PNG / RGBA, 8-bit | Menu-bar / system-tray icon for **dark** menu bars (white silhouette on transparent). Also serves as the macOS template-image source — Fyne handles the dark/light inversion automatically on macOS. |
| `tray-dark.png` | 22×22 | PNG / RGBA, 8-bit | Menu-bar / system-tray icon for **light** menu bars (black silhouette on transparent). |

## Design

- **Background colour (main icon):** `#1a2744` — dark navy blue, OpenBISS brand colour.
- **Foreground colour:** `#ffffff` (main icon and `tray-light.png`) / `#000000` (`tray-dark.png`).
- **Glyph:** smart-card outline (rounded rectangle, ~700×900, stroke ~28 px) enclosing a minimalist padlock (filled body + arched shackle). Flat, modern, macOS-compatible aesthetic — no gradients, no drop shadows, no stylistic ornament.
- **Tray glyph:** simplified card silhouette only (lock omitted at this scale to remain legible at 22×22).

## Source

All three PNGs are generated programmatically using **Go standard library only** (`image`, `image/color`, `image/png`, `math`) — no external image-editing tool, no third-party dependency, no binary blob committed without a deterministic source.

The generator script lives at `/tmp/gen_icons.go` (intentionally **not** committed to the repository — see task spec). Re-run with:

```bash
go run /tmp/gen_icons.go assets/
```

Rendering technique: per-pixel signed-distance-function evaluation with 1-pixel smooth-step coverage anti-aliasing on shape boundaries.

## Generated Build Artefacts

`fyne package` produces platform-specific icon containers from `icon.png` at build time. These are **not** checked into git (see top-level `.gitignore`):

- `assets/*.icns` — macOS `.app` bundle icon (multi-resolution).
- `assets/*.ico` — Windows `.exe` resource icon (multi-resolution).

## License

These assets are licensed under the **MIT License**, the same terms as the rest of the OpenBISS project. They contain no copyrighted third-party imagery (no Bulgarian state coat of arms, no BORICA logos, no bank logos) and may be redistributed freely under the MIT terms.
