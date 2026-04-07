# Reading List

A minimal web app for saving and organizing URLs to read later, built with Go and zero external dependencies.

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)

## Features

- **Save URLs** with automatic title extraction from the page's `<title>` tag
- **Archive/unarchive** items to keep your list tidy
- **Delete** items you no longer need
- **Toggle archived view** to show or hide completed reads
- **Persistent storage** via a local `urls.json` file
- Dark theme UI with Bodoni Moda serif typography

## Getting Started

### Prerequisites

- [Go 1.22+](https://go.dev/dl/)

### Run

```bash
go run main.go
```

Then open [http://localhost:8080](http://localhost:8080).

### Build

```bash
go build -o reading-list main.go
./reading-list
```

## How It Works

The entire app lives in a single `main.go` file — HTTP handlers, HTML template, CSS, and JavaScript included. URLs are stored in `urls.json` in the working directory, created automatically on first use.

### Endpoints

| Route | Method | Description |
|-------|--------|-------------|
| `/` | GET | Display the reading list |
| `/add` | POST | Add a new URL |
| `/archive` | POST | Toggle archive status |
| `/delete` | POST | Remove a URL |

## TODO
- [x] don't focus on input field by default
- [x] do the same "..." concatenation for non-mobile viewports as well

## License

MIT
