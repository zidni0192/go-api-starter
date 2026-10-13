# Go API Starter

A small REST API for managing items, built with Go and a local SQLite database.

## Run locally

Requires Go 1.27 or newer.

```sh
go run .
```

The server listens on `:8080`. SQLite is embedded; no Docker or external database is needed. The database file defaults to `data.db` in the current directory. Set `DATABASE_PATH` to choose another file:

```sh
DATABASE_PATH=./data/items.db go run .
```

The application creates the parent directory and applies versioned migrations at startup. Database files and SQLite sidecar files are excluded from Git.

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

The HTTP handlers use `database/sql` with the pure-Go `modernc.org/sqlite` driver. A schema migration table records applied versions, and the initial migration creates the items table without replacing existing data.

## Verify

```sh
go test -race ./...
go vet ./...
```
