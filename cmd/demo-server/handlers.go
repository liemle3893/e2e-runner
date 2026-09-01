package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

// writeJSON renders v as the response body with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			log.Printf("write response: %v", err)
		}
	}
}

// writeError renders an {"error": "..."} body, the shape every failure uses.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON reads a JSON request body into dst, reporting a 400 on malformed
// input. It returns false when a response has already been written.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// ---------------------------------------------------------------- health ----

// handleHealth reports the reachability of every backend. It answers 503 when
// any of them is down, so a suite can gate on the whole stack being up.
func (s *services) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Probe concurrently: four sequential timeouts would make an all-down
	// stack take 20 seconds to report.
	type probe struct {
		name string
		ok   bool
	}
	results := make(chan probe, 4)
	checks := map[string]func() bool{
		"postgres": func() bool { return s.pgHealthy(ctx) },
		"redis":    func() bool { return s.redisHealthy(ctx) },
		"mongodb":  func() bool { return s.mongoHealthy(ctx) },
		"eventhub": func() bool { return s.eventHubHealthy(ctx) },
	}
	for name, check := range checks {
		go func(n string, c func() bool) { results <- probe{n, c()} }(name, check)
	}

	statuses := make(map[string]bool, len(checks))
	allHealthy := true
	for range checks {
		p := <-results
		statuses[p.name] = p.ok
		if !p.ok {
			allHealthy = false
		}
	}

	status := http.StatusOK
	label := "healthy"
	if !allHealthy {
		status = http.StatusServiceUnavailable
		label = "unhealthy"
	}

	writeJSON(w, status, map[string]any{
		"status":    label,
		"services":  statuses,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// handleServiceHealth reports the reachability of a single named backend.
func (s *services) handleServiceHealth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("service")
	ctx := r.Context()

	var healthy bool
	switch name {
	case "postgres":
		healthy = s.pgHealthy(ctx)
	case "redis":
		healthy = s.redisHealthy(ctx)
	case "mongodb":
		healthy = s.mongoHealthy(ctx)
	case "eventhub":
		healthy = s.eventHubHealthy(ctx)
	default:
		writeError(w, http.StatusNotFound, "Unknown service: "+name)
		return
	}

	status := http.StatusOK
	label := "healthy"
	if !healthy {
		status = http.StatusServiceUnavailable
		label = "unhealthy"
	}
	writeJSON(w, status, map[string]any{
		"service":   name,
		"status":    label,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ----------------------------------------------------------------- users ----

// user mirrors a row of the users table. Field names match the column names so
// the JSON shape is the row itself, as the previous implementation returned.
type user struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// scanUser collects a single users row.
func scanUser(row pgx.Row) (*user, error) {
	var u user
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// userColumns is the explicit projection used everywhere, so the JSON field
// order and set do not depend on the table's physical column order.
const userColumns = "id, email, name, created_at, updated_at"

func (s *services) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "email and name are required")
		return
	}

	u, err := scanUser(s.pg.QueryRow(r.Context(),
		"INSERT INTO users (email, name) VALUES ($1, $2) RETURNING "+userColumns,
		req.Email, req.Name))
	if err != nil {
		log.Printf("error creating user: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *services) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}

	rows, err := s.pg.Query(r.Context(),
		"SELECT "+userColumns+" FROM users ORDER BY created_at DESC")
	if err != nil {
		log.Printf("error listing users: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}
	defer rows.Close()

	// Start from an empty slice so an empty table renders as [] rather than null.
	users := []user{}
	for rows.Next() {
		u, scanErr := scanUser(rows)
		if scanErr != nil {
			log.Printf("error listing users: %v", scanErr)
			writeError(w, http.StatusInternalServerError, "Failed to list users")
			return
		}
		users = append(users, *u)
	}
	if rows.Err() != nil {
		log.Printf("error listing users: %v", rows.Err())
		writeError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *services) handleGetUser(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeError(w, http.StatusInternalServerError, "Failed to get user")
		return
	}

	id, ok := userID(w, r)
	if !ok {
		return
	}

	u, err := scanUser(s.pg.QueryRow(r.Context(),
		"SELECT "+userColumns+" FROM users WHERE id = $1", id))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "User not found")
	case err != nil:
		log.Printf("error getting user: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get user")
	default:
		writeJSON(w, http.StatusOK, u)
	}
}

func (s *services) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeError(w, http.StatusInternalServerError, "Failed to update user")
		return
	}

	id, ok := userID(w, r)
	if !ok {
		return
	}

	// Pointers distinguish "field omitted" from "field set to empty", so a
	// request updating only the name does not blank the email.
	var req struct {
		Email *string `json:"email"`
		Name  *string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	sets := ""
	args := []any{}
	if req.Email != nil {
		args = append(args, *req.Email)
		sets += "email = $" + strconv.Itoa(len(args)) + ", "
	}
	if req.Name != nil {
		args = append(args, *req.Name)
		sets += "name = $" + strconv.Itoa(len(args)) + ", "
	}
	if len(args) == 0 {
		writeError(w, http.StatusBadRequest, "No fields to update")
		return
	}
	args = append(args, id)

	sql := "UPDATE users SET " + sets + "updated_at = NOW() WHERE id = $" +
		strconv.Itoa(len(args)) + " RETURNING " + userColumns

	u, err := scanUser(s.pg.QueryRow(r.Context(), sql, args...))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "User not found")
	case err != nil:
		log.Printf("error updating user: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to update user")
	default:
		writeJSON(w, http.StatusOK, u)
	}
}

func (s *services) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	id, ok := userID(w, r)
	if !ok {
		return
	}

	tag, err := s.pg.Exec(r.Context(), "DELETE FROM users WHERE id = $1", id)
	if err != nil {
		log.Printf("error deleting user: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// userID parses the {id} path segment. A non-numeric id is "not found" rather
// than a 500, since the column is an integer and no such user can exist.
func userID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "User not found")
		return 0, false
	}
	return id, true
}

// ----------------------------------------------------------------- cache ----

func (s *services) handleGetCache(w http.ResponseWriter, r *http.Request) {
	if s.redis == nil {
		writeError(w, http.StatusInternalServerError, "Failed to get cache value")
		return
	}

	key := r.PathValue("key")
	value, err := s.redis.Get(r.Context(), key).Result()
	switch {
	case errors.Is(err, redis.Nil):
		writeError(w, http.StatusNotFound, "Key not found")
	case err != nil:
		log.Printf("error getting cache: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get cache value")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": value})
	}
}

func (s *services) handleSetCache(w http.ResponseWriter, r *http.Request) {
	if s.redis == nil {
		writeError(w, http.StatusInternalServerError, "Failed to set cache value")
		return
	}

	key := r.PathValue("key")

	// A pointer detects an omitted value, which is an error, as distinct from
	// an empty string, which is a legal cache value.
	var req struct {
		Value *string `json:"value"`
		TTL   int64   `json:"ttl"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Value == nil {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}

	expiry := time.Duration(0)
	if req.TTL > 0 {
		expiry = time.Duration(req.TTL) * time.Second
	}
	if err := s.redis.Set(r.Context(), key, *req.Value, expiry).Err(); err != nil {
		log.Printf("error setting cache: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to set cache value")
		return
	}

	// ttl is null rather than 0 when unset, matching the documented shape.
	var ttl any
	if req.TTL > 0 {
		ttl = req.TTL
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key":     key,
		"value":   *req.Value,
		"ttl":     ttl,
		"message": "Cache value set successfully",
	})
}

func (s *services) handleDeleteCache(w http.ResponseWriter, r *http.Request) {
	if s.redis == nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete cache value")
		return
	}

	deleted, err := s.redis.Del(r.Context(), r.PathValue("key")).Result()
	if err != nil {
		log.Printf("error deleting cache: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to delete cache value")
		return
	}
	if deleted == 0 {
		writeError(w, http.StatusNotFound, "Key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *services) handleHeadCache(w http.ResponseWriter, r *http.Request) {
	if s.redis == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	count, err := s.redis.Exists(r.Context(), r.PathValue("key")).Result()
	switch {
	case err != nil:
		log.Printf("error checking cache: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	case count == 0:
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusOK)
	}
}
