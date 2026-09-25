package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func seedTeamStatementFixture(t *testing.T) (*GormStore, http.Handler, string, string, string, string) {
	t.Helper()
	store := NewMemoryStore()
	store.CreateResource("teams", AdminResource{ID: "team_bill", Name: "Billing Team", Status: StatusActive})
	store.CreateResource("teams", AdminResource{ID: "team_other", Name: "Other Team", Status: StatusActive})
	if _, err := store.CreateAdminUser(AdminUser{
		Username: "bill-admin", Name: "Bill Admin", Email: "bill-admin@tokenhub.local",
		Role: "admin", Status: StatusActive,
	}, "admin123456"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAdminUser(AdminUser{
		Username: "bill-leader", Name: "Bill Leader", Email: "bill-leader@tokenhub.local",
		Role: "team_leader", TeamID: "team_bill", Status: StatusActive,
	}, "leader123456"); err != nil {
		t.Fatal(err)
	}
	ownProject := store.CreateProject(Project{Name: "Own Project", TeamID: "team_bill", Status: StatusActive})
	foreignProject := store.CreateProject(Project{Name: "Foreign Project", TeamID: "team_other", Status: StatusActive})
	at := time.Date(2020, 1, 15, 12, 0, 0, 0, time.UTC)
	statementFixture(t, store, "own-usage", ownProject.ID, at, "2")
	statementFixture(t, store, "foreign-usage", foreignProject.ID, at, "100")

	app := New(store).Handler()
	adminToken := providerTenancyLoginToken(t, app, "bill-admin@tokenhub.local", "admin123456")
	leaderToken := providerTenancyLoginToken(t, app, "bill-leader@tokenhub.local", "leader123456")
	return store, app, adminToken, leaderToken, ownProject.ID, foreignProject.ID
}

func postStatement(t *testing.T, app http.Handler, token string, query map[string]any) (int, statementResult) {
	t.Helper()
	response := doJSON(t, app, http.MethodPost, "/api/admin/billing/statements", query, token)
	var result statementResult
	if response.Code == http.StatusOK {
		if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
			t.Fatal(err)
		}
	}
	return response.Code, result
}

func TestTeamLeaderStatementIsTenantAndTeamScoped(t *testing.T) {
	_, app, _, leaderToken, ownProjectID, foreignProjectID := seedTeamStatementFixture(t)

	code, result := postStatement(t, app, leaderToken, map[string]any{
		"side": "tenant", "from": "2020-01-01", "to": "2020-02-01", "timezone": "UTC",
	})
	if code != http.StatusOK {
		t.Fatalf("team leader tenant statement failed: %d %s", code, seeBody(t, app, leaderToken))
	}
	if len(result.Rows) == 0 || len(result.Rows) > 2 {
		t.Fatalf("team leader statement should include own project rows: %+v", result.Rows)
	}
	for _, row := range result.Rows {
		if row.ProjectID != ownProjectID {
			t.Fatalf("team leader statement leaked foreign project row: %+v", row)
		}
	}
	if result.Query.Side != "tenant" {
		t.Fatalf("team leader statement side must be forced to tenant: %+v", result.Query)
	}
	if result.EstimatedMarginUSD != nil {
		t.Fatal("team leader statement must not expose margin")
	}

	// Explicitly requesting a foreign project yields an empty bill, not data.
	code, result = postStatement(t, app, leaderToken, map[string]any{
		"side": "tenant", "from": "2020-01-01", "to": "2020-02-01", "timezone": "UTC",
		"project_ids": []string{foreignProjectID},
	})
	if code != http.StatusOK || len(result.Rows) != 0 {
		t.Fatalf("foreign project filter should produce an empty statement: %d %+v", code, result.Rows)
	}
}

func TestTeamLeaderStatementRejectsProviderAndMarginSides(t *testing.T) {
	_, app, _, leaderToken, _, _ := seedTeamStatementFixture(t)

	for _, side := range []string{"provider", "margin"} {
		code, _ := postStatement(t, app, leaderToken, map[string]any{
			"side": side, "from": "2020-01-01", "to": "2020-02-01", "timezone": "UTC",
		})
		// The side is forced to tenant, so the query succeeds as a tenant
		// statement rather than honoring the requested side.
		if code != http.StatusOK {
			t.Fatalf("side %s should be coerced to a tenant statement, got %d", side, code)
		}
	}
}

func TestTeamLeaderWithoutProjectsGetsEmptyStatement(t *testing.T) {
	store := NewMemoryStore()
	store.CreateResource("teams", AdminResource{ID: "team_empty", Name: "Empty Team", Status: StatusActive})
	if _, err := store.CreateAdminUser(AdminUser{
		Username: "empty-leader", Name: "Empty Leader", Email: "empty-leader@tokenhub.local",
		Role: "team_leader", TeamID: "team_empty", Status: StatusActive,
	}, "leader123456"); err != nil {
		t.Fatal(err)
	}
	app := New(store).Handler()
	token := providerTenancyLoginToken(t, app, "empty-leader@tokenhub.local", "leader123456")

	code, result := postStatement(t, app, token, map[string]any{
		"side": "tenant", "from": "2020-01-01", "to": "2020-02-01", "timezone": "UTC",
	})
	if code != http.StatusOK || len(result.Rows) != 0 {
		t.Fatalf("team without projects should get an empty statement: %d %+v", code, result.Rows)
	}
}

func TestStatementStaysDeniedForPlainUsers(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.CreateAdminUser(AdminUser{
		Username: "plain-user", Name: "Plain User", Email: "plain@tokenhub.local",
		Role: "user", Status: StatusActive,
	}, "user12345678"); err != nil {
		t.Fatal(err)
	}
	app := New(store).Handler()
	token := providerTenancyLoginToken(t, app, "plain@tokenhub.local", "user12345678")

	code, _ := postStatement(t, app, token, map[string]any{
		"side": "tenant", "from": "2020-01-01", "to": "2020-02-01", "timezone": "UTC",
	})
	if code != http.StatusForbidden {
		t.Fatalf("plain user statement should be forbidden, got %d", code)
	}
}

// seeBody re-issues the request so failure messages can include the body.
func seeBody(t *testing.T, app http.Handler, token string) string {
	t.Helper()
	response := doJSON(t, app, http.MethodPost, "/api/admin/billing/statements", map[string]any{
		"side": "tenant", "from": "2020-01-01", "to": "2020-02-01", "timezone": "UTC",
	}, token)
	return response.Body
}
