# API end-points of DataTug agent

When DataTug agent is started with a `serve` command it listens on HTTP port (*by default 8989*).

> datatug serve -p=./example

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
|  **Executor** |
| POST   | /exec/execute | Executes a batch of commands |
| GET | /exec/select | Executes a single non mutating SELECT command |
|  **Entities** |
| GET | /entities/all_entities | |
| GET | /entities/entity | |
| POST | /entities/create_entity | |
| PUT | /entities/save_entity | |
| DELETE | /entities/delete_entity | |
|  **Recordsets** |
| GET | /data/recordsets | |
| GET | /data/recordset_definition | |
| GET | /data/recordset_data | |
| POST | /data/recordset_add_rows | |
| PUT | /data/recordset_update_rows | |
| DELETE | /data/recordset_delete_rows | |
|  **Boards** |
| GET | /boards/board | |
| POST | /boards/create_board | |
| PUT | /boards/save_board | |
| DELETE | /boards/delete_board | |

### Endpoint: POST /execute

Executes a batch of commands

### Endpoint: GET /select

Executes a single non mutating SELECT command
