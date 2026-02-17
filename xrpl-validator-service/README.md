# XRPL Validator Service

A Go service that fetches verified validators from the XRP Ledger mainnet and streams live transactions via WebSocket for Globe.GL visualization.

## Features

- ✅ Fetches validator data from rippled server
- ✅ Periodic validator caching with configurable refresh intervals
- ✅ Real-time transaction streaming via WebSocket
- ✅ Geolocation enrichment support (stub provided)
- ✅ REST API for validator queries
- ✅ Graceful error handling and reconnection logic
- ✅ Docker support for containerized deployment

## Prerequisites

- Go 1.21+
- Running rippled server (Docker container recommended, required only for `local`/`hybrid` validator path)
- Docker (optional, for containerized deployment)

## Installation

### Local Development

1. Clone and navigate to the service directory:
```bash
cd xrpl-validator-service
```

2. Copy environment configuration:
```bash
cp .env.example .env
```

3. Build the service:
```bash
go build ./cmd/validator-service
```

4. Run the service:
```bash
./validator-service
```

### Docker Deployment

1. Build the Docker image:
```bash
cd xrpl-validator-service
docker build -t xrpl-validator-service .
```

2. Add to your `docker-compose.yml`:
```yaml
services:
  go-validator-service:
    build:
      context: ./xrpl-validator-service
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    depends_on:
      - rippled
    environment:
      XRPL_SOURCE_MODE: hybrid
      RIPPLED_JSON_RPC_URL: http://rippled:5005
      RIPPLED_WEBSOCKET_URL: ws://rippled:6006
      PUBLIC_RIPPLED_JSON_RPC_URL: https://xrplcluster.com
      PUBLIC_RIPPLED_WEBSOCKET_URL: wss://xrplcluster.com
      XRPL_NETWORK: mainnet
      LISTEN_PORT: 8080
      LISTEN_ADDR: 0.0.0.0
      VALIDATOR_REFRESH_INTERVAL: 300
      VALIDATOR_LIST_SITES: https://vl.ripple.com,https://unl.xrplf.org
      SECONDARY_VALIDATOR_REGISTRY_URL: https://api.xrpscan.com/api/v1/validatorregistry
      VALIDATOR_METADATA_CACHE_PATH: data/validator-metadata-cache.json
      GEO_CACHE_PATH: data/geolocation-cache.json
      GEO_LOOKUP_MIN_INTERVAL_MS: 1200
      GEO_RATE_LIMIT_COOLDOWN_SECONDS: 900
      MIN_PAYMENT_DROPS: 1000000000
      LOG_LEVEL: info
    container_name: xrpl-validator-service
    restart: unless-stopped
```

3. Run with docker-compose:
```bash
docker-compose up
```

## Configuration

Configure via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `XRPL_SOURCE_MODE` | `hybrid` | Source selection: `local`, `public`, or `hybrid` |
| `RIPPLED_JSON_RPC_URL` | `http://localhost:5005` | rippled JSON-RPC endpoint URL |
| `RIPPLED_WEBSOCKET_URL` | `ws://localhost:6006` | rippled WebSocket endpoint URL |
| `PUBLIC_RIPPLED_JSON_RPC_URL` | `https://xrplcluster.com` | Public JSON-RPC endpoint used in `public`/`hybrid` modes |
| `PUBLIC_RIPPLED_WEBSOCKET_URL` | `wss://xrplcluster.com` | Public WebSocket endpoint used in `public`/`hybrid` modes |
| `XRPL_NETWORK` | `mainnet` | Network label returned with validator data |
| `LISTEN_ADDR` | `0.0.0.0` | HTTP server listen address |
| `LISTEN_PORT` | `8080` | HTTP server listen port |
| `VALIDATOR_REFRESH_INTERVAL` | `300` | Validator refresh interval in seconds |
| `VALIDATOR_LIST_SITES` | `https://vl.ripple.com,https://unl.xrplf.org` | Comma-separated validator list source URLs |
| `SECONDARY_VALIDATOR_REGISTRY_URL` | `https://api.xrpscan.com/api/v1/validatorregistry` | Secondary validator metadata source for domain enrichment |
| `VALIDATOR_METADATA_CACHE_PATH` | `data/validator-metadata-cache.json` | Persistent validator metadata cache keyed by validator key/address |
| `GEO_CACHE_PATH` | `data/geolocation-cache.json` | Persistent geolocation cache path (survives process restarts) |
| `GEO_LOOKUP_MIN_INTERVAL_MS` | `1200` | Minimum delay between outbound geolocation lookups |
| `GEO_RATE_LIMIT_COOLDOWN_SECONDS` | `900` | Cooldown window after a geolocation provider `429` |
| `MIN_PAYMENT_DROPS` | `1000000000` | Minimum streamed payment amount in drops (1,000 XRP) |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |

### Source Modes

- `local`: validators + transactions use local rippled only.
- `public`: validators + transactions use public endpoints only.
- `hybrid` (default): transactions use public endpoint immediately; validators/health prefer local rippled when reachable, otherwise fallback to public.

## API Endpoints

### Health Check

**GET /health**

Returns service health status and connection information.

```bash
curl http://localhost:8080/health
```

Response:
```json
{
  "status": "ok",
  "validators_count": 15,
  "last_validator_update": "2025-02-15T03:30:00Z",
  "transaction_listener_active": true,
  "websocket_clients": 2
}
```

### Get Validators

**GET /validators**

Returns the current list of cached validators with geolocation data.

```bash
curl http://localhost:8080/validators
```

Response:
```json
{
  "validators": [
    {
      "address": "nHBCQviecrnyiZUgkTELcNyKWdKG92jHXo",
      "public_key": "030E58B6B2B5C...",
      "domain": "example.com",
      "name": "Example Validator",
      "network": "mainnet",
      "latitude": 40.7128,
      "longitude": -74.0060,
      "country_code": "US",
      "city": "New York",
      "last_updated": 1708011000,
      "is_active": true
    }
  ],
  "count": 1,
  "timestamp": "2025-02-15T03:30:00Z"
}
```

### Transaction Stream (WebSocket)

**GET /transactions** (WebSocket upgrade)

Establishes a WebSocket connection for streaming validated XRP `Payment` transactions where amount is at least `MIN_PAYMENT_DROPS` (default `1,000 XRP`).

```javascript
// JavaScript example
const ws = new WebSocket('ws://localhost:8080/transactions');

ws.onmessage = (event) => {
  const transaction = JSON.parse(event.data);
  console.log('New transaction:', transaction);
  // {
  //   "hash": "...",
  //   "account": "rN7n7otQDd6FczFgLdlqXRrrfVPqjnKvVQ",
  //   "destination": "rLHzPsX6oXkzU9cRHEwKmMSWJfpJ9nE4VY",
  //   "amount": "25000000000",
  //   "transaction_type": "Payment",
  //   "source_info": { "latitude": 40.7128, "longitude": -74.0060, ... },
  //   "dest_info": { "latitude": 51.5074, "longitude": -0.1278, ... },
  //   ...
  // }
};

ws.onerror = (error) => console.error('WebSocket error:', error);
ws.onclose = () => console.log('WebSocket closed');
```

## Architecture

```
┌─────────────────────────────────────────────┐
│ XRPL Validator Service                      │
├─────────────────────────────────────────────┤
│ ├─ Config Management                        │
│ ├─ rippled Client (JSON-RPC + WebSocket)    │
│ ├─ Validator Fetcher                        │
│ │  ├─ Periodic fetching                     │
│ │  ├─ In-memory caching                     │
│ │  └─ Geolocation enrichment                │
│ ├─ Transaction Listener                     │
│ │  ├─ WebSocket subscription                │
│ │  ├─ Message processing                    │
│ │  └─ Callback system                       │
│ └─ HTTP Server                              │
│    ├─ REST API (/validators, /health)       │
│    └─ WebSocket endpoint (/transactions)    │
└─────────────────────────────────────────────┘
         │                    │
         ▼                    ▼
    rippled Server      Frontend (Globe.GL)
```

## Project Structure

```
xrpl-validator-service/
├── cmd/
│   └── validator-service/
│       └── main.go           # Service entry point
├── internal/
│   ├── config/
│   │   └── config.go         # Configuration management
│   ├── models/
│   │   └── models.go         # Data models
│   ├── rippled/
│   │   └── client.go         # rippled client
│   ├── validator/
│   │   ├── fetcher.go        # Validator fetching logic
│   │   └── geolocation.go    # Geolocation provider
│   ├── transaction/
│   │   └── listener.go       # Transaction listener
│   └── server/
│       └── server.go         # HTTP server & WebSocket
├── tests/                    # Unit tests (to be added)
├── Dockerfile               # Docker image definition
├── go.mod                   # Go module definition
├── go.sum                   # Dependency checksums
├── .env.example             # Environment variables template
└── README.md                # This file
```

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build ./cmd/validator-service
```

### Running with Custom Config

```bash
RIPPLED_JSON_RPC_URL=http://localhost:5005 \
RIPPLED_WEBSOCKET_URL=ws://localhost:6006 \
PUBLIC_RIPPLED_JSON_RPC_URL=https://xrplcluster.com \
PUBLIC_RIPPLED_WEBSOCKET_URL=wss://xrplcluster.com \
XRPL_SOURCE_MODE=hybrid \
XRPL_NETWORK=mainnet \
VALIDATOR_LIST_SITES=https://vl.ripple.com,https://unl.xrplf.org \
SECONDARY_VALIDATOR_REGISTRY_URL=https://api.xrpscan.com/api/v1/validatorregistry \
VALIDATOR_METADATA_CACHE_PATH=data/validator-metadata-cache.json \
GEO_CACHE_PATH=data/geolocation-cache.json \
GEO_LOOKUP_MIN_INTERVAL_MS=1200 \
GEO_RATE_LIMIT_COOLDOWN_SECONDS=900 \
MIN_PAYMENT_DROPS=1000000000 \
LISTEN_PORT=9000 \
LOG_LEVEL=debug \
./validator-service
```

## Next Phases

- [ ] Phase 8: Enhanced error handling and resilience (reconnection logic, circuit breakers)
- [ ] Phase 10: Comprehensive unit tests
- [ ] Phase 11: Frontend integration with Globe.GL
- [ ] Real geolocation provider (GeoIP database integration)
- [ ] Validator transaction filtering and analytics
- [ ] Metrics and monitoring (Prometheus integration)
- [ ] Performance optimization

## Troubleshooting

### Connection refused to rippled

- Ensure rippled is running on the configured host/port
- Check `RIPPLED_JSON_RPC_URL` and `RIPPLED_WEBSOCKET_URL` environment variables
- Verify network connectivity between service and rippled
- If local rippled is still syncing, use `XRPL_SOURCE_MODE=hybrid` or `XRPL_SOURCE_MODE=public` to keep transaction flow live

### No validators returned

- Ensure rippled has fully synced
- Check rippled logs for validation errors
- Verify the service has fetched data (check logs)
- Verify `VALIDATOR_LIST_SITES` and `SECONDARY_VALIDATOR_REGISTRY_URL` are reachable from the service
- In `hybrid` mode, validator fetching will fallback to public if local server info is unavailable

### WebSocket clients not receiving transactions

- Confirm transaction listener is subscribed (check health endpoint)
- Verify rippled transaction stream is active
- Check firewall/network policies for WebSocket connections

### Validators have no mapped coordinates

- Check service logs for `geolocation API returned status 429` (provider rate limit)
- Keep `GEO_CACHE_PATH` on persistent storage so previously mapped validators are reused after restart
- Increase `GEO_LOOKUP_MIN_INTERVAL_MS` and/or `GEO_RATE_LIMIT_COOLDOWN_SECONDS` to reduce repeated rate-limit hits

## License

MIT

## Contributing

Contributions welcome! Please fork and submit PRs.
