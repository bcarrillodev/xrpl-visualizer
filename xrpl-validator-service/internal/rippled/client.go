package rippled

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// RippledClient defines the interface for rippled interactions
type RippledClient interface {
	// Connect establishes connection to rippled
	Connect(ctx context.Context) error

	// Close closes the connection
	Close() error

	// IsConnected returns connection status
	IsConnected() bool

	// Command sends a JSON-RPC command and gets response
	Command(ctx context.Context, method string, params interface{}) (interface{}, error)

	// Subscribe subscribes to streams (used for transactions, ledger_closed, etc)
	Subscribe(ctx context.Context, streams []string, callback func(interface{})) error

	// Unsubscribe unsubscribes from streams
	Unsubscribe(ctx context.Context, streams []string) error

	// GetValidators fetches validator info
	GetValidators(ctx context.Context) (interface{}, error)

	// GetServerInfo fetches server status
	GetServerInfo(ctx context.Context) (interface{}, error)
}

// Client implements RippledClient
type Client struct {
	jsonRPCURL     string
	websocketURL   string
	wsConn         *websocket.Conn
	httpClient     *http.Client
	logger         *logrus.Logger
	callbacks      []func(interface{})
	mu             sync.RWMutex
	connected      bool
	reconnectCount int
	maxReconnects  int
	backoffTime    time.Duration
}

// NewClient creates a new rippled client
func NewClient(jsonRPCURL, websocketURL string, logger *logrus.Logger) *Client {
	if logger == nil {
		logger = logrus.New()
	}
	return &Client{
		jsonRPCURL:    jsonRPCURL,
		websocketURL:  websocketURL,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		logger:        logger,
		callbacks:     make([]func(interface{}), 0),
		maxReconnects: 10,
		backoffTime:   time.Second,
	}
}

// Connect establishes WebSocket connection to rippled
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, c.websocketURL, nil)
	if err != nil {
		c.logger.WithError(err).Error("Failed to connect to rippled WebSocket")
		return err
	}

	c.wsConn = conn
	c.connected = true
	c.reconnectCount = 0
	c.logger.Info("Connected to rippled WebSocket")

	// Start read loop for handling incoming messages
	go c.readLoop()

	return nil
}

// Close closes the connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.wsConn != nil {
		c.connected = false
		return c.wsConn.Close()
	}
	return nil
}

// IsConnected returns connection status
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Command sends a JSON-RPC command via HTTP
func (c *Client) Command(ctx context.Context, method string, params interface{}) (interface{}, error) {
	payload := map[string]interface{}{
		"method":  method,
		"params":  []interface{}{params},
		"id":      1,
		"jsonrpc": "2.0",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.jsonRPCURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).WithField("method", method).Error("RPC command failed")
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Check for JSON-RPC error response
	if errorResult, ok := result["error"]; ok {
		return nil, fmt.Errorf("JSON-RPC error: %v", errorResult)
	}

	return result, nil
}

// Subscribe subscribes to rippled streams
func (c *Client) Subscribe(ctx context.Context, streams []string, callback func(interface{})) error {
	c.mu.RLock()
	if !c.connected || c.wsConn == nil {
		c.mu.RUnlock()
		return fmt.Errorf("not connected to rippled")
	}
	c.mu.RUnlock()

	// Send subscribe command
	cmd := map[string]interface{}{
		"command": "subscribe",
		"streams": streams,
	}

	c.mu.Lock()
	if callback != nil {
		c.callbacks = append(c.callbacks, callback)
	}
	if err := c.wsConn.WriteJSON(cmd); err != nil {
		c.mu.Unlock()
		c.logger.WithError(err).Error("Failed to send subscribe command")
		return err
	}
	c.mu.Unlock()

	return nil
}

// Unsubscribe unsubscribes from streams
func (c *Client) Unsubscribe(ctx context.Context, streams []string) error {
	c.mu.RLock()
	if !c.connected || c.wsConn == nil {
		c.mu.RUnlock()
		return fmt.Errorf("not connected to rippled")
	}
	c.mu.RUnlock()

	cmd := map[string]interface{}{
		"command": "unsubscribe",
		"streams": streams,
	}

	c.mu.Lock()
	if err := c.wsConn.WriteJSON(cmd); err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	return nil
}

// GetValidators fetches validator information
func (c *Client) GetValidators(ctx context.Context) (interface{}, error) {
	return c.Command(ctx, "manifest", map[string]interface{}{})
}

// GetServerInfo fetches server information
func (c *Client) GetServerInfo(ctx context.Context) (interface{}, error) {
	return c.Command(ctx, "server_info", map[string]interface{}{})
}

// readLoop reads incoming messages from WebSocket
func (c *Client) readLoop() {
	for {
		c.mu.RLock()
		if !c.connected || c.wsConn == nil {
			c.mu.RUnlock()
			break
		}
		conn := c.wsConn
		c.mu.RUnlock()

		var msg interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			c.logger.WithError(err).Warn("WebSocket read error")
			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
			break
		}

		c.mu.RLock()
		callbacks := make([]func(interface{}), len(c.callbacks))
		copy(callbacks, c.callbacks)
		c.mu.RUnlock()

		for _, callback := range callbacks {
			if callback != nil {
				callback(msg)
			}
		}
	}
}
