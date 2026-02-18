package server

import (
	"context"
	"testing"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/sirupsen/logrus"
)

func newTestServer() *Server {
	return &Server{
		logger:             logrus.New(),
		wsClients:          make(map[*WSClient]bool),
		broadcast:          make(chan *models.Transaction, 4),
		stopBroadcast:      make(chan struct{}),
		wsClientBufferSize: 4,
	}
}

func TestCloseClientIsIdempotent(t *testing.T) {
	srv := newTestServer()
	client := &WSClient{
		send:   make(chan *models.Transaction),
		server: srv,
	}
	srv.wsClients[client] = true

	srv.closeClient(client)
	srv.closeClient(client)

	select {
	case _, ok := <-client.send:
		if ok {
			t.Fatal("expected client send channel to be closed")
		}
	default:
		t.Fatal("expected closed channel read to be immediately available")
	}

	if count := srv.websocketClientCount(); count != 0 {
		t.Fatalf("expected 0 websocket clients after close, got %d", count)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	srv := newTestServer()

	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop failed: %v", err)
	}
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func TestOnTransactionAfterStopDoesNotEnqueue(t *testing.T) {
	srv := newTestServer()
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	srv.onTransaction(&models.Transaction{Hash: "ABC"})

	select {
	case <-srv.broadcast:
		t.Fatal("did not expect transaction enqueue after stop")
	default:
	}
}

func TestBroadcastLoopStopsWhenSignaled(t *testing.T) {
	srv := newTestServer()
	done := make(chan struct{})

	go func() {
		srv.broadcastLoop()
		close(done)
	}()

	close(srv.stopBroadcast)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcastLoop did not stop after stop signal")
	}
}
