# DataTug Go packages

- [**cli**](./cli) - commands & flags to execute from command line interface
- [**server**](./server) - serving DataTug agent API over HTTP(s)
    - [**endpoints**](./server/endpoints) - HTTP handlers
    - [**api**](./api) - API non-transport specific implementation
    - [**dto**](dto) - DTO definitions for requests & responses
- [**execute**](./execute) - worker implementation that executes commands from CLI & API

