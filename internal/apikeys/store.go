package apikeys

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/SEObserver/seocrawler/internal/customtests"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	Type       string     `json:"type"` // "general" | "project"
	ProjectID  *string    `json:"project_id"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	Active     bool       `json:"active"`
}

type APIKeyCreateResult struct {
	APIKey
	FullKey string `json:"key"`
}

type KeyLookupResult struct {
	ID        string
	Type      string
	ProjectID *string
}

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}

	// Enable WAL mode and foreign keys
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma: %w", err)
		}
	}

	// Create tables
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating projects table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			key_prefix TEXT NOT NULL,
			type TEXT NOT NULL CHECK(type IN ('general', 'project')),
			project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME,
			active INTEGER DEFAULT 1
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating api_keys table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS gsc_connections (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
			property_url TEXT NOT NULL,
			access_token TEXT NOT NULL,
			refresh_token TEXT NOT NULL,
			token_expiry DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating gsc_connections table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS test_profiles (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating test_profiles table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS test_rules (
			id TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL REFERENCES test_profiles(id) ON DELETE CASCADE,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			value TEXT NOT NULL,
			extra TEXT DEFAULT '',
			sort_order INTEGER DEFAULT 0
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating test_rules table: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// --- Projects ---

func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	if projects == nil {
		projects = []Project{}
	}
	return projects, nil
}

func (s *Store) CreateProject(name string) (*Project, error) {
	p := &Project{
		ID:        uuid.New().String(),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.Exec(`INSERT INTO projects (id, name, created_at) VALUES (?, ?, ?)`,
		p.ID, p.Name, p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}
	return p, nil
}

func (s *Store) GetProject(id string) (*Project, error) {
	var p Project
	err := s.db.QueryRow(`SELECT id, name, created_at FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) RenameProject(id, name string) error {
	res, err := s.db.Exec(`UPDATE projects SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

func (s *Store) DeleteProject(id string) error {
	res, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

// --- API Keys ---

func (s *Store) ListAPIKeys() ([]APIKey, error) {
	rows, err := s.db.Query(`
		SELECT id, name, key_prefix, type, project_id, created_at, last_used_at, active
		FROM api_keys ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.Type, &k.ProjectID,
			&k.CreatedAt, &k.LastUsedAt, &k.Active); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	if keys == nil {
		keys = []APIKey{}
	}
	return keys, nil
}

func (s *Store) CreateAPIKey(name, keyType string, projectID *string) (*APIKeyCreateResult, error) {
	if keyType != "general" && keyType != "project" {
		return nil, fmt.Errorf("invalid key type: %s", keyType)
	}
	if keyType == "project" && (projectID == nil || *projectID == "") {
		return nil, fmt.Errorf("project_id required for project keys")
	}

	// Generate random key: 32 bytes -> hex -> prefix with sk_
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}
	fullKey := "sk_" + hex.EncodeToString(rawBytes)

	// Hash for storage
	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])

	// Display prefix: sk_ + first 8 hex chars
	keyPrefix := fullKey[:11] + "..."

	k := APIKey{
		ID:        uuid.New().String(),
		Name:      name,
		KeyPrefix: keyPrefix,
		Type:      keyType,
		ProjectID: projectID,
		CreatedAt: time.Now().UTC(),
		Active:    true,
	}

	_, err := s.db.Exec(`
		INSERT INTO api_keys (id, name, key_hash, key_prefix, type, project_id, created_at, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)`,
		k.ID, k.Name, keyHash, k.KeyPrefix, k.Type, k.ProjectID, k.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("inserting api key: %w", err)
	}

	return &APIKeyCreateResult{APIKey: k, FullKey: fullKey}, nil
}

func (s *Store) DeleteAPIKey(id string) error {
	res, err := s.db.Exec(`DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("api key not found")
	}
	return nil
}

// --- GSC Connections ---

type GSCConnection struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	PropertyURL  string    `json:"property_url"`
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	TokenExpiry  time.Time `json:"token_expiry"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) SaveGSCConnection(conn *GSCConnection) error {
	if conn.ID == "" {
		conn.ID = uuid.New().String()
	}
	_, err := s.db.Exec(`
		INSERT INTO gsc_connections (id, project_id, property_url, access_token, refresh_token, token_expiry, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
			property_url = excluded.property_url,
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			token_expiry = excluded.token_expiry`,
		conn.ID, conn.ProjectID, conn.PropertyURL, conn.AccessToken, conn.RefreshToken, conn.TokenExpiry, time.Now().UTC())
	return err
}

func (s *Store) GetGSCConnection(projectID string) (*GSCConnection, error) {
	var c GSCConnection
	err := s.db.QueryRow(`
		SELECT id, project_id, property_url, access_token, refresh_token, token_expiry, created_at
		FROM gsc_connections WHERE project_id = ?`, projectID).
		Scan(&c.ID, &c.ProjectID, &c.PropertyURL, &c.AccessToken, &c.RefreshToken, &c.TokenExpiry, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) DeleteGSCConnection(projectID string) error {
	res, err := s.db.Exec(`DELETE FROM gsc_connections WHERE project_id = ?`, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("gsc connection not found")
	}
	return nil
}

func (s *Store) ListGSCConnections() ([]GSCConnection, error) {
	rows, err := s.db.Query(`SELECT id, project_id, property_url, access_token, refresh_token, token_expiry, created_at FROM gsc_connections ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []GSCConnection
	for rows.Next() {
		var c GSCConnection
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.PropertyURL, &c.AccessToken, &c.RefreshToken, &c.TokenExpiry, &c.CreatedAt); err != nil {
			return nil, err
		}
		conns = append(conns, c)
	}
	if conns == nil {
		conns = []GSCConnection{}
	}
	return conns, nil
}

func (s *Store) ValidateKey(rawKey string) *KeyLookupResult {
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	var result KeyLookupResult
	err := s.db.QueryRow(`
		SELECT id, type, project_id FROM api_keys
		WHERE key_hash = ? AND active = 1`,
		keyHash).Scan(&result.ID, &result.Type, &result.ProjectID)
	if err != nil {
		return nil
	}

	// Update last_used_at
	if _, err := s.db.Exec(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`, time.Now().UTC(), result.ID); err != nil {
		log.Printf("warning: failed to update last_used_at: %v", err)
	}

	return &result
}

// --- Test Profiles ---

func (s *Store) ListTestProfiles() ([]customtests.TestProfile, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at, updated_at FROM test_profiles ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []customtests.TestProfile
	for rows.Next() {
		var p customtests.TestProfile
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	if profiles == nil {
		profiles = []customtests.TestProfile{}
	}
	return profiles, nil
}

func (s *Store) GetTestProfile(id string) (*customtests.TestProfile, error) {
	var p customtests.TestProfile
	err := s.db.QueryRow(`SELECT id, name, created_at, updated_at FROM test_profiles WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`SELECT id, profile_id, type, name, value, extra, sort_order FROM test_rules WHERE profile_id = ? ORDER BY sort_order`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r customtests.TestRule
		if err := rows.Scan(&r.ID, &r.ProfileID, &r.Type, &r.Name, &r.Value, &r.Extra, &r.SortOrder); err != nil {
			return nil, err
		}
		p.Rules = append(p.Rules, r)
	}
	if p.Rules == nil {
		p.Rules = []customtests.TestRule{}
	}
	return &p, nil
}

func (s *Store) CreateTestProfile(name string, rules []customtests.TestRule) (*customtests.TestProfile, error) {
	now := time.Now().UTC()
	p := &customtests.TestProfile{
		ID:        uuid.New().String(),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO test_profiles (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		p.ID, p.Name, p.CreatedAt, p.UpdatedAt); err != nil {
		return nil, fmt.Errorf("inserting test profile: %w", err)
	}

	for i, r := range rules {
		r.ID = uuid.New().String()
		r.ProfileID = p.ID
		r.SortOrder = i
		if _, err := tx.Exec(`INSERT INTO test_rules (id, profile_id, type, name, value, extra, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.ID, r.ProfileID, r.Type, r.Name, r.Value, r.Extra, r.SortOrder); err != nil {
			return nil, fmt.Errorf("inserting test rule: %w", err)
		}
		p.Rules = append(p.Rules, r)
	}
	if p.Rules == nil {
		p.Rules = []customtests.TestRule{}
	}

	return p, tx.Commit()
}

func (s *Store) UpdateTestProfile(id, name string, rules []customtests.TestRule) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE test_profiles SET name = ?, updated_at = ? WHERE id = ?`, name, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("test profile not found")
	}

	if _, err := tx.Exec(`DELETE FROM test_rules WHERE profile_id = ?`, id); err != nil {
		return err
	}

	for i, r := range rules {
		rID := uuid.New().String()
		if _, err := tx.Exec(`INSERT INTO test_rules (id, profile_id, type, name, value, extra, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			rID, id, r.Type, r.Name, r.Value, r.Extra, i); err != nil {
			return fmt.Errorf("inserting test rule: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) DeleteTestProfile(id string) error {
	res, err := s.db.Exec(`DELETE FROM test_profiles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("test profile not found")
	}
	return nil
}
