<picture>
	<source media="(prefers-color-scheme: dark)" srcset=".github/banner.svg">
	<source media="(prefers-color-scheme: light)" srcset=".github/banner-light.svg">
	<img alt="rawr" src=".github/banner-light.svg">
</picture>

A small HTTP service for exposing remote files through configurable host and path mappings.

rawr supports path variables, document indexes, extension-specific MIME types, allowlists, response caching, stale responses and conditional upstream requests.

## Installation

Download the archive for your platform from the [GitHub Releases](https://github.com/coalaura/rawr/releases) page, extract it and place the `rawr` binary somewhere on your `PATH`.

Then start it:

```sh
rawr
```

rawr reads `config.yml` from the current directory. If the file does not exist, a default configuration is created automatically.

## Configuration

```yaml
server:
  port: 2112

sources:
  - host: src.example.com
    path: /<repo>/<path...>
    target: https://raw.githubusercontent.com/github/<repo>/refs/heads/main/<path>
    mime: text/plain

    allowed:
      path:
        - "*.md"
        - "readme.*"
        - "install.sh"

    cache:
      ttl: 5m
      stale_while_revalidate: 1m
      stale_if_error: 24h
      max_bytes: 64M
      max_body_bytes: 8M
```

Each source maps requests for a host and path to an upstream URL.

For example:

```text
http://src.example.com/project/docs/readme.md
```

is mapped to:

```text
https://raw.githubusercontent.com/github/project/refs/heads/main/docs/readme.md
```

### Path variables

Variables occupy a complete path segment:

```yaml
path: /<repo>/<file>
```

A variable ending in `...` captures the remaining path and must be the final segment:

```yaml
path: /<repo>/<path...>
```

Variables can be used in the source target:

```yaml
target: https://example.com/<repo>/<path>
```

### Document index

Set an optional source-level `index` to a filename such as `index.html`. Requests ending in `/` have that filename appended before path matching and `allowed` validation. The index must be a single valid path segment, excluding `.` and `..`.

For a source with `path: /<repo>/<path...>` and `index: index.html`:

| Request | Resolved `<path>` |
| --- | --- |
| `/project/` | `index.html` |
| `/project/docs/` | `docs/index.html` |
| `/project/docs` | `docs` |
| `/project/docs/guide.html` | `docs/guide.html` |

Only a trailing slash triggers index resolution; extensionless paths are not treated as directories. No redirect or upstream probing is performed. Directory requests and explicit requests for their index file share the same upstream cache entry. Omitting `index` preserves the existing path-matching behavior.

### MIME types

The source-level `mime` sets the default response `Content-Type`. Use `mime_extensions` to override it for individual file extensions. For example, a source serving web documents can include:

```yaml
index: index.html
mime: text/plain
mime_extensions:
  .html: text/html
  .css: text/css
  .js: text/javascript
```

Extensions must include a leading dot and are matched case-insensitively against the resolved document's final extension, including an inserted index filename. Case-insensitive duplicate keys are rejected. Compound extensions such as `.tar.gz` are not supported; use `.gz` instead.

The response uses the matching nonempty extension override, then `mime` if configured, then the upstream `Content-Type`. Omitting `mime_extensions` preserves the existing MIME behavior.

### Allowed values

The optional `allowed` section restricts values captured by a variable:

```yaml
allowed:
  path:
    - "*.md"
    - "docs/*"
    - "LICENSE"
```

Rules may be:

- an exact value, such as `LICENSE`
- a prefix pattern, such as `docs/*`
- a suffix pattern, such as `*.md`

Variables without an `allowed` entry accept any valid path value.

### Cache settings

| Setting | Description | Default |
| --- | --- | --- |
| `ttl` | Time before a cached response expires | `5m` |
| `stale_while_revalidate` | Time stale content may be served while refreshing | disabled |
| `stale_if_error` | Time stale content may be served after an upstream error | disabled |
| `max_bytes` | Maximum cache size | `64M` |
| `max_body_bytes` | Maximum size of one upstream response | `8M` |

Byte sizes accept `K`, `M`, `G` and `T` suffixes using binary units.

## Building from source

Clone the repository and run:

```sh
go build -o rawr .
```

## TLS

rawr serves plain HTTP. For public deployments, place it behind a reverse proxy that handles HTTPS.
