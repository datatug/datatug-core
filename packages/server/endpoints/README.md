# API end-points of DataTug agent

When DataTug agent started with `serve` command it listed on HTTP port
(*by default 8989*).

> datatug serve -p=./example

## Endpoints

| Method | Path | Description |
|---|---|---|
| POST   | /execute | Executes a batch of commands |
| GET | /select | Executes a single non mutating SELECT command |

### Endpoint: POST /execute
Executes a batch of commands

### Endpoint: GET /select
Executes a single non mutating SELECT command