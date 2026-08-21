# arushiahmed-site-api

Backend API for my personal website. It serves photo and document metadata backed
by two S3 buckets, returning CloudFront CDN URLs rather than serving files
directly. The buckets stay private (no public access); a CloudFront
distribution in front of each one, using Origin Access Control, is what's
allowed to read from them.

The same binary runs two ways:

- **Locally / anywhere else**: a plain `net/http` server on `:8080`.
- **In AWS Lambda**: detected via the `AWS_LAMBDA_FUNCTION_NAME` env var,
  using `httpadapter` to run the exact same handlers behind an ALB.

## Project layout

- `main.go` — sets up routing, CORS, and the Lambda/local dispatch.
- `store/` — shared S3 listing + CDN URL logic used by both services.
- `photos/` — photo endpoints (`PhotoService`), backed by the photos bucket.
- `documents/` — document endpoints (`DocumentService`), backed by the
  documents bucket.

## Endpoints

| Method | Path                    | Description                                          |
|--------|-------------------------|-------------------------------------------------------|
| GET    | `/health`               | Liveness check, returns `{"status":"ok"}`             |
| GET    | `/photos`                | List all photos (optional `?prefix=`)                 |
| GET    | `/photos/city/{city}`    | List photos whose key contains `city` (case-insensitive) |
| GET    | `/photos/{key...}`       | Redirect (302) to the CDN URL for that photo           |
| GET    | `/documents`              | List all documents (optional `?prefix=`)               |
| GET    | `/documents/{key...}`     | Redirect (302) to the CDN URL for that document         |

CDN URLs are unsigned and stable — no expiry — so they're cacheable at the
edge. Keys aren't guessable/enumerable outside of these list endpoints, but
anyone who has a URL can access it indefinitely.

## Prerequisites

- Go 1.26+ (see `go.mod`)
- AWS credentials with read access to the S3 buckets, available via any
  standard method the AWS SDK picks up (`~/.aws/credentials`,
  `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`, SSO, etc.) — the app uses
  `config.LoadDefaultConfig`, so whatever `aws sts get-caller-identity`
  resolves to locally is what it will use.

## Running locally

```bash
go run .
```

This starts the server on `http://localhost:8080`. Useful env vars:

| Variable              | Default                  | Purpose                                        |
|-----------------------|---------------------------|-------------------------------------------------|
| `PHOTOS_BUCKET`        | `arushiahmed-photos`      | S3 bucket for photos                            |
| `DOCUMENTS_BUCKET`     | `arushiahmed-documents`   | S3 bucket for documents                         |
| `PHOTOS_CDN_DOMAIN`    | *(none — required)*       | CloudFront domain fronting the photos bucket     |
| `DOCUMENTS_CDN_DOMAIN` | *(none — required)*       | CloudFront domain fronting the documents bucket  |
| `ALLOWED_ORIGIN`       | `http://localhost:3000`   | Value sent in `Access-Control-Allow-Origin`     |

Example:

```bash
PHOTOS_BUCKET=my-test-bucket PHOTOS_CDN_DOMAIN=d123abc.cloudfront.net ALLOWED_ORIGIN=http://localhost:3000 go run .
```

Then exercise it with curl:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/photos
curl http://localhost:8080/photos/city/paris
curl -i http://localhost:8080/photos/some/key.jpg   # expect a 302 redirect to the CDN URL
curl http://localhost:8080/documents
```

Note: `/photos`, `/documents`, and their sub-routes require valid AWS
credentials with access to the target buckets — without them you'll get a
`502` with a `failed to list ...` body. `/health` works with no AWS access
at all.

## Running the tests

```bash
go test ./...
```

`store`'s tests mock the S3 listing client, so they run fully offline with
no AWS credentials required.

## Building

```bash
go build ./...
```

For a Lambda deployment build, cross-compile for Linux and match whatever
architecture the function is configured with (`arm64` or `amd64`):

```bash
GOOS=linux GOARCH=arm64 go build -o bootstrap .
```
