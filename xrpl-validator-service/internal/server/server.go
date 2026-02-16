package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/brandon/xrpl-validator-service/internal/transaction"
	"github.com/brandon/xrpl-validator-service/internal/validator"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// Server manages HTTP and WebSocket connections
type Server struct {
	router              *gin.Engine
	logger              *logrus.Logger
	validatorFetcher    *validator.Fetcher
	transactionListener *transaction.Listener
	listenAddr          string
	listenPort          int
	corsAllowedOrigins  []string
	httpServer          *http.Server
	wsUpgrader          websocket.Upgrader
	wsClients           map[*WSClient]bool
	wsMu                sync.RWMutex
	broadcast           chan *models.Transaction
}

// WSClient represents a WebSocket client connection
type WSClient struct {
	conn   *websocket.Conn
	send   chan *models.Transaction
	server *Server
}

// NewServer creates a new HTTP server
func NewServer(
	validatorFetcher *validator.Fetcher,
	transactionListener *transaction.Listener,
	listenAddr string,
	listenPort int,
	corsAllowedOrigins []string,
	logger *logrus.Logger,
) *Server {
	if logger == nil {
		logger = logrus.New()
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	srv := &Server{
		router:              router,
		logger:              logger,
		validatorFetcher:    validatorFetcher,
		transactionListener: transactionListener,
		listenAddr:          listenAddr,
		listenPort:          listenPort,
		corsAllowedOrigins:  corsAllowedOrigins,
		wsClients:           make(map[*WSClient]bool),
		broadcast:           make(chan *models.Transaction, 256),
		wsUpgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				for _, allowed := range corsAllowedOrigins {
					if origin == allowed {
						return true
					}
				}
				return false
			},
		},
	}

	// Register routes
	srv.registerRoutes()

	// Register transaction callback
	transactionListener.AddCallback(srv.onTransaction)

	// Start broadcast loop
	go srv.broadcastLoop()

	return srv
}

// registerRoutes sets up all HTTP endpoints
func (s *Server) registerRoutes() {
	// CORS middleware (must be registered before routes)
	s.router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowed := false
		for _, allowedOrigin := range s.corsAllowedOrigins {
			if origin == allowedOrigin {
				allowed = true
				break
			}
		}
		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Health check
	s.router.GET("/health", s.handleHealth)

	// Validators endpoint
	s.router.GET("/validators", s.handleGetValidators)

	// Transactions WebSocket
	s.router.GET("/transactions", s.handleTransactionsWebSocket)
}

// handleHealth returns service health status
func (s *Server) handleHealth(c *gin.Context) {
	status := gin.H{
		"status":                      "ok",
		"validators_count":            len(s.validatorFetcher.GetValidators()),
		"last_validator_update":       s.validatorFetcher.GetLastUpdate(),
		"transaction_listener_active": s.transactionListener.IsSubscribed(),
		"websocket_clients":           len(s.wsClients),
	}
	c.JSON(http.StatusOK, status)
}

// handleGetValidators returns the list of validators
func (s *Server) handleGetValidators(c *gin.Context) {
	validators := s.validatorFetcher.GetValidators()
	c.JSON(http.StatusOK, gin.H{
		"validators": validators,
		"count":      len(validators),
		"timestamp":  s.validatorFetcher.GetLastUpdate(),
	})
}

// handleTransactionsWebSocket upgrades HTTP connection to WebSocket
func (s *Server) handleTransactionsWebSocket(c *gin.Context) {
	conn, err := s.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.WithError(err).Error("WebSocket upgrade failed")
		c.JSON(http.StatusBadRequest, gin.H{"error": "WebSocket upgrade failed"})
		return
	}

	client := &WSClient{
		conn:   conn,
		send:   make(chan *models.Transaction, 256),
		server: s,
	}

	s.wsMu.Lock()
	s.wsClients[client] = true
	s.wsMu.Unlock()

	s.logger.WithField("client_addr", conn.RemoteAddr()).Info("WebSocket client connected")

	// Start client goroutines
	go client.readPump()
	go client.writePump()
}

// onTransaction is called when a new transaction is received
func (s *Server) onTransaction(tx *models.Transaction) {
	select {
	case s.broadcast <- tx:
	default:
		s.logger.Warn("Broadcast channel full, dropping transaction")
	}
}

// broadcastLoop distributes transactions to all connected clients
func (s *Server) broadcastLoop() {
	for tx := range s.broadcast {
		s.wsMu.RLock()
		clients := make([]*WSClient, 0, len(s.wsClients))
		for client := range s.wsClients {
			clients = append(clients, client)
		}
		s.wsMu.RUnlock()

		for _, client := range clients {
			select {
			case client.send <- tx:
			default:
				go s.closeClient(client)
			}
		}
	}
}

// closeClient closes a WebSocket client connection
func (s *Server) closeClient(client *WSClient) {
	s.wsMu.Lock()
	delete(s.wsClients, client)
	s.wsMu.Unlock()
	close(client.send)
	client.conn.Close()
	s.logger.WithField("client_addr", client.conn.RemoteAddr()).Info("WebSocket client disconnected")
}

// readPump reads messages from the WebSocket client
func (c *WSClient) readPump() {
	defer func() {
		c.server.closeClient(c)
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.server.logger.WithError(err).Warn("WebSocket error")
			}
			break
		}
	}
}

// writePump writes messages to the WebSocket client
func (c *WSClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case tx, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(tx); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.listenAddr, s.listenPort)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	s.logger.WithField("address", addr).Info("Starting HTTP server")
	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the HTTP server and closes broadcast channel
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		err := s.httpServer.Shutdown(ctx)
		// Close broadcast channel to stop broadcastLoop goroutine
		close(s.broadcast)
		return err
	}
	return nil
}
