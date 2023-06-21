# DataTug agent

Open source codes in Go language under [MIT license](./LICENSE).

[![Test & Build](https://github.com/datatug/datatug/actions/workflows/golangci.yml/badge.svg)](https://github.com/datatug/datatug/actions/workflows/golangci.yml)

## What it is and why?

This is an agent service for https://datatug.app that you can run on your local machine, or some server to allow DataTug
app to scan databases & execute SQL requests.

It can be run with your user account credentials (*e.g. trusted connection*) or under some service account.

## Would you steal my data?

No, we won't.

The project is **free and open source** codes available at https://github.com/datatug/datatug. You are welcome to check - we do not look into your data.

You can easily get executable of the agent from source codes using next command:
```
go install github.com/datatug/datatug
```
Note: _[Go language](https://golang.org/) should be [pre-installed](https://golang.org/dl/)_

## Where metadata are stored?

When DataTug agent scans or compare your database it stores meta information in a datatug project as set of simple to
understand & easy to compare JSON files.

We recommend to check-in the project to some source versioning control system like GIT.

You can run commands for different projects by passing path to DataTugProject folder. E.g.:
```
> datatug show --project ~/my-datatug-projects/DemoProject
```

Paths to the DataTug project files, and their names are stored in `~/datatug.yaml` in the root of your user's home directory.
This allows you to address a DataTug project in a console using a short alias. Like this:

```
> datatug show --project DemoProject
```

If current directory is a DataTug project folder you don't need to specify project name or path.

```
> datatug show
```

## How to get DataTug agent CLI?

Get from source codes by running:
```
> go install github.com/datatug/datatug
```
If it passes you are good to go:
```
> datatug --help
```

## How to run?

Check the [CLI](./packages/cli) section on how to run DataTug agent.

## Supported databases

- **Microsoft SQL Server** - *via [https://github.com/denisenkom/go-mssqldb](https://github.com/denisenkom/go-mssqldb)*

At the moment we have connectors only to MS SQL Server.

We are open for pull requests to support other DBs.

## Project structure

- [./datatug.go](datatug.go) - entry point
- [packages](packages) - source codes
- [docs](docs) - documentation
- [.github/workflows](.github/workflows) - continuous integration

## Download

http://datatug.app/download

## Dependencies & Credits

- https://github.com/denisenkom/go-mssqldb - Go language driver to connect to MS SQL Server
- https://gihub.com/strongo/validation - helpers for requests & models validations

## Plans for improvements

- https://github.com/rivo/tview - show tables & query text
- Dashboard: consider either https://github.com/gizak/termui or https://github.com/mum4k/termdash

# CI/CD

There is a [continuous integration build](docs/CI-CD.md).

## Contribution

Contributors wanted. For a start check [issues](https://github.com/datatug/datatug/issues)
tagged with [`help wanted`](https://github.com/datatug/datatug/labels/help%20wanted)
and [`good first issue`](https://github.com/datatug/datatug/labels/good%20first%20issue).

## TODO integrations
- [Dasel](https://github.com/TomWright/dasel) - Select, put and delete data from JSON, TOML, YAML, XML and CSV files with a single tool. Supports conversion between formats and can be used as a Go package.

## Licensing

Free & open source under [MIT license](./LICENSE).
