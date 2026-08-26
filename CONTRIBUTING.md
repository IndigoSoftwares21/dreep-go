# Contributing to dreep-go

Thanks for your interest in improving the Dreep Go SDK!

## Getting started

```sh
git clone https://github.com/IndigoSoftwares21/dreep-go
cd dreep-go
go test ./... -count=1   # unit tests, no network or credentials needed
go vet ./...
```

## Ground rules

* **Zero dependencies.** The SDK uses only the Go standard library, matching
  the official Node SDK's philosophy. Do not add third-party imports.
* **Context-first APIs.** Every network method takes a `context.Context` as
  its first parameter.
* **Test everything.** New endpoints and behaviors need table-driven
  `httptest`-based unit tests. Live-API checks belong in `live_test.go`
  (behind the `live` build tag), never in the default test suite.
* **Match the wire format.** When in doubt about request/response shapes,
  verify against the real API rather than a single doc page — the Dreep docs
  have known inconsistencies (see the presets endpoint).
* **Format and vet cleanly.** `gofmt -l .` must print nothing; `go vet ./...`
  must pass.

## Commits

Write concise, descriptive commit messages focused on *why*, not just *what*
(e.g. "Fix KnownSize content length and unwrap API response envelope").

## Reporting bugs & security issues

* Bugs and feature requests: open a GitHub issue with reproduction steps.
* Security vulnerabilities: **never** via public issues — see
  [SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions will be licensed under the
MIT License that covers this project.
