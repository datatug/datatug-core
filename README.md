# DataTug CLI - a command line data browser & editor

[![Build, Test, Vet, Lint](https://github.com/datatug/datatug/actions/workflows/golangci.yml/badge.svg)](https://github.com/datatug/datatug/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/datatug/datatug)](https://goreportcard.com/report/github.com/datatug/datatug)
[![Version](https://img.shields.io/github/v/tag/datatug/datatug?filter=v*.*.*&logo=Go)](https://github.com/datatug/datatug/tags)
[![GoDoc](https://godoc.org/github.com/datatug/datatug?status.svg)](https://godoc.org/github.com/datatug/datatug)


## Project structure

- [pkg](pkg) - source codes for core packages
- [docs](docs) - documentation
- [.github/workflows](.github/workflows) - continuous integration

## Dependencies & Credits

- https://gihub.com/strongo/validation - helpers for requests & models validations


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
