package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// seedProviderTenancyFixture builds two teams with one leader each, one
// platform-owned provider, and one provider owned by team B. It returns the
// HTTP app plus login tokens for the admin, leader A, and leader B.
func seedProviderTenancyFixture(t *testing.T) (http.Handler, string, string, string, Provider, Provider) {
	t.Helper()
	store := NewMemoryStore()
	store.CreateResource("teams", AdminResource{ID: "team_a", Name: "Team A", Status: StatusActive})
	store.CreateResource("teams", AdminResource{ID: "team_b", Name: "Team B", Status: StatusActive})
	if _, err := store.CreateAdminUser(AdminUser{
		Username: "platform-admin", Name: "Platform Admin", Email: "admin@tokenhub.local",
		Role: "admin", Status: StatusActive,
	}, "admin123456"); err != nil {
		t.Fatal(err)
	}
	leaderA, err := store.CreateAdminUser(AdminUser{
		Username: "leader-a", Name: "Leader A", Email: "leader-a@tokenhub.local",
		Role: "team_leader", TeamID: "team_a", Status: StatusActive,
	}, "leader123456")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAdminUser(AdminUser{
		Username: "leader-b", Name: "Leader B", Email: "leader-b@tokenhub.local",
		Role: "team_leader", TeamID: "team_b", Status: StatusActive,
	}, "leader123456"); err != nil {
		t.Fatal(err)
	}
	platformProvider := store.AddProvider(Provider{ID: "prv_platform", Name: "Platform Provider", Type: "mock", Priority: 10})
	teamBProvider := store.AddProvider(Provider{ID: "prv_team_b", Name: "Team B Provider", Type: "mock", OwnerTeamID: "team_b", Priority: 10})

	app := New(store).Handler()
	adminToken := providerTenancyLoginToken(t, app, "admin@tokenhub.local", "admin123456")
	tokenA := providerTenancyLoginToken(t, app, "leader-a@tokenhub.local", "leader123456")
	tokenB := providerTenancyLoginToken(t, app, "leader-b@tokenhub.local", "leader123456")
	_ = leaderA
	return app, adminToken, tokenA, tokenB, platformProvider, teamBProvider
}

func providerTenancyLoginToken(t *testing.T, app http.Handler, identity string, password string) string {
	t.Helper()
	response := doJSON(t, app, http.MethodPost, "/api/admin/auth/login", map[string]any{
		"identity": identity,
		"password": password,
	}, "")
	if response.Code != http.StatusOK {
		t.Fatalf("login as %s failed: %d %s", identity, response.Code, response.Body)
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(response.Body), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Token
}

func decodeProviderCreateResult(t *testing.T, body string) Provider {
	t.Helper()
	var result ProviderCreateResult
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	return result.Provider
}

func TestTeamLeaderProviderManagementIsTeamScoped(t *testing.T) {
	app, adminToken, tokenA, _, platformProvider, teamBProvider := seedProviderTenancyFixture(t)

	created := doJSON(t, app, http.MethodPost, "/api/admin/providers", map[string]any{
		"name": "Teacher Provider",
		"type": "mock",
	}, tokenA)
	if created.Code != http.StatusCreated {
		t.Fatalf("team leader provider create failed: %d %s", created.Code, created.Body)
	}
	provider := decodeProviderCreateResult(t, created.Body)
	if provider.OwnerTeamID != "team_a" {
		t.Fatalf("team leader provider should be owned by own team, got %q", provider.OwnerTeamID)
	}

	listed := doJSON(t, app, http.MethodGet, "/api/admin/providers", nil, tokenA)
	if listed.Code != http.StatusOK {
		t.Fatalf("team leader provider list failed: %d %s", listed.Code, listed.Body)
	}
	if !strings.Contains(listed.Body, provider.ID) {
		t.Fatalf("team leader should see own provider: %s", listed.Body)
	}
	if strings.Contains(listed.Body, platformProvider.ID) || strings.Contains(listed.Body, teamBProvider.ID) {
		t.Fatalf("team leader should not see platform or foreign providers: %s", listed.Body)
	}

	patched := doJSON(t, app, http.MethodPatch, "/api/admin/providers/"+provider.ID, map[string]any{
		"name": "Renamed Teacher Provider",
	}, tokenA)
	if patched.Code != http.StatusOK {
		t.Fatalf("team leader should patch own provider: %d %s", patched.Code, patched.Body)
	}

	for _, target := range []string{platformProvider.ID, teamBProvider.ID} {
		blocked := doJSON(t, app, http.MethodPatch, "/api/admin/providers/"+target, map[string]any{
			"name": "Hijacked",
		}, tokenA)
		if blocked.Code != http.StatusForbidden {
			t.Fatalf("team leader patch of %s should be forbidden, got %d %s", target, blocked.Code, blocked.Body)
		}
	}

	deleted := doJSON(t, app, http.MethodDelete, "/api/admin/providers/"+platformProvider.ID, nil, tokenA)
	if deleted.Code != http.StatusForbidden {
		t.Fatalf("team leader delete of platform provider should be forbidden, got %d %s", deleted.Code, deleted.Body)
	}

	adminList := doJSON(t, app, http.MethodGet, "/api/admin/providers", nil, adminToken)
	for _, id := range []string{platformProvider.ID, teamBProvider.ID, provider.ID} {
		if !strings.Contains(adminList.Body, id) {
			t.Fatalf("admin should see provider %s: %s", id, adminList.Body)
		}
	}
}

func TestTeamLeaderProviderCreateCannotReuseExistingID(t *testing.T) {
	app, _, tokenA, _, platformProvider, _ := seedProviderTenancyFixture(t)

	response := doJSON(t, app, http.MethodPost, "/api/admin/providers", map[string]any{
		"id":   platformProvider.ID,
		"name": "Impersonating Provider",
		"type": "mock",
	}, tokenA)
	if response.Code != http.StatusConflict {
		t.Fatalf("team leader create with an existing provider ID should conflict, got %d %s", response.Code, response.Body)
	}
}

func TestTeamLeaderProviderCreateIgnoresForeignOwnerTeam(t *testing.T) {
	app, _, tokenA, _, _, _ := seedProviderTenancyFixture(t)

	created := doJSON(t, app, http.MethodPost, "/api/admin/providers", map[string]any{
		"name":          "Claimed Provider",
		"type":          "mock",
		"owner_team_id": "team_b",
	}, tokenA)
	if created.Code != http.StatusCreated {
		t.Fatalf("provider create failed: %d %s", created.Code, created.Body)
	}
	provider := decodeProviderCreateResult(t, created.Body)
	if provider.OwnerTeamID != "team_a" {
		t.Fatalf("team leader provider owner must be forced to own team, got %q", provider.OwnerTeamID)
	}
}

func TestAdminCanAssignProviderOwnerTeam(t *testing.T) {
	app, adminToken, _, _, _, _ := seedProviderTenancyFixture(t)

	assigned := doJSON(t, app, http.MethodPost, "/api/admin/providers", map[string]any{
		"name":          "Assigned Provider",
		"type":          "mock",
		"owner_team_id": "team_b",
	}, adminToken)
	if assigned.Code != http.StatusCreated {
		t.Fatalf("admin assign provider to team failed: %d %s", assigned.Code, assigned.Body)
	}
	provider := decodeProviderCreateResult(t, assigned.Body)
	if provider.OwnerTeamID != "team_b" {
		t.Fatalf("admin provider should be assigned to requested team, got %q", provider.OwnerTeamID)
	}

	invalid := doJSON(t, app, http.MethodPost, "/api/admin/providers", map[string]any{
		"name":          "Missing Team Provider",
		"type":          "mock",
		"owner_team_id": "team_missing",
	}, adminToken)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("admin provider create with unknown team should fail, got %d %s", invalid.Code, invalid.Body)
	}
}

func TestTeamLeaderProviderResourcesAreTeamScoped(t *testing.T) {
	app, _, tokenA, tokenB, platformProvider, teamBProvider := seedProviderTenancyFixture(t)

	createdProvider := doJSON(t, app, http.MethodPost, "/api/admin/providers", map[string]any{
		"name": "Teacher Provider",
		"type": "mock",
	}, tokenA)
	if createdProvider.Code != http.StatusCreated {
		t.Fatalf("team leader provider create failed: %d %s", createdProvider.Code, createdProvider.Body)
	}
	own := decodeProviderCreateResult(t, createdProvider.Body)

	created := doJSON(t, app, http.MethodPost, "/api/admin/provider-resources", map[string]any{
		"provider_id": own.ID,
		"name":        "Teacher Key",
	}, tokenA)
	if created.Code != http.StatusCreated {
		t.Fatalf("team leader resource create on own provider failed: %d %s", created.Code, created.Body)
	}
	var ownResource ProviderResource
	if err := json.Unmarshal([]byte(created.Body), &ownResource); err != nil {
		t.Fatal(err)
	}

	for _, providerID := range []string{platformProvider.ID, teamBProvider.ID} {
		blocked := doJSON(t, app, http.MethodPost, "/api/admin/provider-resources", map[string]any{
			"provider_id": providerID,
			"name":        "Foreign Key",
		}, tokenA)
		if blocked.Code != http.StatusForbidden {
			t.Fatalf("team leader resource create on %s should be forbidden, got %d %s", providerID, blocked.Code, blocked.Body)
		}
	}

	listed := doJSON(t, app, http.MethodGet, "/api/admin/provider-resources", nil, tokenA)
	if !strings.Contains(listed.Body, ownResource.ID) {
		t.Fatalf("team leader should see own resources: %s", listed.Body)
	}
	if strings.Contains(listed.Body, teamBProvider.ID) {
		t.Fatalf("team leader should not see foreign provider resources: %s", listed.Body)
	}

	foreignResource := doJSON(t, app, http.MethodPost, "/api/admin/provider-resources", map[string]any{
		"provider_id": teamBProvider.ID,
		"name":        "Team B Key",
	}, tokenB)
	var teamBResource ProviderResource
	if err := json.Unmarshal([]byte(foreignResource.Body), &teamBResource); err != nil {
		t.Fatal(err)
	}

	bulkMixed := doJSON(t, app, http.MethodPost, "/api/admin/provider-resources/bulk", map[string]any{
		"action": "enable",
		"ids":    []string{ownResource.ID, teamBResource.ID},
	}, tokenA)
	if bulkMixed.Code != http.StatusForbidden {
		t.Fatalf("bulk action mixing foreign resources should be forbidden, got %d %s", bulkMixed.Code, bulkMixed.Body)
	}

	bulkOwn := doJSON(t, app, http.MethodPost, "/api/admin/provider-resources/bulk", map[string]any{
		"action": "disable",
		"ids":    []string{ownResource.ID},
	}, tokenA)
	if bulkOwn.Code != http.StatusOK {
		t.Fatalf("bulk action on own resources failed: %d %s", bulkOwn.Code, bulkOwn.Body)
	}

	importForeign := doJSON(t, app, http.MethodPost, "/api/admin/provider-resources/import", map[string]any{
		"resources": []map[string]any{
			{"provider_id": platformProvider.ID, "name": "Imported Key"},
		},
	}, tokenA)
	if importForeign.Code != http.StatusForbidden {
		t.Fatalf("import targeting a foreign provider should be forbidden, got %d %s", importForeign.Code, importForeign.Body)
	}

	patchForeign := doJSON(t, app, http.MethodPatch, "/api/admin/provider-resources/"+teamBResource.ID, map[string]any{
		"name": "Hijacked Key",
	}, tokenA)
	if patchForeign.Code != http.StatusForbidden {
		t.Fatalf("patch of a foreign resource should be forbidden, got %d %s", patchForeign.Code, patchForeign.Body)
	}

	deleteForeign := doJSON(t, app, http.MethodDelete, "/api/admin/provider-resources/"+teamBResource.ID, nil, tokenA)
	if deleteForeign.Code != http.StatusForbidden {
		t.Fatalf("delete of a foreign resource should be forbidden, got %d %s", deleteForeign.Code, deleteForeign.Body)
	}
}

func TestTeamLeaderProviderCreateRejectsLoopbackBaseURL(t *testing.T) {
	// The package TestMain opts the whole suite into loopback upstreams; this
	// SSRF regression must run with the production default-deny.
	t.Setenv("TOKENHUB_PROVIDER_UPSTREAM_ALLOW_LOOPBACK", "")
	t.Setenv("TOKENHUB_PROVIDER_UPSTREAM_ACCESS_MODE", "strict")
	app, _, tokenA, _, _, _ := seedProviderTenancyFixture(t)

	response := doJSON(t, app, http.MethodPost, "/api/admin/providers", map[string]any{
		"name":     "Loopback Provider",
		"type":     "mock",
		"base_url": "http://127.0.0.1:9/v1",
	}, tokenA)
	if response.Code == http.StatusCreated {
		t.Fatalf("team leader provider create with a loopback base URL must be rejected: %s", response.Body)
	}
	if !strings.Contains(response.Body, "not_be_a_loopback") && !strings.Contains(response.Body, "provider_base_url_not_allowed") {
		t.Fatalf("loopback rejection should surface the upstream access error: %s", response.Body)
	}
}

func TestProviderEgressTestStaysPlatformAdminOnly(t *testing.T) {
	app, adminToken, tokenA, _, platformProvider, _ := seedProviderTenancyFixture(t)

	blocked := doJSON(t, app, http.MethodPost, "/api/admin/provider-egress/test", map[string]any{
		"provider_id": platformProvider.ID,
	}, tokenA)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("team leader egress test should be forbidden, got %d %s", blocked.Code, blocked.Body)
	}

	allowed := doJSON(t, app, http.MethodPost, "/api/admin/provider-egress/test", map[string]any{
		"provider_id": platformProvider.ID,
	}, adminToken)
	if allowed.Code == http.StatusForbidden {
		t.Fatalf("admin egress test should not be forbidden: %d %s", allowed.Code, allowed.Body)
	}
}
