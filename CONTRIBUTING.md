# Contributing

Thanks for helping improve this project! It's a dependency-free Go port of
Python's FastMCP, so contributions that improve protocol fidelity, tests, or
docs are especially welcome.

## Getting started
- Requires **Go 1.24+**.
- `go test ./...` — run the tests.
- `go test -race -covermode=atomic -coverprofile=coverage.out ./...` — race + coverage.
- `golangci-lint run` — lint (config in `.golangci.yml`).
- `gofmt -w .` — format.

## Pull requests
1. Branch from `main` and keep changes focused.
2. Add tests for any new behavior; keep them deterministic.
3. Make sure `gofmt -l .` is empty, and `go vet ./...`, tests, and lint all pass —
   CI enforces all of these on Go 1.24.
4. Keep the module **dependency-free**: the standard library only. A new
   third-party import needs a very good reason.
5. Preserve the **FastMCP-mirroring API** where it makes sense (names and
   ergonomics are chosen to echo the original Python library on purpose).

## Reporting issues
Open an issue with a minimal reproduction and the Go version you're using.

By contributing, you agree that your contributions are licensed under the MIT License.
