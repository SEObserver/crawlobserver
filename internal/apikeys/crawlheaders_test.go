package apikeys

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectCrawlHeaders_RoundTrip(t *testing.T) {
	s := newTestStore(t)

	p, err := s.CreateProject("client")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if got, err := s.ProjectCrawlHeaders(p.ID); err != nil || len(got) != 0 {
		t.Errorf("a new project has headers %v (err %v), want none", got, err)
	}

	headers := map[string]string{
		"Signature-Agent": `"https://example.com/.well-known/http-message-signatures-directory"`,
		"Signature-Input": `sig1=("@authority");created=1757000000`,
	}
	if err := s.SetProjectCrawlHeaders(p.ID, headers); err != nil {
		t.Fatalf("SetProjectCrawlHeaders: %v", err)
	}

	got, err := s.ProjectCrawlHeaders(p.ID)
	if err != nil {
		t.Fatalf("ProjectCrawlHeaders: %v", err)
	}
	for name, want := range headers {
		if got[name] != want {
			t.Errorf("header %s = %q, want %q", name, got[name], want)
		}
	}

}

// The project listings are reachable by any key that can reach the API, so a
// header — which can hold a credential — must not travel with a project. This
// checks the serialised form, not the struct: a field added back with a json
// tag would pass a field-level check and still ship the value.
func TestProjectJSON_CarriesNoCrawlHeaders(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("client")

	secret := "Bearer a-real-credential"
	if err := s.SetProjectCrawlHeaders(p.ID, map[string]string{"Authorization": secret}); err != nil {
		t.Fatalf("SetProjectCrawlHeaders: %v", err)
	}

	listed, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	paged, _, err := s.ListProjectsPaginated(30, 0, "")
	if err != nil {
		t.Fatalf("ListProjectsPaginated: %v", err)
	}
	fetched, err := s.GetProject(p.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}

	for name, payload := range map[string]any{
		"ListProjects":          listed,
		"ListProjectsPaginated": paged,
		"GetProject":            fetched,
	} {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshalling %s: %v", name, err)
		}
		for _, unwanted := range []string{secret, "Authorization", "crawl_headers"} {
			if strings.Contains(string(encoded), unwanted) {
				t.Errorf("%s serialises %q:\n%s", name, unwanted, encoded)
			}
		}
	}

	// The headers are still readable on their own, for the guarded endpoint.
	got, err := s.ProjectCrawlHeaders(p.ID)
	if err != nil {
		t.Fatalf("ProjectCrawlHeaders: %v", err)
	}
	if got["Authorization"] != secret {
		t.Errorf("ProjectCrawlHeaders = %v, want the stored header", got)
	}
}

func TestSetProjectCrawlHeaders_EmptyRemovesThem(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("client")

	if err := s.SetProjectCrawlHeaders(p.ID, map[string]string{"Signature": "sig1=::"}); err != nil {
		t.Fatalf("SetProjectCrawlHeaders: %v", err)
	}
	if err := s.SetProjectCrawlHeaders(p.ID, map[string]string{}); err != nil {
		t.Fatalf("SetProjectCrawlHeaders(empty): %v", err)
	}

	got, _ := s.ProjectCrawlHeaders(p.ID)
	if len(got) != 0 {
		t.Errorf("headers = %v, want none after clearing", got)
	}
}

func TestSetProjectCrawlHeaders_UnknownProject(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetProjectCrawlHeaders("no-such-project", map[string]string{"X-A": "1"}); err == nil {
		t.Error("SetProjectCrawlHeaders on an unknown project = nil, want an error")
	}
}

// A crawl whose project was deleted between its queueing and its start must
// still run, without headers, rather than fail to start.
func TestProjectCrawlHeaders_UnknownProjectIsEmpty(t *testing.T) {
	s := newTestStore(t)

	got, err := s.ProjectCrawlHeaders("no-such-project")
	if err != nil {
		t.Errorf("ProjectCrawlHeaders(unknown) error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("headers = %v, want none", got)
	}

	if got, _ = s.ProjectCrawlHeaders(""); len(got) != 0 {
		t.Errorf("headers for the empty project id = %v, want none", got)
	}
}

func TestDecodeCrawlHeaders(t *testing.T) {
	tests := []struct {
		name   string
		stored string
		want   int
	}{
		{"a row written before the column existed", "", 0},
		{"an empty object", "{}", 0},
		{"unreadable JSON", "{not json", 0},
		{"JSON null", "null", 0},
		{"a JSON value that is not an object", `"a string"`, 0},
		{"two headers", `{"Signature":"a","Signature-Agent":"b"}`, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeCrawlHeaders(tt.stored)
			if got == nil {
				t.Fatal("decodeCrawlHeaders returned nil, want an empty map")
			}
			if len(got) != tt.want {
				t.Errorf("decodeCrawlHeaders(%q) has %d headers, want %d", tt.stored, len(got), tt.want)
			}
		})
	}
}
