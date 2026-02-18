# CS2Go Demo Analyzer (Golang + Angular)

Simple app to upload CS2 `.dem` files, process them in the Go backend, and view results in an Angular frontend.

## Tech Stack

- Go backend (`chi`, `websocket`, `demoinfocs-golang`)
- Angular frontend
- In-memory job store + worker pipeline

## Project Structure

- `main.go`: app entrypoint
- `internal/server`: HTTP API + WebSocket + static file serving
- `internal/parser`: demo parsing pipeline
- `internal/storage`: in-memory job state
- `frontend`: Angular app
- `uploads`: uploaded demo files

## Requirements

- Go `1.25+`
- Node.js `20+` and npm

## Run Locally

1. Install frontend deps:

```powershell
cd frontend
npm install
cd ..
```

2. Run Go server from repo root:

```powershell
go run .
```

Default server address is `:8080`.

You can change it with:

```powershell
$env:ADDR=":9090"
go run .
```

## Frontend Dev Mode

Run Angular dev server separately:

```powershell
cd frontend
npm start
```

## Build Frontend for Go Server

```powershell
cd frontend
npm run build
```

The backend serves static files from:

- `frontend/dist/cs2go/browser` (preferred)
- `frontend/dist/cs2go` (fallback)
- `frontend` (last fallback)

## API Endpoints

- `POST /upload` (multipart form field: `demo`)
- `GET /jobs`
- `GET /jobs/{jobID}`
- `GET /jobs/{jobID}/result`
- `GET /ws?job_id={jobID}` (WebSocket progress stream)

Example upload:

```powershell
curl -X POST http://localhost:8080/upload -F "demo=@match.dem"
```

![Screenshot 2026-02-18 153024.png](images/Screenshot%202026-02-18%20153024.png)
![Screenshot 2026-02-18 153035.png](images/Screenshot%202026-02-18%20153035.png)
![Screenshot 2026-02-18 153044.png](images/Screenshot%202026-02-18%20153044.png)
