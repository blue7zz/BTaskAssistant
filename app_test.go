package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/blue7zz/BTaskAssistant/internal/credentials"
)

type memoryCredentialStore struct {
	secrets map[string]string
}

func (s *memoryCredentialStore) Set(account string, secret string) error {
	s.secrets[account] = secret
	return nil
}

func (s *memoryCredentialStore) Get(account string) (string, error) {
	secret, ok := s.secrets[account]
	if !ok {
		return "", credentials.ErrNotFound
	}
	return secret, nil
}

func (s *memoryCredentialStore) Delete(account string) error {
	delete(s.secrets, account)
	return nil
}

func TestSetupPlaneConnectionDiscoversProjectsAndStoresToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/api/v1/workspaces/team/projects/" {
				t.Fatalf("unexpected request path %q", request.URL.Path)
			}
			if request.Header.Get("X-API-Key") != "plane_api_test" {
				t.Fatal("missing Plane API key")
			}
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"results": []map[string]string{{
					"id":         "project-1",
					"name":       "Team project",
					"identifier": "TEAM",
				}},
			})
		},
	))
	defer server.Close()

	store := &memoryCredentialStore{secrets: map[string]string{}}
	app := &App{credentials: store}
	setup, err := app.SetupPlaneConnection(
		server.URL+"/team/projects/project-1/",
		"plane_api_test",
	)
	if err != nil {
		t.Fatalf("setup Plane connection: %v", err)
	}
	if setup.BaseURL != server.URL || setup.WorkspaceSlug != "team" {
		t.Fatalf("unexpected setup result %#v", setup)
	}
	if len(setup.Projects) != 1 || setup.Projects[0].ID != "project-1" {
		t.Fatalf("unexpected projects %#v", setup.Projects)
	}
	account, err := planeCredentialAccount(server.URL)
	if err != nil {
		t.Fatalf("credential account: %v", err)
	}
	if store.secrets[account] != "plane_api_test" {
		t.Fatal("token was not stored under the instance account")
	}
}

func TestHasPlaneTokenMigratesLegacyCredential(t *testing.T) {
	store := &memoryCredentialStore{secrets: map[string]string{}}
	app := &App{credentials: store}
	legacyAccount, err := legacyPlaneCredentialAccount(
		"https://plane.example.com",
		"team",
	)
	if err != nil {
		t.Fatalf("legacy credential account: %v", err)
	}
	store.secrets[legacyAccount] = "plane_api_legacy"

	found, err := app.HasPlaneToken("https://plane.example.com", "team")
	if err != nil {
		t.Fatalf("has Plane token: %v", err)
	}
	if !found {
		t.Fatal("expected the legacy token to be found")
	}
	account, err := planeCredentialAccount("https://plane.example.com")
	if err != nil {
		t.Fatalf("credential account: %v", err)
	}
	if store.secrets[account] != "plane_api_legacy" {
		t.Fatal("legacy token was not copied to the instance account")
	}
}
