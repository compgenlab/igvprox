# Plan

## Recommended Product Shape

Build `igvprox` as a Go daemon that serves:

- a UNIX-socket-bound HTTP service on the cluster
- a static `igv.js` client
- JSON endpoints for dataset discovery and manifest generation
- byte-range endpoints for genomic file access

Use SSH local port forwarding from the user's machine to the remote UNIX socket.

Primary invocation model:

- user SSHes into cluster
- user runs `igvprox` against explicit files or directories
- user forwards local TCP port to the remote UNIX socket
- user opens local browser to use the ephemeral `igv.js` page

## Phase 1

1. Bootstrap Go module and project layout.
2. Implement CLI parsing with defaults for socket path, genome, recursion, and logging.
3. Implement config parsing from `~/.config/igvprox/config.toml` and `~/.igvproxrc`.
4. Implement file discovery for supported genomics types.
5. Implement format/index validation, with indexes required by default where applicable.
6. Implement HTTP server over UNIX socket.
7. Implement range-aware file serving endpoints.
8. Implement dynamic track-manifest generation sorted by file path.
9. Add minimal `igv.js` UI that loads tracks through the proxy.
10. Add a single top-level `Makefile` that builds into `bin/`.
11. Add GitHub Actions CI for Linux `amd64` and `arm64` builds on `main` pushes and pull requests.
12. Document SSH tunnel usage and local access flow.

## Phase 2

1. Add richer filtering and track grouping.
2. Add saved sessions or bookmarkable URLs.
3. Add optional desktop IGV export/session generation.
4. Add auditing/logging and stronger access controls.
5. Add automated tests for range serving and discovery behavior.

## Decision Record

Chosen first interface: custom `igv.js` site.

Deferred alternative: IGV desktop data XML/session integration.

Reason: `igv.js` gives tighter control, simpler delivery through SSH-forwarded local HTTP, and a cleaner foundation for a new Go service.

## Fixed Assumptions

- single-user process
- ephemeral session
- minimal browser UI
- standard reference genomes
- track sources come from CLI-provided paths
- recursive scanning is opt-in
- indexed formats require indexes by default
- config defaults come from `~/.config/igvprox/config.toml` then `~/.igvproxrc`
