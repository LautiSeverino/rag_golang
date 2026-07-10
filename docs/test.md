# Sample Project

A small example project used to test document retrieval in a RAG pipeline.

## Overview

This repository contains a simple command-line tool that helps you search and filter text quickly. It is designed as a compact example with clear sections, short paragraphs, and a few code blocks so it is easy to chunk and retrieve.

## Features

- Fast text filtering
- Simple configuration
- Works from the terminal
- Easy to extend

## Requirements

- Go 1.22 or newer
- Git
- A terminal on Linux, macOS, or Windows

## Installation

```bash
git clone https://example.com/sample-project.git
cd sample-project
go build ./...
```

## Usage

Run the tool with a query:

```bash
./sample-project search "error"
```

Use the help command to see the available flags:

```bash
./sample-project --help
```

## Configuration

The tool reads settings from a `config.yml` file.

Example:

```yaml
server:
  port: 8080

search:
  max_results: 10
  timeout_ms: 2000
```

## Examples

Search for logs that contain the word `timeout`:

```bash
./sample-project search "timeout"
```

Show only the first 5 matches:

```bash
./sample-project search "timeout" --limit 5
```

## Troubleshooting

### The command is not found

Make sure the binary was built successfully and that you are running it from the correct directory.

### No results are returned

Check that the query matches the text in the input source and that the configuration file is valid.

## Contributing

Pull requests and issues are welcome. Keep changes small and focused so the document stays easy to read and test.

## License

This project is provided for testing and demonstration purposes.
