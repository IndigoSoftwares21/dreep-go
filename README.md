# dreep-go

[![Go Reference](https://pkg.go.dev/badge.svg)](https://pkg.go.dev/github.com/IndigoSoftwares21/dreep-go)
[![Tests](https://github.com/IndigoSoftwares21/dreep-go/actions/workflows/test.yml/badge.svg)](https://github.com/IndigoSoftwares21/dreep-go/actions/workflows/test.yml)

The Go SDK for the [Dreep media API](https://docs.dreep.cloud) — upload, store,
process, and deliver images, videos, and documents.

**Zero dependencies.** Built entirely on the Go standard library (`net/http`,
`mime/multipart`, `crypto/hmac`), mirroring the philosophy of the official
Node SDK.

## Installation

```
go get github.com/IndigoSoftwares21/dreep-go
```

Requires Go 1.22 or later.

## Authentication

Create an API key on the **API Keys** page of your Dreep dashboard. The key is
a full-project credential and must stay server-side.

```go
client, err := dreep.New(os.Getenv("DREEP_API_KEY"))
if err != nil { /* ... */ }

// Only needed for SignedURL / signed folders:
client, err := dreep.New(
    os.Getenv("DREEP_API_KEY"),
    dreep.WithSigningSecret(os.Getenv("DREEP_SIGNING_SECRET")),
)
```

## Quickstart

```go
f, _ := os.Open("hero.jpg")
defer f.Close()

asset, err := client.Upload(ctx, dreep.UploadOptions{
    File:        f,
    Filename:    "hero.jpg",
    Destination: dreep.Destination{Folder: "marketing/2026"},
    Transform:   &dreep.Transform{Width: 1200, Format: dreep.FormatWebP},
})
// asset.URL — https://cdn.dreep.cloud/api/v1/fetch/<id>.webp

// Build a smaller variant — no network call:
small := client.URL(asset.ID, &dreep.Transform{Width: 400})
```

## API surface

| Method | Endpoint |
|---|---|
| `Upload` | `POST /upload` |
| `PresignUpload` | `POST /upload/presign` |
| `ConfirmUpload` | `POST /upload/:id/confirm` |
| `UploadPresigned` | presign → PUT → confirm in one call |
| `ListMedia` | `GET /media` |
| `ListAllMedia` | `GET /media`, every page |
| `DeleteMedia` | `DELETE /media/:id` |
| `URL` | builds a transform URL (no network call) |
| `SignedURL` | time-limited HMAC-SHA256 link |
| `CreateFolder` / `ListFolders` | `POST` / `GET /folders` |
| `CreatePreset` / `ListPresets` / `DeletePreset` | `/presets` |
| `RemoveBackground` | `POST /bg-remove` |
| `ExtractText` / `UploadAndExtractText` | OCR via `fetch/{id}.txt` / `POST /ocr` |
| `GetUsage` | `GET /usage` |

Every method takes a `context.Context` first.

### Direct-to-storage uploads

For large files and browser uploads, bytes bypass the API:

```go
asset, err := client.UploadPresigned(ctx, dreep.DirectUploadOptions{
    PresignOptions: dreep.PresignOptions{
        ContentType: "video/mp4",
        SizeBytes:   size,
        Filename:    "clip.mp4",
        ContentHash: sha256Hex, // optional dedupe
    },
    File: f,
})
```

Already-existing content (`alreadyExists`) skips the PUT automatically, and
409 conflicts during confirm are retried while the bytes settle.

### Signed URLs

```go
signed, err := client.SignedURL("med_123", time.Hour, nil)
// https://cdn.dreep.cloud/api/v1/fetch/med_123?exp=1774118400&sig=a3f9e2b1…
```

The signature is an HMAC-SHA256 hex digest of `<assetID>:<expires>` keyed with
your project's URL Signing Secret; transforms stay free to vary at delivery.

## Errors

Every non-2xx response decodes into `*dreep.Error` with helpers:

```go
asset, err := client.RemoveBackground(ctx, r, "shoe.png", nil)
switch {
case dreep.IsPaymentRequired(err): // billing limit / no bg-removal credits
case dreep.IsNotFound(err):
case dreep.IsConflict(err):        // presigned upload not landed yet
case dreep.IsInvalidRequest(err):
}
```

Fields that the API serialises as strings for BIGINT safety (e.g.
`sizeBytes`, `storageBytes`) decode transparently via `.Int64()`.

## Development

```
go test ./... -count=1   # httptest-backed unit tests, no network needed
go vet ./...
```

## License

MIT — see [LICENSE](LICENSE).
