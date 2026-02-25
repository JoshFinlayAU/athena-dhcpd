package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
)

// sseClient is a connected SSE client with a buffered send channel.
type sseClient struct {
	send chan []byte
}

// SSEHub manages Server-Sent Event connections for live event streaming.
type SSEHub struct {
	bus     *events.Bus
	logger  *slog.Logger
	clients map[*sseClient]struct{}
	mu      sync.Mutex
	done    chan struct{}
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub(bus *events.Bus, logger *slog.Logger) *SSEHub {
	return &SSEHub{
		bus:     bus,
		logger:  logger,
		clients: make(map[*sseClient]struct{}),
		done:    make(chan struct{}),
	}
}

// Run starts the SSE hub, subscribing to the event bus and broadcasting.
func (h *SSEHub) Run() {
	ch := h.bus.Subscribe(500)

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			h.broadcast(data)
		case <-h.done:
			h.bus.Unsubscribe(ch)
			return
		}
	}
}

// Stop shuts down the SSE hub and closes all client channels.
func (h *SSEHub) Stop() {
	close(h.done)
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		close(client.send)
		delete(h.clients, client)
	}
}

// broadcast sends data to all connected SSE clients.
func (h *SSEHub) broadcast(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			// Client too slow — disconnect
			close(client.send)
			delete(h.clients, client)
		}
	}
}

func (h *SSEHub) addClient(c *sseClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	metrics.SSEConnections.Inc()
}

func (h *SSEHub) removeClient(c *sseClient) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		close(c.send)
		delete(h.clients, c)
		metrics.SSEConnections.Dec()
	}
	h.mu.Unlock()
}

// handleSSE streams events to the client via Server-Sent Events.
// Works over plain HTTP, no SSL required, auto-reconnects in browsers via EventSource.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	client := &sseClient{
		send: make(chan []byte, 256),
	}
	s.sseHub.addClient(client)
	defer s.sseHub.removeClient(client)

	s.logger.Debug("SSE client connected", "remote", r.RemoteAddr)

	// Keep-alive ticker sends a comment line every 30s to prevent proxy timeouts
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-client.send:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			s.logger.Debug("SSE client disconnected", "remote", r.RemoteAddr)
			return
		}
	}
}
