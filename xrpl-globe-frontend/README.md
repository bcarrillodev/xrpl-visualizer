# XRPL Globe Frontend

React + Globe.GL frontend for visualizing XRPL activity.

## Prerequisites

- Node.js 18+
- Backend service running at `http://localhost:8080`

## Run

```bash
npm install
npm start
```

Open `http://localhost:3000`.

## Backend Endpoints Used

- `http://localhost:8080/validators`
- `http://localhost:8080/network-health`
- `ws://localhost:8080/transactions`

## Scripts

- `npm start`: start dev server
- `npm test`: run tests
- `npm run build`: production build
