package dashboard

import (
	"context"
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

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/agent"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/config"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/orchestrator"
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

type contextKey string

const contextKeyUserEmail contextKey = "user_email"
const contextKeyUserRole contextKey = "user_role"

// Server holds all state for the dashboard HTTP/WebSocket server.
type Server struct {
	hub          *hub
	mu           sync.RWMutex
	brokers      []Broker
	store        *db.Store
	cfg          *config.Config
	agents       map[int]agent.Agent
	batchRunning bool
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewServer initialises the server with all brokers in pending state.
func NewServer(store *db.Store, cfg *config.Config, agents map[int]agent.Agent) *Server {
	s := &Server{
		hub:     newHub(),
		brokers: initBrokers(),
		store:   store,
		cfg:     cfg,
		agents:  agents,
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

func getUserRole(ctx context.Context) string {
	if role, ok := ctx.Value(contextKeyUserRole).(string); ok {
		return role
	}
	return "viewer"
}

func getUserEmail(ctx context.Context) string {
	if email, ok := ctx.Value(contextKeyUserEmail).(string); ok {
		return email
	}
	return "anonymous"
}

func (s *Server) identityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := r.Header.Get("Cf-Access-Authenticated-User-Email")
		if email == "" {
			email = "anonymous"
		}

		role := "viewer"
		if s.store != nil {
			var err error
			role, err = s.store.GetUserRole(email)
			if err != nil {
				log.Printf("[auth] Error fetching user role for %s: %v", email, err)
				role = "viewer"
			}
		} else {
			// In standalone demo mode with no store, let the user be an admin so they can play around
			role = "admin"
		}

		ctx := context.WithValue(r.Context(), contextKeyUserEmail, email)
		ctx = context.WithValue(ctx, contextKeyUserRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	email := getUserEmail(r.Context())
	role := getUserRole(r.Context())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"email": email,
		"role":  role,
	})
}

func (s *Server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if s.store == nil {
		// Mock list in demo mode
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{
			{"email": "admin@example.com", "role": "admin"},
			{"email": "viewer@example.com", "role": "viewer"},
		})
		return
	}
	users, err := s.store.ListAllowedUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func (s *Server) handleAddUser(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}
	if req.Role != "admin" && req.Role != "viewer" {
		http.Error(w, "Invalid role", http.StatusBadRequest)
		return
	}
	if s.store != nil {
		if err := s.store.AddAllowedUser(req.Email, req.Role); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Role != "admin" && req.Role != "viewer" {
		http.Error(w, "Invalid role", http.StatusBadRequest)
		return
	}
	if s.store != nil {
		if err := s.store.UpdateAllowedUserRole(req.Email, req.Role); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "Email query parameter is required", http.StatusBadRequest)
		return
	}
	if s.store != nil {
		if err := s.store.DeleteAllowedUser(email); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var prof *db.SubjectProfile
	if s.store != nil {
		var err error
		prof, err = s.store.GetSubjectProfile()
		if err != nil {
			// Fallback to config values (loaded from env) if row doesn't exist
			s.mu.RLock()
			prof = &db.SubjectProfile{
				Name:  s.cfg.SubjectName,
				Email: s.cfg.SubjectEmail,
				State: s.cfg.SubjectState,
			}
			s.mu.RUnlock()
		}
	} else {
		// Mock profile in demo mode
		prof = &db.SubjectProfile{
			Name:  "Jane Doe",
			Email: "jane.doe@example.com",
			State: "CA",
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prof)
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req db.SubjectProfile
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.State = strings.TrimSpace(req.State)
	if req.Name == "" || req.Email == "" || req.State == "" {
		http.Error(w, "All fields (Name, Email, State) are required", http.StatusBadRequest)
		return
	}
	if s.store != nil {
		if err := s.store.UpdateSubjectProfile(&req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Update the live in-memory config
		s.mu.Lock()
		if s.cfg != nil {
			s.cfg.SubjectName = req.Name
			s.cfg.SubjectEmail = req.Email
			s.cfg.SubjectState = req.State
		}
		s.mu.Unlock()
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStartBatch(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var req struct {
		Strategy int  `json:"strategy"`
		DryRun   bool `json:"dry_run"`
		Limit    int  `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Strategy < 1 || req.Strategy > 6 {
		http.Error(w, "Invalid strategy number (must be 1-6)", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.batchRunning {
		s.mu.Unlock()
		http.Error(w, "A batch is already running", http.StatusConflict)
		return
	}
	s.batchRunning = true
	s.mu.Unlock()

	// Run in goroutine so triggering request returns immediately
	go func() {
		defer func() {
			s.mu.Lock()
			s.batchRunning = false
			s.mu.Unlock()
		}()

		runner := orchestrator.New(s.store, s.cfg, s.agents, func(u orchestrator.StatusUpdate) {
			s.updateBroker(u.BrokerID, Status(u.Status))
		})

		delay := 5 * time.Second
		if req.DryRun {
			delay = 500 * time.Millisecond // faster in dry-run
		}

		ctx := context.Background()

		log.Printf("[dashboard] Starting batch for strategy %d (dry_run=%v, limit=%d)...", req.Strategy, req.DryRun, req.Limit)
		err := runner.RunBatch(ctx, orchestrator.BatchConfig{
			Strategy: req.Strategy,
			DryRun:   req.DryRun,
			Limit:    req.Limit,
			Delay:    delay,
		})
		if err != nil {
			log.Printf("[dashboard] Batch for strategy %d failed: %v", req.Strategy, err)
		} else {
			log.Printf("[dashboard] Batch for strategy %d complete.", req.Strategy)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "ID query parameter is required", http.StatusBadRequest)
		return
	}
	if s.store != nil {
		if err := s.store.Reset(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	// Update in-memory and broadcast
	s.updateBroker(id, StatusPending)
	w.WriteHeader(http.StatusOK)
}

// ServeHTTP implements http.Handler, routing static files and /ws.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()

	// Identity and role endpoints
	mux.HandleFunc("/api/me", s.handleMe)
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleGetUsers(w, r)
		case http.MethodPost:
			s.handleAddUser(w, r)
		case http.MethodPut:
			s.handleUpdateUser(w, r)
		case http.MethodDelete:
			s.handleDeleteUser(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Subject profile endpoints
	mux.HandleFunc("/api/profile", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleGetProfile(w, r)
		case http.MethodPut:
			s.handleUpdateProfile(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Batch and run control endpoints
	mux.HandleFunc("/api/batch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.handleStartBatch(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.handleReset(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Strip the "static/" prefix from the embedded FS so "/" serves index.html
	sub, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/ws", s.handleWS)

	s.identityMiddleware(mux).ServeHTTP(w, r)
}

// Serve starts the dashboard HTTP server.
//
// If store is non-nil, broker state is loaded from SQLite and the demo
// simulator is NOT started — the dashboard shows real data only.
// If store is nil, the demo simulator runs (standalone / dev mode).
//
// Security Model: This server is designed to bind to 0.0.0.0 inside a private,
// network-isolated tunnel. It trusts the "Cf-Access-Authenticated-User-Email"
// header injected by Cloudflare Access at the network edge. The allowed_users
// table maps these authenticated identities to 'admin' or 'viewer' roles.
func Serve(addr string, store *db.Store, cfg *config.Config, agents map[int]agent.Agent) error {
	s := NewServer(store, cfg, agents)

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
