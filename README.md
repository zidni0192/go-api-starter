# Go API Starter

A small REST API for managing items, built with Go's standard library.

## Run locally

Requires Go 1.27 or newer.

```sh
go run .
```

The server listens on `:8080`. Data is kept in memory and resets when the process stops.

## API

| Method | Path | Behavior |
| --- | --- | --- |
| `GET` | `/items` | List items |
| `POST` | `/items` | Create an item with `{"name":"Example"}` |
| `GET` | `/items/{id}` | Get an item |
| `PUT` | `/items/{id}` | Replace its name |
| `DELETE` | `/items/{id}` | Delete an item |

Names must not be empty. Unknown IDs return `404`; malformed IDs and invalid input return `400`.

## Architecture

The HTTP handlers use a concurrency-safe in-memory store. Keeping persistence out of this first vertical slice makes the API runnable without external services; a later issue can add a database and migrations.

## Verify

```sh
go test -race ./...
go vet ./...
```
