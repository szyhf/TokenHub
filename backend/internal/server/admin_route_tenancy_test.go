package server

import (
	"net/http"
	"strings"
	"testing"
)

// seedRouteTenancyFixture builds two teams with leaders, a team-A provider,
// a platform provider, and matching provider models for both.
func seedRouteTenancyFixture(t *testing.T) (*GormStore, http.Handler, string, string, Provider, Provider, ModelRoute, ModelRoute) {
	t.Helper()
	store := NewMemoryStore()
	store.CreateResource("teams", AdminResource{ID: "team_a", Name: "Team A", Status: StatusActive})
	store.CreateResource("teams", AdminResource{ID: "team_b", Name: "Team B", Status: StatusActive})
	if _, err := store.CreateAdminUser(AdminUser{
		Username: "route-admin", Name: "Route Admin", Email: "route-admin@tokenhub.local",
		Role: "admin", Status: StatusActive,
	}, "admin123456"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAdminUser(AdminUser{
		Username: "route-leader-a", Name: "Leader A", Email: "route-leader-a@tokenhub.local",
		Role: "team_leader", TeamID: "team_a", Status: StatusActive,
	}, "leader123456"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAdminUser(AdminUser{
		Username: "route-leader-b", Name: "Leader B", Email: "route-leader-b@tokenhub.local",
		Role: "team_leader", TeamID: "team_b", Status: StatusActive,
	}, "leader123456"); err != nil {
		t.Fatal(err)
	}
	teamProvider := store.AddProvider(Provider{ID: "prv_team_a", Name: "Team A Provider", Type: ProviderMock, OwnerTeamID: "team_a", Priority: 10})
	platformProvider := store.AddProvider(Provider{ID: "prv_platform", Name: "Platform Provider", Type: ProviderMock, Priority: 10})
	store.AddModel(Model{Name: "shared-model", Modality: "chat", Status: StatusActive})
	store.AddModel(Model{Name: "teacher-model", Modality: "chat", Status: StatusActive})
	store.AddProviderModel(ProviderModel{ProviderID: teamProvider.ID, UpstreamModel: "upstream-model"})
	store.AddProviderModel(ProviderModel{ProviderID: platformProvider.ID, UpstreamModel: "upstream-model"})
	teamRoute := store.AddRoute(ModelRoute{
		ID: "route_team_a", ModelName: "shared-model", ProviderID: teamProvider.ID,
		ProviderModel: "upstream-model", Priority: 1, Weight: 100, Status: StatusActive,
	})
	platformRoute := store.AddRoute(ModelRoute{
		ID: "route_platform", ModelName: "shared-model", ProviderID: platformProvider.ID,
		ProviderModel: "upstream-model", Priority: 2, Weight: 100, Status: StatusActive,
	})

	app := New(store).Handler()
	adminToken := providerTenancyLoginToken(t, app, "route-admin@tokenhub.local", "admin123456")
	tokenA := providerTenancyLoginToken(t, app, "route-leader-a@tokenhub.local", "leader123456")
	return store, app, adminToken, tokenA, teamProvider, platformProvider, teamRoute, platformRoute
}

func TestTeamOwnedProviderRoutesAreTeamScoped(t *testing.T) {
	server := New(NewMemoryStore())
	teamAProject := Project{ID: "prj_team_a", TeamID: "team_a", Status: StatusActive}
	teamBProject := Project{ID: "prj_team_b", TeamID: "team_b", Status: StatusActive}
	unteamedProject := Project{ID: "prj_free", Status: StatusActive}
	routes := []RouteSelection{
		{
			Provider: Provider{ID: "prv_team_a", OwnerTeamID: "team_a"},
			Route:    ModelRoute{ID: "route_team_a", Priority: 1, Weight: 100},
		},
		{
			Provider: Provider{ID: "prv_platform"},
			Route:    ModelRoute{ID: "route_platform", Priority: 2, Weight: 100},
		},
	}

	planA := server.planRouteOrder(CallContext{RequestID: "req_a", Project: teamAProject}, routes)
	if len(planA) != 2 || planA[0].Provider.ID != "prv_team_a" {
		t.Fatalf("team A project should see own and platform routes in order: %+v", planA)
	}
	planB := server.planRouteOrder(CallContext{RequestID: "req_b", Project: teamBProject}, routes)
	if len(planB) != 1 || planB[0].Provider.ID != "prv_platform" {
		t.Fatalf("team B project must only see the platform route: %+v", planB)
	}
	planFree := server.planRouteOrder(CallContext{RequestID: "req_free", Project: unteamedProject}, routes)
	if len(planFree) != 1 || planFree[0].Provider.ID != "prv_platform" {
		t.Fatalf("unteamed project must only see the platform route: %+v", planFree)
	}
}

func TestTeamLeaderRouteManagementIsTeamScoped(t *testing.T) {
	_, app, _, tokenA, _, platformProvider, teamRoute, platformRoute := seedRouteTenancyFixture(t)

	created := doJSON(t, app, http.MethodPost, "/api/admin/routing-rules", map[string]any{
		"model_name":     "teacher-model",
		"provider_id":    "prv_team_a",
		"provider_model": "upstream-model",
		"priority":       3,
		"weight":         100,
	}, tokenA)
	if created.Code != http.StatusCreated {
		t.Fatalf("team leader route create on own provider failed: %d %s", created.Code, created.Body)
	}

	foreign := doJSON(t, app, http.MethodPost, "/api/admin/routing-rules", map[string]any{
		"model_name":     "shared-model",
		"provider_id":    platformProvider.ID,
		"provider_model": "upstream-model",
		"priority":       4,
		"weight":         100,
	}, tokenA)
	if foreign.Code != http.StatusForbidden {
		t.Fatalf("team leader route create on platform provider should be forbidden, got %d %s", foreign.Code, foreign.Body)
	}

	listed := doJSON(t, app, http.MethodGet, "/api/admin/routing-rules", nil, tokenA)
	if !strings.Contains(listed.Body, teamRoute.ID) {
		t.Fatalf("team leader should see own provider routes: %s", listed.Body)
	}
	if strings.Contains(listed.Body, platformRoute.ID) {
		t.Fatalf("team leader must not see platform provider routes: %s", listed.Body)
	}

	patched := doJSON(t, app, http.MethodPatch, "/api/admin/routing-rules/"+teamRoute.ID, map[string]any{
		"priority": 5,
	}, tokenA)
	if patched.Code != http.StatusOK {
		t.Fatalf("team leader should patch own provider route: %d %s", patched.Code, patched.Body)
	}

	blockedPatch := doJSON(t, app, http.MethodPatch, "/api/admin/routing-rules/"+platformRoute.ID, map[string]any{
		"priority": 6,
	}, tokenA)
	if blockedPatch.Code != http.StatusForbidden {
		t.Fatalf("team leader patch of platform route should be forbidden, got %d %s", blockedPatch.Code, blockedPatch.Body)
	}

	blockedDelete := doJSON(t, app, http.MethodDelete, "/api/admin/routing-rules/"+platformRoute.ID, nil, tokenA)
	if blockedDelete.Code != http.StatusForbidden {
		t.Fatalf("team leader delete of platform route should be forbidden, got %d %s", blockedDelete.Code, blockedDelete.Body)
	}
}

func TestTeamLeaderCannotReachGlobalRoutingSurfaces(t *testing.T) {
	store, app, _, tokenA, _, _, _, _ := seedRouteTenancyFixture(t)

	modelPolicy := doJSON(t, app, http.MethodPatch, "/api/admin/model-routing-policies/shared-model", map[string]any{
		"strategy": "priority",
	}, tokenA)
	if modelPolicy.Code != http.StatusForbidden {
		t.Fatalf("team leader model routing policy patch should be forbidden, got %d %s", modelPolicy.Code, modelPolicy.Body)
	}

	policyCreate := doJSON(t, app, http.MethodPost, "/api/admin/resources/routing-policies", map[string]any{
		"name":     "Teacher Global Policy",
		"scope":    "global",
		"scope_id": "global",
		"strategy": "priority",
	}, tokenA)
	if policyCreate.Code != http.StatusForbidden {
		t.Fatalf("team leader routing policy create should be forbidden, got %d %s", policyCreate.Code, policyCreate.Body)
	}

	simProject := store.CreateProject(Project{Name: "Foreign Project", TeamID: "team_b", Status: StatusActive})
	simKey, _, err := store.CreateAPIKey(simProject.ID, APIKey{Name: "Foreign Key", Status: StatusActive}, "thk_foreign_sim")
	if err != nil {
		t.Fatal(err)
	}
	simulation := doJSON(t, app, http.MethodPost, "/api/admin/routing-policies/simulate", map[string]any{
		"project_id": simProject.ID,
		"api_key_id": simKey.ID,
		"model":      "shared-model",
	}, tokenA)
	if simulation.Code != http.StatusForbidden {
		t.Fatalf("team leader simulation on a foreign team project should be forbidden, got %d %s", simulation.Code, simulation.Body)
	}
}

func TestGatewayServesTeamProviderOnlyToTeamProjects(t *testing.T) {
	store := NewMemoryStore()
	teamProvider := store.AddProvider(Provider{ID: "prv_iso_team", Name: "Team", Type: ProviderMock, OwnerTeamID: "team_iso", Priority: 10})
	platformProvider := store.AddProvider(Provider{ID: "prv_iso_platform", Name: "Platform", Type: ProviderMock, Priority: 10})
	store.AddProviderModel(ProviderModel{ProviderID: teamProvider.ID, UpstreamModel: "iso-model"})
	store.AddProviderModel(ProviderModel{ProviderID: platformProvider.ID, UpstreamModel: "iso-model"})
	store.AddRoute(ModelRoute{ID: "route_iso_team", ModelName: "iso-model", ProviderID: teamProvider.ID, ProviderModel: "iso-model", Priority: 1, Weight: 100, Status: StatusActive})
	store.AddRoute(ModelRoute{ID: "route_iso_platform", ModelName: "iso-model", ProviderID: platformProvider.ID, ProviderModel: "iso-model", Priority: 2, Weight: 100, Status: StatusActive})

	teamCandidates, err := store.SelectRouteCandidates("iso-model")
	if err != nil {
		t.Fatal(err)
	}
	server := New(store)
	teamProject := Project{ID: "prj_iso_team", TeamID: "team_iso", Status: StatusActive}
	otherProject := Project{ID: "prj_iso_other", TeamID: "team_other", Status: StatusActive}

	teamPlan := server.planRouteOrder(CallContext{RequestID: "req_iso_team", Project: teamProject}, teamCandidates)
	if len(teamPlan) != 2 || teamPlan[0].Provider.ID != teamProvider.ID {
		t.Fatalf("team project should prefer and include its own provider route: %+v", teamPlan)
	}
	otherPlan := server.planRouteOrder(CallContext{RequestID: "req_iso_other", Project: otherProject}, teamCandidates)
	if len(otherPlan) != 1 || otherPlan[0].Provider.ID != platformProvider.ID {
		t.Fatalf("foreign project must not use the team provider route: %+v", otherPlan)
	}
}
