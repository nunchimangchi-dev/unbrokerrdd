package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/agent"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/config"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
)

func TestAdminPanelAuth(t *testing.T) {
	// 1. Create a temp directory and store for testing
	tmpDir, err := os.MkdirTemp("", "unbrokerrdd-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbFile := filepath.Join(tmpDir, "test.db")
	
	// Set the bootstrap admin email env var
	os.Setenv("UNBROKERRDD_ADMIN_EMAIL", "admin@example.com")
	defer os.Unsetenv("UNBROKERRDD_ADMIN_EMAIL")

	store, err := db.New(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := &config.Config{
		SubjectName:  "Test Subject",
		SubjectEmail: "subject@example.com",
		SubjectState: "OH",
		AnthropicKey: "sk-ant-test-key-123",
		GmailSender:  "sender@example.com",
	}

	agents := make(map[int]agent.Agent)

	s := NewServer(store, cfg, agents)

	// (a) Verify a request with no header defaults to viewer role, and viewer cannot reach admin endpoints.
	t.Run("Viewer blocked from admin endpoints", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		// No Cf-Access-Authenticated-User-Email header
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
		}
	})

	t.Run("Viewer blocked with random email not in allowed_users", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		req.Header.Set("Cf-Access-Authenticated-User-Email", "random@example.com")
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
		}
	})

	t.Run("Admin can reach admin endpoints", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		req.Header.Set("Cf-Access-Authenticated-User-Email", "admin@example.com")
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		var users []db.AllowedUser
		if err := json.NewDecoder(rec.Body).Decode(&users); err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].Email != "admin@example.com" || users[0].Role != "admin" {
			t.Errorf("unexpected users returned: %+v", users)
		}
	})

	// (b) Verify last-admin invariant blocks demotion/removal
	t.Run("Last admin demotion blocked", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email": "admin@example.com",
			"role":  "viewer",
		})
		req := httptest.NewRequest(http.MethodPut, "/api/users", bytes.NewReader(body))
		req.Header.Set("Cf-Access-Authenticated-User-Email", "admin@example.com")
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d (bad request), got %d", http.StatusBadRequest, rec.Code)
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("cannot demote the last admin")) {
			t.Errorf("expected 'cannot demote the last admin' error, got: %s", rec.Body.String())
		}
	})

	t.Run("Last admin removal blocked", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/users?email=admin@example.com", nil)
		req.Header.Set("Cf-Access-Authenticated-User-Email", "admin@example.com")
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d (bad request), got %d", http.StatusBadRequest, rec.Code)
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("cannot remove the last admin")) {
			t.Errorf("expected 'cannot remove the last admin' error, got: %s", rec.Body.String())
		}
	})

	// Add another admin to verify we can demote/delete once there is another admin
	t.Run("CRUD allowed users", func(t *testing.T) {
		// Add new admin
		body, _ := json.Marshal(map[string]string{
			"email": "second@example.com",
			"role":  "admin",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
		req.Header.Set("Cf-Access-Authenticated-User-Email", "admin@example.com")
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
		}

		// Now we should be able to demote second admin
		bodyUpdate, _ := json.Marshal(map[string]string{
			"email": "second@example.com",
			"role":  "viewer",
		})
		reqUpdate := httptest.NewRequest(http.MethodPut, "/api/users", bytes.NewReader(bodyUpdate))
		reqUpdate.Header.Set("Cf-Access-Authenticated-User-Email", "admin@example.com")
		recUpdate := httptest.NewRecorder()
		s.ServeHTTP(recUpdate, reqUpdate)

		if recUpdate.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, recUpdate.Code)
		}

		// Delete the second user
		reqDelete := httptest.NewRequest(http.MethodDelete, "/api/users?email=second@example.com", nil)
		reqDelete.Header.Set("Cf-Access-Authenticated-User-Email", "admin@example.com")
		recDelete := httptest.NewRecorder()
		s.ServeHTTP(recDelete, reqDelete)

		if recDelete.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, recDelete.Code)
		}
	})

	// (c) starting a second batch run while one is active is rejected
	t.Run("Concurrent batch rejection", func(t *testing.T) {
		s.mu.Lock()
		s.batchRunning = true
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.batchRunning = false
			s.mu.Unlock()
		}()

		body, _ := json.Marshal(map[string]interface{}{
			"strategy": 1,
			"dry_run":  true,
			"limit":    1,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/batch", bytes.NewReader(body))
		req.Header.Set("Cf-Access-Authenticated-User-Email", "admin@example.com")
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Errorf("expected status %d (conflict), got %d", http.StatusConflict, rec.Code)
		}
	})

	// (d) confirm nothing in config or profile code ever logs or returns AnthropicKey or GmailSender
	t.Run("No leak of secrets", func(t *testing.T) {
		// Try to fetch profile, check response does not contain secret strings
		req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
		req.Header.Set("Cf-Access-Authenticated-User-Email", "admin@example.com")
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		bodyStr := rec.Body.String()
		if bytes.Contains(rec.Body.Bytes(), []byte("sk-ant-test-key-123")) {
			t.Errorf("secret AnthropicKey leaked in profile response: %s", bodyStr)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("sender@example.com")) {
			t.Errorf("secret GmailSender leaked in profile response: %s", bodyStr)
		}
	})
}
