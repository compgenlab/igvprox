# igvprox

`igvprox` is a small Go service for viewing genomics data on a remote HPC cluster through a local web browser.

You run `igvprox` on the cluster against one or more files or directories. It binds an HTTP server to a UNIX socket, and you expose that server locally with SSH port forwarding. The browser then loads a minimal `igv.js` page from the proxy and streams the selected tracks through range requests.

This is designed for the common case where the data should stay on-cluster and the user already has SSH access.

## What It Does

- Serves a minimal `igv.js` page for an ephemeral viewing session
- Discovers genomics files from CLI-provided paths
- Supports optional recursive directory scanning
- Sorts tracks by full file path
- Serves only the discovered files and their indexes
- Listens on a UNIX socket instead of a public TCP port on the cluster

## Supported Formats

Phase 1 support is aimed at these common file types:

- `BAM` with `.bai`
- `CRAM` with `.crai`
- `VCF.gz` with `.tbi` or `.csi`
- `BED.gz` with `.tbi` or `.csi`
- `BedGraph` as `.bedgraph.gz` or `.bg.gz` with `.tbi` or `.csi`
- `BigWig` / `.bw`
- `BigBed` / `.bb`
- plain `BED`
- plain `SAM`

Notes:

- Indexed formats require an index by default.
- `BigWig` and `BigBed` are treated as self-indexed.
- Plain `BED` and plain `SAM` are recognized, but they do not have the same indexing behavior as compressed or binary formats and may not perform well.

## Build

`igvprox` uses a single top-level `Makefile`.

```sh
make build
```

The binary is written to:

```text
bin/igvprox
```

Run tests with:

```sh
make test
```

## Basic Usage

Serve everything directly under `output/`:

```sh
./bin/igvprox output/
```

Serve recursively under `output/`:

```sh
./bin/igvprox -R output/
```

Serve a specific set of files:

```sh
./bin/igvprox sample1.bam sample2.bam cohort.vcf.gz
```

Override the reference genome for one session:

```sh
./bin/igvprox -g hg19 -R output/
```

Override the socket path:

```sh
./bin/igvprox -s /tmp/igvprox.sock -R output/
```

Allow indexed formats without a discovered index:

```sh
./bin/igvprox --allow-missing-index output/
```

## SSH Tunnel Workflow

On the HPC cluster:

```sh
./bin/igvprox -R output/
```

By default the server listens on:

- `$XDG_RUNTIME_DIR/igvprox.sock` when `XDG_RUNTIME_DIR` exists
- otherwise `/tmp/igvprox-$UID.sock`

From your local machine, forward a local port to the remote UNIX socket:

```sh
ssh -L 8080:/tmp/igvprox-$(id -u).sock user@cluster
```

Then open:

```text
http://localhost:8080
```

If the socket path is different, forward that path instead.

## CLI

```text
igvprox [flags] <path> [<path> ...]
```

Flags:

- `-R`, `--recursive`: recursively scan directory arguments
- `-g`, `--genome <id>`: override the reference genome
- `-s`, `--socket <path>`: set the UNIX socket path
- `--config <path>`: load a config file from an explicit path
- `--allow-missing-index`: do not reject missing sidecar indexes
- `-v`, `--verbose`: enable verbose logging
- `--open-browser-url <url>`: override the printed browser URL hint

Defaults:

- genome: `hg38`
- browser URL hint: `http://localhost:8080`
- recursion: off
- sorting: ascending full path

## Config

Config lookup order:

1. `--config <path>`
2. `~/.config/igvprox/config.toml`
3. `~/.igvproxrc`

Supported config keys:

```toml
genome = "hg38"
browser_url = "http://localhost:8080"
socket_path = ""
allow_missing_index = false
```

If `socket_path` is empty, the runtime default socket path is used.

## HTTP Interface

The server exposes a minimal internal API:

- `GET /`: embedded `igv.js` page
- `GET /api/session`: JSON session manifest
- `GET /files/<id>/data`: data file endpoint
- `GET /files/<id>/index`: index file endpoint

The proxy serves only the files discovered from the command line inputs. It does not expose arbitrary file-system access.

## GitHub Actions

The repository includes a GitHub Actions workflow that builds the binary for:

- `linux/amd64`
- `linux/arm64`

The workflow runs on:

- pushes to `main`
- pull requests

## Current Scope

The current implementation is intentionally small:

- single-user
- ephemeral session
- one process started manually over SSH
- minimal browser UI

Desktop IGV session export and richer session management can be added later on top of the same discovery and file-serving model.
