# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

igvprox is a Go service that serves genomics files (BAM, CRAM, VCF, BigWig, etc.) over a UNIX socket HTTP server for viewing in a local browser via igv.js. It's designed for HPC cluster use with SSH port forwarding. Zero external Go dependencies — stdlib only.

## Build Commands

```bash
make build              # Build to bin/igvprox
make test               # Run tests (go test ./...)
make fmt                # Format code (gofmt)
make clean              # Remove bin/
make build-linux-amd64  # Cross-compile for Linux AMD64
make build-linux-arm64  # Cross-compile for Linux ARM64
```

Builds use CGO_ENABLED=0 for static binaries.

## Architecture

```
cmd/igvprox/main.go          # CLI entry point, flag parsing, config merging, startup
internal/
  config/config.go            # TOML-like config parsing, socket path resolution, Track/Config structs
  discovery/discovery.go       # File classification, index validation, format detection
  server/
    server.go                  # HTTP routes, range-aware file serving, track management
    static/index.html          # Embedded single-file web UI (HTML/CSS/JS with igv.js from CDN)
```

**Request flow:** CLI args → config merge (CLI > config file > defaults) → file discovery with index validation → UNIX socket HTTP server → igv.js UI fetches `/api/session` for track manifest → streams file data via `/files/<id>/data` with HTTP range requests.

**Key API routes:**
- `GET /api/session` — track manifest (genome, tracks, hostname, cwd)
- `GET /api/browse` — filesystem browser for adding tracks dynamically
- `POST /api/track` — add track at runtime
- `GET /files/<id>/data` and `/files/<id>/index` — serve file content with range support

**Config search order:** `--config` flag → `~/.config/igvprox/config.toml` → `~/.igvproxrc`

**Socket path resolution:** explicit config → `$XDG_RUNTIME_DIR/igvprox.sock` → `/tmp/igvprox-$UID.sock`

## Supported Formats

Indexed (require sidecar index unless `--allow-missing-index`): BAM (.bai), CRAM (.crai), VCF.gz (.tbi/.csi), BED.gz (.tbi/.csi), BedGraph.gz (.tbi/.csi). Self-indexed: BigWig (.bw/.bigwig), BigBed (.bb/.bigbed). Unindexed (with warning): plain BED, plain SAM.

## Design Constraints

- Single-user, ephemeral sessions — no auth, relies on UNIX socket permissions (0600) and SSH tunnel
- File IDs are SHA1 hashes of absolute paths
- Server only serves discovered or dynamically-added files, never arbitrary filesystem access
- Web UI is embedded via `go:embed` — changes to `static/index.html` require rebuild
- Thread-safe dynamic track addition uses RWMutex in server
