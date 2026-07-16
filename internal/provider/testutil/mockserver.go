// Package testutil provides an in-memory mock of the Claude Enterprise
// Admin API spend limits endpoints for acceptance tests.
package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

// Member is an organization member served by the mock.
type Member struct {
	UserID  string
	Email   string
	Name    string
	Deleted bool
}

type storedLimit struct {
	ID     string
	UserID string
	Amount string
	Period string
}

// MockAdminAPI is a stateful fake of the spend limits endpoints.
type MockAdminAPI struct {
	server *httptest.Server
	apiKey string

	mu                sync.Mutex
	pageSize          int
	members           []Member
	overrides         map[string]*storedLimit // key: userID + "\x00" + period
	byID              map[string]*storedLimit
	nextID            int
	effectiveRequests int
}

// NewMockAdminAPI starts the mock server. Callers must Close it.
func NewMockAdminAPI(apiKey string, members []Member) *MockAdminAPI {
	m := &MockAdminAPI{
		apiKey:    apiKey,
		pageSize:  2,
		members:   members,
		overrides: map[string]*storedLimit{},
		byID:      map[string]*storedLimit{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/organizations/spend_limits/effective", m.handleEffective)
	mux.HandleFunc("/v1/organizations/spend_limits", m.handleUpsert)
	mux.HandleFunc("/v1/organizations/spend_limits/", m.handleByID)
	m.server = httptest.NewServer(mux)
	return m
}

// URL returns the mock's base URL for the provider's base_url attribute.
func (m *MockAdminAPI) URL() string { return m.server.URL }

// Close shuts the underlying server down.
func (m *MockAdminAPI) Close() { m.server.Close() }

// SetPageSize adjusts how many effective rows are served per page.
func (m *MockAdminAPI) SetPageSize(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pageSize = n
}

// EffectiveRequestCount reports how many times /effective was called.
func (m *MockAdminAPI) EffectiveRequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.effectiveRequests
}

// OverrideCount reports how many per-user overrides currently exist.
func (m *MockAdminAPI) OverrideCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.byID)
}

// SeedOverride injects a pre-existing override and returns its id.
func (m *MockAdminAPI) SeedOverride(userID, amount string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upsertLocked(userID, amount, "monthly").ID
}

// DeleteAllOverrides clears every stored override, simulating out-of-band
// deletion (e.g. an admin removing limits in the claude.ai console).
func (m *MockAdminAPI) DeleteAllOverrides() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.overrides = map[string]*storedLimit{}
	m.byID = map[string]*storedLimit{}
}

func (m *MockAdminAPI) upsertLocked(userID, amount, period string) *storedLimit {
	key := userID + "\x00" + period
	if existing, ok := m.overrides[key]; ok {
		existing.Amount = amount
		return existing
	}
	m.nextID++
	sl := &storedLimit{
		ID:     fmt.Sprintf("spl_mock%04d", m.nextID),
		UserID: userID,
		Amount: amount,
		Period: period,
	}
	m.overrides[key] = sl
	m.byID[sl.ID] = sl
	return sl
}

func (m *MockAdminAPI) authorized(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("x-api-key") != m.apiKey {
		writeError(w, http.StatusUnauthorized, "authentication_error", "invalid x-api-key")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":       "error",
		"error":      map[string]string{"type": errType, "message": msg},
		"request_id": "req_mock",
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func spendLimitJSON(sl *storedLimit) map[string]any {
	return map[string]any{
		"type":       "spend_limit",
		"id":         sl.ID,
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-01T00:00:00Z",
		"scope":      map[string]string{"type": "user", "user_id": sl.UserID},
		"amount":     sl.Amount,
		"currency":   "USD",
		"period":     sl.Period,
	}
}

func (m *MockAdminAPI) effectiveRow(member Member) map[string]any {
	row := map[string]any{
		"scope": map[string]string{"type": "user", "user_id": member.UserID},
		"actor": map[string]any{
			"type":          "user_actor",
			"user_id":       member.UserID,
			"name":          member.Name,
			"email_address": member.Email,
			"deleted":       member.Deleted,
		},
		"currency":             "USD",
		"period":               "monthly",
		"period_to_date_spend": "0",
	}
	if ov, ok := m.overrides[member.UserID+"\x00monthly"]; ok {
		row["amount"] = ov.Amount
		row["source"] = map[string]string{"type": "user"}
		row["spend_limit_id"] = ov.ID
	} else {
		row["amount"] = nil
		row["source"] = map[string]string{"type": "seat_tier", "seat_tier": "enterprise_standard"}
		row["spend_limit_id"] = "spl_inherited_" + member.UserID
	}
	return row
}

func (m *MockAdminAPI) handleEffective(w http.ResponseWriter, r *http.Request) {
	if !m.authorized(w, r) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.effectiveRequests++

	filter := map[string]bool{}
	for _, id := range r.URL.Query()["user_ids[]"] {
		filter[id] = true
	}
	var rows []map[string]any
	for _, member := range m.members {
		if len(filter) > 0 && !filter[member.UserID] {
			continue
		}
		rows = append(rows, m.effectiveRow(member))
	}

	offset := 0
	if cursor := r.URL.Query().Get("page"); cursor != "" {
		n, err := strconv.Atoi(strings.TrimPrefix(cursor, "page_"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request_error", "cursor does not match current query parameters")
			return
		}
		offset = n
	}
	end := offset + m.pageSize
	if end > len(rows) {
		end = len(rows)
	}
	var nextPage any
	if end < len(rows) {
		nextPage = fmt.Sprintf("page_%d", end)
	}
	page := rows[offset:end]
	if page == nil {
		page = []map[string]any{}
	}
	writeJSON(w, map[string]any{"data": page, "next_page": nextPage})
}

func (m *MockAdminAPI) handleUpsert(w http.ResponseWriter, r *http.Request) {
	if !m.authorized(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	var body struct {
		Scope struct {
			Type   string `json:"type"`
			UserID string `json:"user_id"`
		} `json:"scope"`
		Amount string `json:"amount"`
		Period string `json:"period"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "malformed JSON body")
		return
	}
	if body.Scope.Type != "user" {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "only scope.type \"user\" can be written")
		return
	}
	if body.Period == "" {
		body.Period = "monthly"
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	known := false
	for _, member := range m.members {
		if member.UserID == body.Scope.UserID {
			known = true
			break
		}
	}
	if !known {
		writeError(w, http.StatusNotFound, "not_found_error", "user not found: "+body.Scope.UserID)
		return
	}
	writeJSON(w, spendLimitJSON(m.upsertLocked(body.Scope.UserID, body.Amount, body.Period)))
}

func (m *MockAdminAPI) handleByID(w http.ResponseWriter, r *http.Request) {
	if !m.authorized(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/organizations/spend_limits/")

	m.mu.Lock()
	defer m.mu.Unlock()
	sl, ok := m.byID[id]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found_error", "spend limit not found: "+id)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, spendLimitJSON(sl))
	case http.MethodDelete:
		delete(m.byID, id)
		delete(m.overrides, sl.UserID+"\x00"+sl.Period)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}
