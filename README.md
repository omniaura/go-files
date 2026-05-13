# go-files

File and blob storage utilities for Go — Backblaze B2, S3-compatible
object stores (AWS, MinIO, Hippius), presigned URLs, and media
conversion helpers.

Companion to [`omniaura/go-kit`](https://github.com/omniaura/go-kit).
Modules here target Go 1.25.5+ and prefer the Go 1.26.1 toolchain when
available.

## Backends

| Module     | Status   | Notes                                                  |
| ---------- | -------- | ------------------------------------------------------ |
| `hippius/` | ✅ ready | S3 client preset for the Hippius decentralized gateway |
| `s3/`      | planned  | Generic S3-compatible client (AWS, MinIO, B2 S3-compat)|
| `b2/`      | planned  | Backblaze native B2 API (uploads w/o S3 facade)        |
| `blob/`    | planned  | Backend-agnostic interface (Upload, Get, Presign, …)   |
| `presign/` | planned  | Presigned URL helpers + validity checking              |
| `keypath/` | planned  | Object-key path validation (no traversal, subfolders)  |
| `mediaconv/` | planned | ffmpeg image/audio format conversion                 |

## Package naming convention

**Do not shadow standard library package names.** When picking a name
for a new sub-package, check `go doc std` first. The following names
are off-limits because they collide with the stdlib:

- `bufio`, `bytes`, `embed`, `errors`, `io`, `mime`, `os`, `path`
- `image` (and `image/jpeg`, `image/png`, `image/gif`)
- `archive`, `compress`, `crypto`, `encoding`, `hash`
- `net`, `time`, `sync`, `context`, `fmt`, `log`

Prefer descriptive non-colliding names. For example:

| Domain                | Avoid              | Prefer                       |
| --------------------- | ------------------ | ---------------------------- |
| Blob/object storage   | `io`, `os`         | `blob`, `blobstore`          |
| Presigned URLs        | `url`              | `presign`                    |
| Path validation       | `path`, `filepath` | `keypath`, `objkey`          |
| MIME / content type   | `mime`             | `mimetype`, `contenttype`    |
| Media conversion      | `image`            | `mediaconv`, `imgconv`       |

## Workspace layout

Each sub-package is its own Go module so consumers can import only
what they need without dragging in heavy dependencies (B2 SDK, AWS
SDK, ffmpeg bindings, etc.).

```
go-files/
├── go.work             # development workspace
├── <pkg>/              # each sub-package is its own module
│   ├── go.mod
│   └── *.go
```

## Testing

Run tests across all workspace modules:

```
./scripts/test-all.sh
```
