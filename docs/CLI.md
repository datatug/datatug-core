# DataTug CLI tool

DataTug can be used as a CLI tool.

## Commands

- `datatug projects` - list DataTug projects
- `datatug init` - initialize DataTug project
- `datatug serve` - run DataTug server
- `datatug config` - manage DataTug configuration

### `config` command

Without parameters will display current configuration.

Can be used to set configuration values.

Subcommands:

- `server` - manage DataTug server configuration. Example:
   ```shell
    datatug config server --host localhost --port 8989
   ```
    - `--port` - sets port number
    - `--host` - sets host name
- `client` - manage DataTug client configuration