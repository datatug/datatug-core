# DataTug CLI - a command line data browser & editor

[![Build, Test, Vet, Lint](https://github.com/datatug/datatug/actions/workflows/golangci.yml/badge.svg)](https://github.com/datatug/datatug/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/datatug/datatug)](https://goreportcard.com/report/github.com/datatug/datatug)
[![Version](https://img.shields.io/github/v/tag/datatug/datatug?filter=v*.*.*&logo=Go)](https://github.com/datatug/datatug/tags)
[![GoDoc](https://godoc.org/github.com/datatug/datatug?status.svg)](https://godoc.org/github.com/datatug/datatug)

## What it is and why?

This is an agent service for https://datatug.app that you can run on your local machine, or some server to allow DataTug
app to scan databases & execute SQL requests.

It can be run with your user account credentials (*e.g. trusted connection*) or under some service account.

## Family of DataTug apps

The `datatug` app has a lot of modules. Some ot that modules can be run as standalone CLI apps:

- [`fsv`](apps/firestoreviewer) - A Firestore Viewer, similar to running `datatug firestore`

## Would you steal my data?

No, we won't.

The project is **free and open source** codes available at https://github.com/datatug/datatug. You are welcome to
check - we do not look into your data.

You can easily get executable of the agent from source codes using next command:

```
go install github.com/datatug/datatug
```

Note: _[Go language](https://golang.org/) should be [pre-installed](https://golang.org/dl/)_

## Where are metadata stored?

When DataTug agent scans or compare your database it stores meta information in a datatug project as set of simple to
understand & easy to compare JSON files.

We recommend to check-in the project to some source versioning control system like GIT.

You can run commands for different projects by passing path to DataTugProject folder. E.g.:

```
> datatug show --project ~/my-datatug-projects/DemoProject
```

Paths to the DataTug project files, and their names are stored in `~/datatug.yaml` in the root of your user's home
directory.
This allows you to address a DataTug project in a console using a short alias. Like this:

```
> datatug show -p DemoProject
```

If the current directory is a DataTug project folder you don't need to specify project name or path.

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

At the moment we any DB supported by [DALgo](https://github.com/dal-go/dalgo). Like:

- [dalgo2firestore](https://github.com/dal-go/dalgo2firestore)
- [dalgo2sql](https://github.com/dal-go/dalgo2sql)

### Supported `sql` Databases:

Datatug can work with `sql` DBs if a relevant driver has been linked into `datatug`

- **SQLite** - via  [github.com/mattn/go-sqlite3](https://github.com/mattn/go-sqlite3 )
- **Microsoft SQL Server** - via [go-mssqldb](https://github.com/denisenkom/go-mssqldb)

We are open for pull requests to support other `sql` DBs.

## Project structure

- [./main.go](main.go) - main entry point for `datatug` CLI
- [apps](apps) - contains mini-apps like Firestore Viewer
    - [datatug](apps/datatug) - defines `datatug` CLI commands & modules
    - [firestoreviewer](apps/firestoreviewer) - the `fsv` CLI utility for managing Firestore databases
- [packages](packages) - source codes
- [docs](docs) - documentation
- [.github/workflows](.github/workflows) - continuous integration

## Download

http://datatug.app/download

## Dependencies & Credits

- https://github.com/denisenkom/go-mssqldb - Go language driver to connect to MS SQL Server
- https://gihub.com/strongo/validation - helpers for requests & models validations

## Sample Databases

### By Database Platform

- SQLite
    - [Chinook Database](https://github.com/lerocha/chinook-database)
    - [Northwind](https://github.com/jpwhite3/northwind-SQLite3)
- MS SQL Server
    - [Northwind](https://github.com/Microsoft/sql-server-samples/tree/master/samples/databases/northwind-pubs)
- Oracle
    - [Northwind](https://github.com/dshifflet/NorthwindOracle_DDL)

### Northwind Database

- [SQLite](https://github.com/jpwhite3/northwind-SQLite3)
- [MS SQL Server](https://github.com/Microsoft/sql-server-samples/tree/master/samples/databases/northwind-pubs)
- [Oracle](https://github.com/dshifflet/NorthwindOracle_DDL)

## CI/CD

There is a [continuous integration build](docs/CI-CD.md).

## Open Source Libraries we use

- [DALgo](https://github.com/dal-go/dalgo) - Database Abstraction Layer for Go
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - A Go framework for TUI apps

## Contribution

Contributors wanted. For a start check [issues](https://github.com/datatug/datatug/issues)
tagged with [`help wanted`](https://github.com/datatug/datatug/labels/help%20wanted)
and [`good first issue`](https://github.com/datatug/datatug/labels/good%20first%20issue).

## Plans for improvements & TODO integrations

- https://github.com/rivo/tview - show tables & query text
- Dashboard: consider either https://github.com/gizak/termui or https://github.com/mum4k/termdash
- [Dasel](https://github.com/TomWright/dasel) - Select, put and delete data from JSON, TOML, YAML, XML and CSV files
  with a single tool. Supports conversion between formats and can be used as a Go package.
- [DbMate](https://github.com/amacneil/dbmate) - A lightweight, framework-agnostic database migration tool.
- use [SuperFile](https://github.com/yorukot/superfile) for file browsing?

## Licensing

The `datatug` and other DataTug CLIs are free to use and source codes are open source under [MIT license](./LICENSE).
