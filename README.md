# cdk-go

Go FFI bindings for [cashubtc/cdk](https://github.com/cashubtc/cdk) (Cashu Development Kit), generated via [uniffi-bindgen-go](https://github.com/NordSecurity/uniffi-bindgen-go).

Prebuilt native libraries are committed to this repo — downstream consumers only need Go.

## Usage

```bash
go get github.com/cashubtc/cdk-go
```

CGO link flags are automatically selected per platform. No manual setup required.

### Supported platforms

| OS      | Arch  | Library                |
|---------|-------|------------------------|
| Linux   | amd64 | `libcdk_ffi.so`        |
| macOS   | arm64 | `libcdk_ffi.dylib`     |
| macOS   | amd64 | `libcdk_ffi.dylib`     |

## Development

### Prerequisites

- Go 1.22+
- Rust toolchain (stable)
- `CGO_ENABLED=1`

### Regenerating bindings

```bash
make generate   # clone CDK, build libcdk_ffi, generate Go bindings
make verify     # CGO_ENABLED=1 go test ./bindings/cdkffi
make clean      # rm -rf .work bindings/cdkffi
```

### Environment variables

| Variable               | Default                              | Description                          |
|------------------------|--------------------------------------|--------------------------------------|
| `CDK_REF`              | `v0.16.0`                            | CDK branch, tag, or SHA to build     |
| `CDK_REPO`             | `https://github.com/cashubtc/cdk.git`| CDK source repository                |
| `UNIFFI_BINDGEN_GO_TAG`| `v0.5.0+v0.29.5`                     | Pinned `uniffi-bindgen-go` version   |
| `BUILD_PROFILE`        | `release`                            | Rust build profile                   |

## CI/CD

| Workflow                  | Trigger              | Description                                               |
|---------------------------|----------------------|-----------------------------------------------------------|
| `ci.yml`                  | Push / PR            | Regenerates bindings; fails if committed source has drifted |
| `update-bindings-pr.yml`  | Scheduled / manual   | Rebuilds cross-platform and opens a PR with updated files |
| `release.yml`             | Dispatch / manual    | Rebuilds from a CDK ref, tags the repo, and publishes a GitHub release |
