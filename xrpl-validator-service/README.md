# XRPL Validator Service

A Go service that fetches verified validators from the XRP Ledger mainnet and streams live transactions via WebSocket for Globe.GL visualization.

## Features

- ✅ Fetches validator data from external XRPL endpoints
- ✅ Periodic validator caching with configurable refresh intervals
- ✅ Real-time transaction streaming via WebSocket
- ✅ GeoLite2 (MMDB) geolocation enrichment for validators and transaction accounts
- ✅ REST API for validator queries
- ✅ Graceful error handling and reconnection logic
- ✅ Docker support for containerized deployment

## Prerequisites

- Go 1.21+
- Internet access to external XRPL endpoints
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
    environment:
      PUBLIC_RIPPLED_JSON_RPC_URL: https://xrplcluster.com
      PUBLIC_RIPPLED_WEBSOCKET_URL: wss://xrplcluster.com
      TRANSACTION_JSON_RPC_URL: https://xrplcluster.com
      TRANSACTION_WEBSOCKET_URL: wss://xrplcluster.com
      XRPL_NETWORK: mainnet
      LISTEN_PORT: 8080
      LISTEN_ADDR: 0.0.0.0
      VALIDATOR_REFRESH_INTERVAL: 300
      VALIDATOR_LIST_SITES: https://vl.ripple.com,https://unl.xrplf.org
      SECONDARY_VALIDATOR_REGISTRY_URL: https://api.xrpscan.com/api/v1/validatorregistry
      VALIDATOR_METADATA_CACHE_PATH: data/validator-metadata-cache.json
      NETWORK_HEALTH_JSON_RPC_URLS: https://xrplcluster.com,https://s2.ripple.com:51234
      NETWORK_HEALTH_RETRIES: 2
      GEO_CACHE_PATH: data/geolocation-cache.json
      GEOLITE_DB_PATH: data/GeoLite2-City.mmdb
      GEOLITE_DOWNLOAD_URL: https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb
      GEOLITE_AUTO_DOWNLOAD: "true"
      MIN_PAYMENT_DROPS: 1000000
      TRANSACTION_BUFFER_SIZE: 2048
      GEO_ENRICHMENT_QUEUE_SIZE: 2048
      GEO_ENRICHMENT_WORKERS: 8
      MAX_GEO_CANDIDATES: 6
      BROADCAST_BUFFER_SIZE: 2048
      WS_CLIENT_BUFFER_SIZE: 512
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
| `PUBLIC_RIPPLED_JSON_RPC_URL` | `https://xrplcluster.com` | External JSON-RPC endpoint used for validator/health fetches |
| `PUBLIC_RIPPLED_WEBSOCKET_URL` | `wss://xrplcluster.com` | External WebSocket endpoint paired with validator source |
| `TRANSACTION_JSON_RPC_URL` | `https://xrplcluster.com` | External JSON-RPC endpoint used for transaction account/domain lookups |
| `TRANSACTION_WEBSOCKET_URL` | `wss://xrplcluster.com` | External WebSocket endpoint used for live transaction stream subscription |
| `XRPL_NETWORK` | `mainnet` | Network label returned with validator data |
| `LISTEN_ADDR` | `0.0.0.0` | HTTP server listen address |
| `LISTEN_PORT` | `8080` | HTTP server listen port |
| `VALIDATOR_REFRESH_INTERVAL` | `300` | Validator refresh interval in seconds |
| `VALIDATOR_LIST_SITES` | `https://vl.ripple.com,https://unl.xrplf.org` | Comma-separated validator list source URLs |
| `SECONDARY_VALIDATOR_REGISTRY_URL` | `https://api.xrpscan.com/api/v1/validatorregistry` | Secondary validator metadata source for domain enrichment |
| `VALIDATOR_METADATA_CACHE_PATH` | `data/validator-metadata-cache.json` | Persistent validator metadata cache keyed by validator key/address |
| `NETWORK_HEALTH_JSON_RPC_URLS` | `https://xrplcluster.com,https://s2.ripple.com:51234` | Ordered JSON-RPC fallback endpoints for `/network-health` |
| `NETWORK_HEALTH_RETRIES` | `2` | Retry attempts per health endpoint before trying next fallback |
| `GEO_CACHE_PATH` | `data/geolocation-cache.json` | Persistent geolocation cache path (survives process restarts) |
| `GEOLITE_DB_PATH` | `data/GeoLite2-City.mmdb` | Local path to GeoLite2 City MMDB file |
| `GEOLITE_DOWNLOAD_URL` | `https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb` | Download URL used when `GEOLITE_AUTO_DOWNLOAD=true` and DB file is missing |
| `GEOLITE_AUTO_DOWNLOAD` | `true` | Auto-download GeoLite DB at startup when missing |
| `MIN_PAYMENT_DROPS` | `1000000` | Minimum streamed payment amount in drops (1 XRP) |
| `TRANSACTION_BUFFER_SIZE` | `2048` | Internal listener queue for parsed transactions awaiting callback dispatch |
| `GEO_ENRICHMENT_QUEUE_SIZE` | `2048` | Queue for asynchronous geolocation enrichment jobs |
| `GEO_ENRICHMENT_WORKERS` | `8` | Number of concurrent workers resolving account geolocation |
| `MAX_GEO_CANDIDATES` | `6` | Max account candidates enriched per transaction (source/destination prioritized) |
| `BROADCAST_BUFFER_SIZE` | `2048` | Internal broadcast queue size before WebSocket fanout |
| `WS_CLIENT_BUFFER_SIZE` | `512` | Per-WebSocket-client pending transaction buffer size |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |

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

Establishes a WebSocket connection for streaming validated XRP `Payment` transactions where amount is at least `MIN_PAYMENT_DROPS` (default `1 XRP`).

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
  //   "locations": [
  //     { "latitude": 40.7128, "longitude": -74.0060, "validator_address": "r..." },
  //     { "latitude": 35.6895, "longitude": 139.6917, "validator_address": "r..." }
  //   ],
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
 External XRPL Endpoints  Frontend (Globe.GL)
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
│   ├── geolocation/
│   │   └── resolver.go       # GeoLite resolver + domain/IP/account cache
│   ├── validator/
│   │   └── fetcher.go        # Validator fetching logic
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
PUBLIC_RIPPLED_JSON_RPC_URL=https://xrplcluster.com \
PUBLIC_RIPPLED_WEBSOCKET_URL=wss://xrplcluster.com \
TRANSACTION_JSON_RPC_URL=https://xrplcluster.com \
TRANSACTION_WEBSOCKET_URL=wss://xrplcluster.com \
XRPL_NETWORK=mainnet \
VALIDATOR_LIST_SITES=https://vl.ripple.com,https://unl.xrplf.org \
SECONDARY_VALIDATOR_REGISTRY_URL=https://api.xrpscan.com/api/v1/validatorregistry \
VALIDATOR_METADATA_CACHE_PATH=data/validator-metadata-cache.json \
NETWORK_HEALTH_JSON_RPC_URLS=https://xrplcluster.com,https://s2.ripple.com:51234 \
NETWORK_HEALTH_RETRIES=2 \
GEO_CACHE_PATH=data/geolocation-cache.json \
GEOLITE_DB_PATH=data/GeoLite2-City.mmdb \
GEOLITE_DOWNLOAD_URL=https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb \
GEOLITE_AUTO_DOWNLOAD=true \
MIN_PAYMENT_DROPS=1000000 \
TRANSACTION_BUFFER_SIZE=2048 \
GEO_ENRICHMENT_QUEUE_SIZE=2048 \
GEO_ENRICHMENT_WORKERS=8 \
MAX_GEO_CANDIDATES=6 \
BROADCAST_BUFFER_SIZE=2048 \
WS_CLIENT_BUFFER_SIZE=512 \
LISTEN_PORT=9000 \
LOG_LEVEL=debug \
./validator-service
```

## Next Phases

- [ ] Phase 8: Enhanced error handling and resilience (reconnection logic, circuit breakers)
- [ ] Phase 10: Comprehensive unit tests
- [ ] Phase 11: Frontend integration with Globe.GL
- [ ] Validator transaction filtering and analytics
- [ ] Metrics and monitoring (Prometheus integration)
- [ ] Performance optimization

## Troubleshooting

### Connection refused to XRPL source

- Check `PUBLIC_RIPPLED_JSON_RPC_URL`, `PUBLIC_RIPPLED_WEBSOCKET_URL`, `TRANSACTION_JSON_RPC_URL`, and `TRANSACTION_WEBSOCKET_URL`
- Verify outbound network connectivity from the service to external XRPL endpoints
- Confirm endpoint health and that the URLs are reachable from your runtime environment

### No validators returned

- Verify the service has fetched data (check logs)
- Verify `VALIDATOR_LIST_SITES` and `SECONDARY_VALIDATOR_REGISTRY_URL` are reachable from the service
- Verify `PUBLIC_RIPPLED_JSON_RPC_URL` is reachable and serving `server_info`/`validators` requests

### WebSocket clients not receiving transactions

- Confirm transaction listener is subscribed (check health endpoint)
- Verify rippled transaction stream is active
- Check firewall/network policies for WebSocket connections

### Validators have no mapped coordinates

- Confirm the GeoLite MMDB exists at `GEOLITE_DB_PATH` (or that `GEOLITE_AUTO_DOWNLOAD` can fetch it)
- Keep `GEO_CACHE_PATH` on persistent storage so previously mapped validators are reused after restart
- Check that validator/account domains resolve to public IP addresses

## License

MIT

## Contributing

Contributions welcome! Please fork and submit PRs.
