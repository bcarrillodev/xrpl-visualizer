# xrpl-visualizer

An XRPL visualization stack with:
- `xrpl-service` (Go backend for validators, health, and live transaction streaming)
- `xrpl-globe-frontend` (React + Globe.GL frontend)

![XRPL Visualizer Screenshot](images/xrpl-visualizer-screenshot.avif)

## Local Run

1. Start backend:
```bash
cd xrpl-service
cp .env.example .env
go run ./cmd/validator-service
```

2. Start frontend (new terminal):
```bash
cd xrpl-globe-frontend
npm start
```

## Local URLs

- Frontend: `http://localhost:3000`
- Backend health: `http://localhost:8080/health`
- Backend validators: `http://localhost:8080/validators`
- Backend transaction websocket: `ws://localhost:8080/transactions`

## Docker (Backend)

```bash
cd xrpl-service
docker compose up --build
```
