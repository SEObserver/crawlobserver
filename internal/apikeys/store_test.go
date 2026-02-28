package apikeys

import (
	"strings"
	"testing"
	"time"

	"github.com/SEObserver/crawlobserver/internal/customtests"
	"github.com/SEObserver/crawlobserver/internal/providers"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("newTestStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// --- Projects ---

func TestCreateProject(t *testing.T) {
	s := newTestStore(t)
	p, err := s.CreateProject("my-site")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" || p.Name != "my-site" {
		t.Fatalf("unexpected project: %+v", p)
	}
}

func TestCreateProjectDuplicate(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("dup"); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateProject("dup")
	if err == nil {
		t.Fatal("expected UNIQUE constraint error")
	}
}

func TestListProjectsEmpty(t *testing.T) {
	s := newTestStore(t)
	list, err := s.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(list))
	}
}

func TestListProjectsOrdered(t *testing.T) {
	s := newTestStore(t)
	s.CreateProject("first")
	time.Sleep(10 * time.Millisecond)
	s.CreateProject("second")

	list, err := s.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	// DESC order: second first
	if list[0].Name != "second" || list[1].Name != "first" {
		t.Fatalf("wrong order: %v, %v", list[0].Name, list[1].Name)
	}
}

func TestListProjectsPaginated(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		s.CreateProject("p" + strings.Repeat("x", i))
	}
	list, total, err := s.ListProjectsPaginated(2, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("expected total=5, got %d", total)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 results, got %d", len(list))
	}
}

func TestListProjectsPaginatedSearch(t *testing.T) {
	s := newTestStore(t)
	s.CreateProject("alpha-site")
	s.CreateProject("beta-site")
	s.CreateProject("gamma")

	list, total, err := s.ListProjectsPaginated(10, 0, "site")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("expected total=2, got %d", total)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 results, got %d", len(list))
	}
}

func TestGetProject(t *testing.T) {
	s := newTestStore(t)
	created, _ := s.CreateProject("test")
	got, err := s.GetProject(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "test" {
		t.Fatalf("expected 'test', got %q", got.Name)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetProject("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing project")
	}
}

func TestRenameProject(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("old")
	if err := s.RenameProject(p.ID, "new"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetProject(p.ID)
	if got.Name != "new" {
		t.Fatalf("expected 'new', got %q", got.Name)
	}
}

func TestRenameProjectNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.RenameProject("nonexistent", "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteProject(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("doomed")
	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetProject(p.ID)
	if err == nil {
		t.Fatal("expected project to be gone")
	}
}

func TestDeleteProjectCascade(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("proj")
	_, err := s.CreateAPIKey("key1", "project", &p.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatal(err)
	}
	keys, _ := s.ListAPIKeys()
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys after cascade, got %d", len(keys))
	}
}

func TestDeleteProjectNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteProject("ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- API Keys ---

func TestCreateAPIKeyGeneral(t *testing.T) {
	s := newTestStore(t)
	res, err := s.CreateAPIKey("admin key", "general", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.FullKey, "sk_") {
		t.Fatalf("key should start with sk_, got %q", res.FullKey)
	}
	if len(res.FullKey) != 67 { // sk_ + 64 hex
		t.Fatalf("expected 67 chars, got %d", len(res.FullKey))
	}
	if !strings.HasSuffix(res.KeyPrefix, "...") {
		t.Fatalf("prefix should end with ..., got %q", res.KeyPrefix)
	}
	if res.Type != "general" || res.ProjectID != nil {
		t.Fatalf("unexpected type/project: %s / %v", res.Type, res.ProjectID)
	}
}

func TestCreateAPIKeyProject(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("proj")
	res, err := s.CreateAPIKey("proj key", "project", &p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != "project" || res.ProjectID == nil || *res.ProjectID != p.ID {
		t.Fatalf("unexpected: type=%s pid=%v", res.Type, res.ProjectID)
	}
}

func TestCreateAPIKeyInvalidType(t *testing.T) {
	s := newTestStore(t)
	_, err := s.CreateAPIKey("bad", "invalid", nil)
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestCreateAPIKeyProjectWithoutID(t *testing.T) {
	s := newTestStore(t)
	_, err := s.CreateAPIKey("bad", "project", nil)
	if err == nil {
		t.Fatal("expected error when project type lacks project_id")
	}
}

func TestListAPIKeysEmpty(t *testing.T) {
	s := newTestStore(t)
	keys, err := s.ListAPIKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0, got %d", len(keys))
	}
}

func TestListAPIKeysMultiple(t *testing.T) {
	s := newTestStore(t)
	s.CreateAPIKey("k1", "general", nil)
	s.CreateAPIKey("k2", "general", nil)
	keys, err := s.ListAPIKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2, got %d", len(keys))
	}
}

func TestDeleteAPIKey(t *testing.T) {
	s := newTestStore(t)
	res, _ := s.CreateAPIKey("temp", "general", nil)
	if err := s.DeleteAPIKey(res.ID); err != nil {
		t.Fatal(err)
	}
	keys, _ := s.ListAPIKeys()
	if len(keys) != 0 {
		t.Fatalf("expected 0, got %d", len(keys))
	}
}

func TestDeleteAPIKeyNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteAPIKey("ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Validation ---

func TestValidateKeyGeneral(t *testing.T) {
	s := newTestStore(t)
	res, _ := s.CreateAPIKey("k", "general", nil)
	lookup := s.ValidateKey(res.FullKey)
	if lookup == nil {
		t.Fatal("expected non-nil result")
	}
	if lookup.Type != "general" || lookup.ProjectID != nil {
		t.Fatalf("unexpected: type=%s pid=%v", lookup.Type, lookup.ProjectID)
	}
}

func TestValidateKeyProject(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("proj")
	res, _ := s.CreateAPIKey("k", "project", &p.ID)
	lookup := s.ValidateKey(res.FullKey)
	if lookup == nil {
		t.Fatal("expected non-nil result")
	}
	if lookup.ProjectID == nil || *lookup.ProjectID != p.ID {
		t.Fatal("expected project ID in lookup")
	}
}

func TestValidateKeyInvalid(t *testing.T) {
	s := newTestStore(t)
	if s.ValidateKey("sk_invalid") != nil {
		t.Fatal("expected nil for invalid key")
	}
}

func TestValidateKeyAfterDelete(t *testing.T) {
	s := newTestStore(t)
	res, _ := s.CreateAPIKey("k", "general", nil)
	s.DeleteAPIKey(res.ID)
	if s.ValidateKey(res.FullKey) != nil {
		t.Fatal("expected nil after delete")
	}
}

// --- GSC Connections ---

func TestSaveGSCConnectionInsert(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("proj")
	conn := &GSCConnection{
		ProjectID:    p.ID,
		PropertyURL:  "sc-domain:example.com",
		AccessToken:  "at",
		RefreshToken: "rt",
		TokenExpiry:  time.Now().Add(time.Hour),
	}
	if err := s.SaveGSCConnection(conn); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGSCConnection(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PropertyURL != "sc-domain:example.com" {
		t.Fatalf("unexpected property: %s", got.PropertyURL)
	}
}

func TestSaveGSCConnectionUpsert(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("proj")
	conn := &GSCConnection{
		ProjectID: p.ID, PropertyURL: "old",
		AccessToken: "a", RefreshToken: "r", TokenExpiry: time.Now(),
	}
	s.SaveGSCConnection(conn)

	conn2 := &GSCConnection{
		ProjectID: p.ID, PropertyURL: "new",
		AccessToken: "a2", RefreshToken: "r2", TokenExpiry: time.Now(),
	}
	if err := s.SaveGSCConnection(conn2); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetGSCConnection(p.ID)
	if got.PropertyURL != "new" {
		t.Fatalf("expected 'new', got %q", got.PropertyURL)
	}
}

func TestGetGSCConnectionNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetGSCConnection("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteGSCConnection(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("proj")
	s.SaveGSCConnection(&GSCConnection{
		ProjectID: p.ID, PropertyURL: "x",
		AccessToken: "a", RefreshToken: "r", TokenExpiry: time.Now(),
	})
	if err := s.DeleteGSCConnection(p.ID); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetGSCConnection(p.ID)
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestDeleteGSCConnectionNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteGSCConnection("ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListGSCConnectionsEmpty(t *testing.T) {
	s := newTestStore(t)
	list, err := s.ListGSCConnections()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0, got %d", len(list))
	}
}

func TestListGSCConnectionsMultiple(t *testing.T) {
	s := newTestStore(t)
	p1, _ := s.CreateProject("p1")
	p2, _ := s.CreateProject("p2")
	s.SaveGSCConnection(&GSCConnection{
		ProjectID: p1.ID, PropertyURL: "a",
		AccessToken: "a", RefreshToken: "r", TokenExpiry: time.Now(),
	})
	s.SaveGSCConnection(&GSCConnection{
		ProjectID: p2.ID, PropertyURL: "b",
		AccessToken: "a", RefreshToken: "r", TokenExpiry: time.Now(),
	})
	list, err := s.ListGSCConnections()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

// --- Rulesets ---

func TestCreateRulesetWithRules(t *testing.T) {
	s := newTestStore(t)
	rules := []customtests.TestRule{
		{Type: "contains", Name: "r1", Value: "val1"},
		{Type: "regex", Name: "r2", Value: "val2"},
	}
	rs, err := s.CreateRuleset("test-rs", rules)
	if err != nil {
		t.Fatal(err)
	}
	if rs.ID == "" || rs.Name != "test-rs" {
		t.Fatalf("unexpected: %+v", rs)
	}
	if len(rs.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rs.Rules))
	}
	for i, r := range rs.Rules {
		if r.ID == "" {
			t.Fatalf("rule %d missing ID", i)
		}
		if r.RulesetID != rs.ID {
			t.Fatalf("rule %d wrong RulesetID", i)
		}
		if r.SortOrder != i {
			t.Fatalf("rule %d wrong SortOrder: %d", i, r.SortOrder)
		}
	}
}

func TestCreateRulesetNoRules(t *testing.T) {
	s := newTestStore(t)
	rs, err := s.CreateRuleset("empty", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Rules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(rs.Rules))
	}
}

func TestGetRulesetNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetRuleset("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateRuleset(t *testing.T) {
	s := newTestStore(t)
	rs, _ := s.CreateRuleset("orig", []customtests.TestRule{
		{Type: "contains", Name: "old", Value: "v"},
	})
	newRules := []customtests.TestRule{
		{Type: "regex", Name: "new1", Value: "a"},
		{Type: "regex", Name: "new2", Value: "b"},
	}
	if err := s.UpdateRuleset(rs.ID, "renamed", newRules); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRuleset(rs.ID)
	if got.Name != "renamed" {
		t.Fatalf("expected 'renamed', got %q", got.Name)
	}
	if len(got.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got.Rules))
	}
}

func TestUpdateRulesetNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateRuleset("ghost", "x", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteRuleset(t *testing.T) {
	s := newTestStore(t)
	rs, _ := s.CreateRuleset("del", nil)
	if err := s.DeleteRuleset(rs.ID); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetRuleset(rs.ID)
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestDeleteRulesetNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteRuleset("ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListRulesetsEmpty(t *testing.T) {
	s := newTestStore(t)
	list, err := s.ListRulesets()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0, got %d", len(list))
	}
}

// --- Provider Connections ---

func TestSaveProviderConnectionInsert(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("proj")
	conn := &providers.ProviderConnection{
		ProjectID: p.ID,
		Provider:  "seobserver",
		Domain:    "example.com",
		APIKey:    "secret",
	}
	if err := s.SaveProviderConnection(conn); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProviderConnection(p.ID, "seobserver")
	if err != nil {
		t.Fatal(err)
	}
	if got.Domain != "example.com" {
		t.Fatalf("unexpected domain: %s", got.Domain)
	}
}

func TestSaveProviderConnectionUpsert(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("proj")
	s.SaveProviderConnection(&providers.ProviderConnection{
		ProjectID: p.ID, Provider: "seobserver", Domain: "old.com", APIKey: "k",
	})
	s.SaveProviderConnection(&providers.ProviderConnection{
		ProjectID: p.ID, Provider: "seobserver", Domain: "new.com", APIKey: "k2",
	})
	got, _ := s.GetProviderConnection(p.ID, "seobserver")
	if got.Domain != "new.com" {
		t.Fatalf("expected 'new.com', got %q", got.Domain)
	}
}

func TestGetProviderConnectionNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetProviderConnection("x", "y")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteProviderConnection(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("proj")
	s.SaveProviderConnection(&providers.ProviderConnection{
		ProjectID: p.ID, Provider: "seobserver", Domain: "d", APIKey: "k",
	})
	if err := s.DeleteProviderConnection(p.ID, "seobserver"); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetProviderConnection(p.ID, "seobserver")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestDeleteProviderConnectionNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteProviderConnection("x", "y")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListProviderConnectionsEmpty(t *testing.T) {
	s := newTestStore(t)
	list, err := s.ListProviderConnections("proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0, got %d", len(list))
	}
}

func TestListProviderConnectionsFiltered(t *testing.T) {
	s := newTestStore(t)
	p1, _ := s.CreateProject("p1")
	p2, _ := s.CreateProject("p2")
	s.SaveProviderConnection(&providers.ProviderConnection{
		ProjectID: p1.ID, Provider: "a", Domain: "d", APIKey: "k",
	})
	s.SaveProviderConnection(&providers.ProviderConnection{
		ProjectID: p2.ID, Provider: "b", Domain: "d", APIKey: "k",
	})
	list, _ := s.ListProviderConnections(p1.ID)
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if list[0].ProjectID != p1.ID {
		t.Fatal("wrong project in results")
	}
}
