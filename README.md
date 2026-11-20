# Debris

**Bridge the Gap Between Design Library Formats.**

[简体中文](docs/README/README_zh-Hans.md) | [繁體中文](docs/README/README_zh-Hant.md)

Debris is a cross-platform command-line tool (and JSON-RPC 2.0 stdio server) for inspecting, planning, and converting design asset libraries across formats. It translates libraries directly through a unified domain model—preserving folders, tags, tag groups, colors, ratings, annotations, origin URLs, and asset files without needing an intermediate format.

## Supported Formats

| Format | Read (Import) | Write (Export) | Notes |
| :--- | :---: | :---: | :--- |
| **Eagle** (`.library`, 4.x) | ✅ | ✅ | Tested & verified (including Hive compatibility) |
| **Pixcall** (`.pixcall`, v22) | ✅ | ✅ | Tested & verified (see [Pixcall compatibility notes](docs/pixcall-compatibility.md)) |
| **Billfish** (`.bf`) | ✅ | ⚠️ Unmaintained | Supported for migrating *out* of Billfish. Billfish is discontinued, so exporting *to* Billfish is not actively maintained. |

## Features

* **Direct Cross-Format Conversion:** Convert directly between supported formats (e.g., `billfish` → `eagle`, `billfish` → `pixcall`, `eagle` ↔ `pixcall`).
* **Format Auto-Detection:** Omit `--from` / `--format` and Debris will automatically probe the input library format.
* **Inspection & Pre-Flight Planning:** Use `inspect` to summarize a library and `plan` to preview asset counts, warnings, and potential compatibility losses before converting.
* **Flexible Asset Transfer:** Supports `copy`, `hardlink`, and `auto` (`--copy-mode`), along with `--overwrite`, `--include-deleted`, `--include-hidden`, and `--name`.
* **Machine-Readable Output:** Emit NDJSON progress/result streams with `--json`.
* **Cross-Platform:** Written in Go (`1.23+`) for macOS, Linux, and Windows.

## Usage

### Convert a Library

```sh
# Convert Billfish to Eagle
debris convert --from billfish --to eagle --input ./lib.billfish --output ./lib.library

# Convert Billfish to Pixcall (source format auto-detected)
debris convert -t pixcall -i ./lib.billfish -o ./pixcall-lib

# Convert Eagle to Pixcall using hardlinks and NDJSON output
debris convert -t pixcall -i ./lib.library -o ./pixcall-lib --copy-mode hardlink --json

# Convert Pixcall to Eagle
debris convert -t eagle -i ./pixcall-lib -o ./lib.library
```

#### Common `convert` Flags

* `--from`, `-f`: Source format (`billfish`, `eagle`, `pixcall`). Auto-detected if omitted.
* `--to`, `-t`: Destination format (`eagle`, `pixcall`).
* `--input`, `-i`: Input library directory path.
* `--output`, `-o`: Output library directory path.
* `--copy-mode`: Asset transfer mode: `auto` (default), `copy`, or `hardlink`.
* `--overwrite`: Overwrite existing target directory contents.
* `--include-deleted`: Include deleted/trashed items.
* `--include-hidden`: Include hidden items.
* `--name`: Override the target library name.
* `--json`: Emit NDJSON progress events and the final result.

### Inspect a Library

```sh
debris inspect --path ./lib.library
debris inspect -p ./pixcall-lib --json
```

### Preview a Conversion Plan

```sh
debris plan --to pixcall --input ./lib.library --output ./pixcall-lib
```
