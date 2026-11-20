# Debris

**Bridge the Gap Between Design Library Formats.**

[简体中文](docs/README/README_zh-Hans.md) | [繁體中文](docs/README/README_zh-Hant.md)

Debris is a cross-platform CLI tool and Desktop GUI application (powered by MyGo + SolidJS) for inspecting, planning, and converting design asset libraries across formats. It translates libraries directly through a unified domain model—preserving folders, tags, tag groups, colors, ratings, annotations, origin URLs, and asset files without needing an intermediate format.

## Supported Formats

| Format | Read (Import) | Write (Export) | Notes |
| :--- | :---: | :---: | :--- |
| **Eagle** (`.library`, 4.x) | ✅ | ✅ | Tested & verified (including Hive compatibility) |
| **Pixcall** (`.pixcall`, v22) | ✅ | ✅ | Tested & verified (see [Pixcall compatibility notes](docs/pixcall-compatibility.md)) |
| **Billfish** (`.bf`) | ✅ | ⚠️ Unmaintained | Supported for migrating *out* of Billfish. Billfish is discontinued, so exporting *to* Billfish is not actively maintained. |

## Features

* **Desktop GUI (MyGo + SolidJS):** Native cross-platform desktop app for macOS, Windows, and Linux featuring directory auto-detection, live library stats, pre-flight compatibility planning, real-time progress with cancellation, System/Light/Dark themes, and i18n (`English`, `简体中文`, `繁體中文`).
* **Direct Cross-Format Conversion:** Convert directly between supported formats (e.g., `billfish` → `eagle`, `billfish` → `pixcall`, `eagle` ↔ `pixcall`).
* **Format Auto-Detection & Smart Output Paths:** Omit `--from` / `--format` to auto-detect the input library format, and omit `--output` to automatically derive the target path next to the input library.
* **Inspection & Pre-Flight Planning:** Use `inspect` to summarize a library and `plan` to preview asset counts, warnings, and potential compatibility losses before converting.
* **Flexible Asset Transfer:** Supports `auto` (default, prefers hardlinks and falls back to copy across volumes), `hardlink`, and `copy` (`--copy-mode`), along with `--overwrite`, `--include-deleted`, `--include-hidden`, and `--name`.
* **Script-Friendly Output:** Supports inline TTY progress, `--quiet` (`-q`), and machine-readable JSON / NDJSON output via `--json`.
* **Cross-Platform:** Written in Go (`1.27+`) for macOS, Linux, and Windows.

## Desktop GUI

The desktop app lives in [`gui/`](gui/) and is built with **MyGo** and **SolidJS** (Tailwind CSS v4 + TypeScript + Vite).

Pre-built desktop packages (`.dmg` for macOS, installer `.exe` for Windows, and `.deb` / `.rpm` / `.tar.gz` for Linux) are available on the Releases page.

### Run & Build from Source

Requires Go (`1.27+`) and [Bun](https://bun.sh/).

```sh
cd gui
bun install

# Start desktop app in development mode
bun run dev

# Typecheck & build desktop bundle
bun run typecheck
bun run build
```

## CLI Usage

### Convert a Library

`debris convert [input] [output]` accepts positional arguments as well as flags. If `[output]` / `--output` is omitted, Debris derives it automatically from the input path and `--to` (e.g., `./lib.billfish` with `-t eagle` → `./lib.library`).

```sh
# Convert Billfish to Eagle using positional arguments
debris convert ./lib.billfish ./lib.library -t eagle

# Convert Billfish to Pixcall (source format auto-detected, output path auto-derived)
debris convert ./lib.billfish -t pixcall

# Convert Eagle to Pixcall using flags, hardlinks, and NDJSON output
debris convert -t pixcall -i ./lib.library -o ./pixcall-lib --copy-mode hardlink --json

# Convert Pixcall to Eagle quietly
debris convert ./pixcall-lib -t eagle -q
```

#### `convert` Flags

* `--from`, `-f`: Source format (`billfish`, `eagle`, `pixcall`). Auto-detected if omitted.
* `--to`, `-t`: Destination format (`eagle`, `pixcall`, `billfish`).
* `--input`, `-i`: Input library path (can also be passed as the 1st positional argument).
* `--output`, `-o`: Output library path (can also be passed as the 2nd positional argument; derived from input and `--to` if omitted).
* `--copy-mode`: Asset transfer mode: `auto` (default), `copy`, or `hardlink`.
* `--overwrite`: Overwrite existing target directory contents.
* `--include-deleted`: Include deleted/trashed items.
* `--include-hidden`: Include hidden items.
* `--name`: Override the target library name.
* `--quiet`, `-q`: Suppress progress output in non-JSON mode.
* `--json`: Emit NDJSON progress events and the final result.

### Inspect a Library

```sh
debris inspect ./lib.library
debris inspect -p ./pixcall-lib --json
```

#### `inspect` Flags

* `--path`, `-p`: Library path to inspect (can also be passed as a positional argument).
* `--format`, `-f`: Source format (auto-detected if omitted).
* `--include-deleted`: Include deleted/trashed items.
* `--include-hidden`: Include hidden items.
* `--json`: Output inspection result as JSON.

### Preview a Conversion Plan

`debris plan [input] [output]` supports the same source, target, transfer, and filter flags as `convert` (except `--quiet`), and outputs JSON when `--json` is passed.

```sh
debris plan ./lib.billfish -t eagle
debris plan -t pixcall -i ./lib.library -o ./pixcall-lib --json
```
