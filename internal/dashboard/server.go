package dashboard

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
)

// dbBroker is an alias so the file compiles without repeating the import path.
type dbBroker = db.Broker

//go:embed static
var staticFiles embed.FS

// Stats holds live aggregate counts for the dashboard.
type Stats struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Success    int `json:"success"`
	Failed     int `json:"failed"`
	Skipped    int `json:"skipped"`
	Manual     int `json:"manual"`
}

// Message is the WebSocket envelope sent to clients.
type Message struct {
	Type    string   `json:"type"`
	Broker  *Broker  `json:"broker,omitempty"`
	Brokers []Broker `json:"brokers,omitempty"`
	Stats   *Stats   `json:"stats,omitempty"`
}

// ─── Hub ─────────────────────────────────────────────────────────────────────

// hub manages all WebSocket subscribers. It is goroutine-safe.
type hub struct {
	mu        sync.Mutex
	clients   map[chan []byte]bool
	broadcast chan []byte
}

func newHub() *hub {
	h := &hub{
		clients:   make(map[chan []byte]bool),
		broadcast: make(chan []byte, 512),
	}
	go h.run()
	return h
}

func (h *hub) run() {
	for msg := range h.broadcast {
		h.mu.Lock()
		for ch := range h.clients {
			select {
			case ch <- msg:
			default:
				// slow client — drop the frame, don't block
			}
		}
		h.mu.Unlock()
	}
}

func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, 128)
	h.mu.Lock()
	h.clients[ch] = true
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

// ─── Server ──────────────────────────────────────────────────────────────────

// Server holds all state for the dashboard HTTP/WebSocket server.
type Server struct {
	hub     *hub
	mu      sync.RWMutex
	brokers []Broker
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewServer initialises the server with all brokers in pending state.
func NewServer() *Server {
	s := &Server{
		hub:     newHub(),
		brokers: initBrokers(),
	}
	// All brokers start pending — already set by zero-value of Status field,
	// but be explicit for clarity.
	for i := range s.brokers {
		s.brokers[i].Status = StatusPending
	}
	return s
}

func (s *Server) getStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Stats{Total: len(s.brokers)}
	for _, b := range s.brokers {
		switch b.Status {
		case StatusPending:
			st.Pending++
		case StatusInProgress:
			st.InProgress++
		case StatusSuccess:
			st.Success++
		case StatusFailed:
			st.Failed++
		case StatusSkipped:
			st.Skipped++
		case StatusManual:
			st.Manual++
		}
	}
	return st
}

// updateBroker changes a broker's status and broadcasts the change to all clients.
func (s *Server) updateBroker(id string, status Status) {
	s.mu.Lock()
	var updated *Broker
	for i := range s.brokers {
		if s.brokers[i].ID == id {
			s.brokers[i].Status = status
			b := s.brokers[i]
			updated = &b
			break
		}
	}
	s.mu.Unlock()

	if updated == nil {
		return
	}

	stats := s.getStats()
	msg := Message{Type: "update", Broker: updated, Stats: &stats}
	data, _ := json.Marshal(msg)
	s.hub.broadcast <- data
}

// loadFromStore replaces the in-memory broker list with real SQLite state.
func (s *Server) loadFromStore(dbBrokers []db.Broker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.brokers = make([]Broker, len(dbBrokers))
	for i, b := range dbBrokers {
		s.brokers[i] = Broker{
			ID:       b.ID,
			Name:     b.Name,
			Strategy: b.Strategy,
			URL:      b.URL,
			Status:   Status(b.Status),
		}
	}
}

// RunDemo simulates a realistic agent run through all brokers in strategy order.
// It is intended to run as a goroutine. Replace with real agent dispatch in Phase A.
func (s *Server) RunDemo() {
	time.Sleep(2 * time.Second) // let clients connect first

	s.mu.RLock()
	var order []string
	for strategy := 1; strategy <= 6; strategy++ {
		for _, b := range s.brokers {
			if b.Strategy == strategy {
				order = append(order, b.ID)
			}
		}
	}
	s.mu.RUnlock()

	for _, id := range order {
		s.updateBroker(id, StatusInProgress)

		// Simulate work: 1.2s – 4.5s
		work := time.Duration(1200+rand.Intn(3300)) * time.Millisecond
		time.Sleep(work)

		// Determine realistic outcome per strategy
		s.mu.RLock()
		var broker Broker
		for _, b := range s.brokers {
			if b.ID == id {
				broker = b
				break
			}
		}
		s.mu.RUnlock()

		final := outcomeFor(broker)
		s.updateBroker(id, final)

		// Gap between brokers
		time.Sleep(400 * time.Millisecond)
	}

	log.Println("Demo simulation complete.")
}

// outcomeFor returns a realistic simulated outcome for a broker based on its strategy.
func outcomeFor(b Broker) Status {
	switch b.Strategy {
	case 4: // Business directories — mostly no personal data
		if rand.Float32() < 0.78 {
			return StatusSkipped
		}
		return StatusSuccess
	case 6: // Profile brokers — some require manual handling
		r := rand.Float32()
		switch {
		case r < 0.25:
			return StatusManual
		case r < 0.70:
			return StatusSuccess
		default:
			return StatusFailed
		}
	default:
		r := rand.Float32()
		switch {
		case r < 0.76:
			return StatusSuccess
		case r < 0.91:
			return StatusFailed
		default:
			return StatusManual
		}
	}
}

// handleWS upgrades an HTTP connection to WebSocket and streams broker updates.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ws upgrade:", err)
		return
	}
	defer conn.Close()

	// Send full state on connect
	s.mu.RLock()
	snapshot := make([]Broker, len(s.brokers))
	copy(snapshot, s.brokers)
	s.mu.RUnlock()

	stats := s.getStats()
	init_, _ := json.Marshal(Message{Type: "state", Brokers: snapshot, Stats: &stats})
	conn.WriteMessage(websocket.TextMessage, init_)

	// Subscribe to live updates
	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	// Read loop (keep-alive / detect disconnect)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case msg := <-ch:
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// ServeHTTP implements http.Handler, routing static files and /ws.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()

	// Strip the "static/" prefix from the embedded FS so "/" serves index.html
	sub, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/ws", s.handleWS)

	mux.ServeHTTP(w, r)
}

// Serve starts the dashboard HTTP server.
//
// If store is non-nil, broker state is loaded from SQLite and the demo
// simulator is NOT started — the dashboard shows real data only.
// If store is nil, the demo simulator runs (standalone / dev mode).
//
// Security: addr MUST be a loopback address (127.0.0.1:port).
// Never bind to 0.0.0.0 — this dashboard has no auth layer.
func Serve(addr string, store interface{ GetAll() ([]dbBroker, error) }) error {
	s := NewServer()

	if store != nil {
		// Load real state from SQLite
		brokers, err := store.GetAll()
		if err != nil {
			log.Printf("warn: could not load from store: %v — falling back to demo", err)
			go s.RunDemo()
		} else {
			s.loadFromStore(brokers)
			log.Printf("loaded %d brokers from SQLite", len(brokers))
		}
	} else {
		go s.RunDemo()
	}

	log.Printf("Dashboard: http://localhost%s", addr[strings.LastIndex(addr, ":"):])
	return http.ListenAndServe(addr, s)
}
