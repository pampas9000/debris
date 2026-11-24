# Debris

**Bridge the Gap Between Design Library Formats.**

[简体中文](docs/README/README_zh-Hans.md) | [繁體中文](docs/README/README_zh-Hant.md)

Debris is a cross-platform Desktop GUI (MyGo + SolidJS) and CLI tool for inspecting, planning, and converting design asset libraries across formats. It translates libraries directly through a unified domain model—preserving folders, tags, tag groups, colors, ratings, annotations, origin URLs, and asset files without needing an intermediate format.

![Debris Desktop App](docs/assets/screenshot.avif)

## Supported Formats

| Format | Read (Import) | Write (Export) | Notes |
| :--- | :---: | :---: | :--- |
| **Eagle** (4.0.0 build 23) | ✅ | ✅ | Tested & verified (including Hive compatibility) |
| **Pixcall** (0.9.7+build5) | ✅ | ✅ | Tested & verified (see [Pixcall compatibility notes](docs/pixcall-compatibility.md)) |
| **Refto** (0.1.0-beta.10) | ✅ | ✅ | Tested & verified (see [Refto compatibility notes](docs/refto-compatibility.md)) |
| **Billfish** (3.0.32) | ✅ | ⚠️ Unmaintained | Supported for migrating *out* of Billfish. Billfish is discontinued, so exporting *to* Billfish is not actively maintained. |

For a complete per-platform feature matrix and details on how cross-format differences are handled, see the [Format Compatibility & Feature Handling Guide](docs/format-compatibility.md).

## Features

* **Desktop GUI (MyGo + SolidJS):** Native cross-platform desktop app for macOS, Windows, and Linux featuring directory auto-detection, live library stats, pre-flight compatibility planning, real-time progress with cancellation, System/Light/Dark themes, and i18n (`English`, `简体中文`, `繁體中文`).
* **Direct Cross-Format Conversion:** Convert directly between supported formats (e.g., `eagle` ↔ `pixcall` ↔ `refto`, `billfish` → `eagle` / `pixcall` / `refto`).
* **Format Auto-Detection & Smart Output Paths:** Omit `--from` / `--format` to auto-detect the input library format, and omit `--output` to automatically derive the target path next to the input library.
* **Inspection & Pre-Flight Planning:** Use `inspect` to summarize a library and `plan` to preview asset counts, warnings, and potential compatibility losses before converting.
* **Flexible Asset Transfer:** Supports `auto` (default, prefers hardlinks and falls back to copy across volumes), `hardlink`, and `copy` (`--copy-mode`), along with `--overwrite`, `--include-deleted`, `--include-hidden`, `--include-suggested-tags`, and `--name`.
* **Script-Friendly Output:** Supports inline TTY progress, `--quiet` (`-q`), and machine-readable JSON / NDJSON output via `--json`.

## Installation

Download pre-built binaries and desktop installers from **[GitHub Releases](https://github.com/pampas9000/debris/releases/latest)**:

* **Desktop GUI:**
  * **macOS** (Universal Apple Silicon + Intel): `Debris-*.dmg`
  * **Windows** (`amd64` / `arm64`): `Debris-*-setup-*.exe`
  * **Linux** (`amd64` / `arm64`): `.deb`, `.rpm`, or `.tar.gz`
* **CLI (`debris`):**
  * Download the `debris-<version>-<os>-<arch>.zip` archive for your platform and add `debris` to your `PATH`, or install with Go:
    ```sh
    go install github.com/pampas9000/debris@latest
    ```

> [!TIP]
> On macOS, if Gatekeeper warns about an unidentified developer on first launch, right-click `Debris.app` and choose **Open** (or allow it in **System Settings → Privacy & Security**).

## Desktop App

1. **Select Source Library:** Click **Browse…** in **Source Library** and pick your existing library folder (Eagle, Pixcall, Refto, or Billfish — no specific folder suffix is required). Debris automatically detects the format from the folder structure and displays asset, folder, tag, and size statistics.
2. **Configure Target Output:** Choose the **Target Format** (`Eagle`, `Pixcall`, or `Refto`), pick a **Save Location** (parent directory), and adjust the **Library Name (Folder)** if needed.
3. **Choose Transfer Mode & Options:**
   * **Auto** (recommended): Creates hardlinks on the same disk to save space, and automatically falls back to copying across different volumes.
   * **Hardlink**: Zero extra disk usage (requires source and target on the same volume).
   * **Copy**: Duplicates all asset files (safest across drives, uses additional disk space).
   * Toggle **Overwrite target**, **Include trashed**, **Include hidden**, or **Include AI-suggested tags** (when supported by the source format) as needed.
4. **Plan & Convert:** Click **Plan** to preview projected counts and any compatibility notes/losses before touching disk, then click **Convert** to start. You can cancel a running conversion at any time.
5. **Theme & Language:** Use the top-right controls to switch between **System / Light / Dark** themes and **English / 简体中文 / 繁體中文**.

## CLI Usage

### Convert a Library

`debris convert [input] [output]` accepts both positional arguments and flags. If `[output]` / `--output` is omitted, Debris automatically derives a target folder path next to the input directory based on its name and `--to`.

```sh
# Convert Billfish to Eagle using positional arguments
debris convert ./billfish-lib ./eagle-lib -t eagle

# Convert Billfish to Pixcall (source format auto-detected, output path auto-derived)
debris convert ./billfish-lib -t pixcall

# Convert Eagle to Pixcall using flags, hardlinks, and NDJSON output
debris convert -t pixcall -i ./eagle-lib -o ./pixcall-lib --copy-mode hardlink --json

# Convert Pixcall to Eagle quietly
debris convert ./pixcall-lib -t eagle -q
```

**`convert` Flags:**

* `--from`, `-f`: Source format (`billfish`, `eagle`, `pixcall`, `refto`). Auto-detected if omitted.
* `--to`, `-t`: Destination format (`eagle`, `pixcall`, `refto`, `billfish`).
* `--input`, `-i`: Input library path (can also be passed as the 1st positional argument).
* `--output`, `-o`: Output library path (can also be passed as the 2nd positional argument; derived from input and `--to` if omitted).
* `--copy-mode`: Asset transfer mode: `auto` (default), `copy`, or `hardlink`.
* `--overwrite`: Overwrite existing target directory contents.
* `--include-deleted`: Include deleted/trashed items.
* `--include-hidden`: Include hidden/archived items.
* `--include-suggested-tags`: Include AI-suggested (unconfirmed) tags when supported by the source format (e.g. Refto).
* `--name`: Override the target library name.
* `--quiet`, `-q`: Suppress progress output in non-JSON mode.
* `--json`: Emit NDJSON progress events and the final result.

### Inspect a Library

```sh
debris inspect ./eagle-lib
debris inspect -p ./pixcall-lib --json
```

**`inspect` Flags:**

* `--path`, `-p`: Library path to inspect (can also be passed as a positional argument).
* `--format`, `-f`: Source format (auto-detected if omitted).
* `--include-deleted`: Include deleted/trashed items.
* `--include-hidden`: Include hidden/archived items.
* `--include-suggested-tags`: Include AI-suggested (unconfirmed) tags when supported by the source format.
* `--json`: Output inspection result as JSON.

### Preview a Conversion Plan

`debris plan [input] [output]` supports the same source, target, transfer, and filter flags as `convert` (except `--quiet`), and outputs JSON when `--json` is passed.

```sh
debris plan ./billfish-lib -t eagle
debris plan -t pixcall -i ./eagle-lib -o ./pixcall-lib --json
```

## Development

### Prerequisites

* **Go** `1.27+`
* **[Bun](https://bun.sh/)** (required only for building/running the Desktop GUI)

### Project Structure

* [`cmd/`](cmd/) — Cobra CLI commands (`convert`, `inspect`, `plan`)
* [`pkg/debris/`](pkg/debris/) — Public Go Engine API (`Engine`, `Probe`, `Inspect`, `Plan`, `Convert`)
* [`internal/domain/`](internal/domain/) — Unified domain model (`Library`, `Asset`, `Folder`, `Tag`)
* [`internal/engine/`](internal/engine/) — Conversion runner, pre-flight planner, and format registry
* [`internal/format/`](internal/format/) — Format adapters (`eagle`, `pixcall`, `refto`, `billfish`)
* [`gui/`](gui/) — Desktop GUI built with **MyGo** + **SolidJS** + **Tailwind CSS v4** + **Vite**

### Build & Test CLI

```sh
# Run all Go tests
go test ./...

# Run static checks
go vet ./...

# Build CLI binary
go build -o debris .
```

### Develop & Build Desktop GUI

```sh
cd gui
bun install

# Start desktop app in development mode (hot-reload frontend + Go backend)
bun run dev

# Typecheck frontend
bun run typecheck

# Build production desktop bundle
bun run build
```

### Embedding Debris as a Go Library

Use [`cmd.NewDefaultEngine()`](cmd/root.go) (or wire custom adapters via [`pkg/debris`](pkg/debris/)) to call `Probe`, `Inspect`, `Plan`, and `Convert` directly from Go code.
