package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func seedRegistrationFixture(t *testing.T) (http.Handler, *GormStore) {
	t.Helper()
	registrationAttempts.mu.Lock()
	registrationAttempts.attempts = map[string][]time.Time{}
	registrationAttempts.mu.Unlock()
	store := NewMemoryStore()
	app := New(store).Handler()
	return app, store
}

func enableRegistration(t *testing.T, store *GormStore, inviteCode string) {
	t.Helper()
	setting, err := store.CreateResourceChecked("settings", AdminResource{
		ID: gatewaySettingsID, Name: "Gateway Base Settings", Status: StatusActive,
		Fields: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{
		registrationSettingEnabledField: true,
	}
	if inviteCode != "" {
		fields[registrationSettingCodeField] = inviteCode
	}
	updated, err := store.UpdateResource("settings", setting.ID, AdminResource{Fields: fields})
	if err != nil {
		t.Fatal(err)
	}
	_ = updated
}

func registrationStatusAllowed(t *testing.T, app http.Handler) bool {
	t.Helper()
	response := doJSON(t, app, http.MethodGet, "/api/admin/auth/registration-status", nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("registration status failed: %d %s", response.Code, response.Body)
	}
	var payload struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.Unmarshal([]byte(response.Body), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Allowed
}

func TestRegistrationDisabledByDefault(t *testing.T) {
	app, _ := seedRegistrationFixture(t)

	if registrationStatusAllowed(t, app) {
		t.Fatal("registration must be disabled by default")
	}
	response := doJSON(t, app, http.MethodPost, "/api/admin/auth/register", map[string]any{
		"username": "teacher", "email": "teacher@school.test", "password": "teacher123456", "invite_code": "whatever",
	}, "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("registration on disabled setting should be forbidden, got %d %s", response.Code, response.Body)
	}
}

func TestRegistrationEnabledWithoutCodeFailsClosed(t *testing.T) {
	app, store := seedRegistrationFixture(t)
	enableRegistration(t, store, "")

	if registrationStatusAllowed(t, app) {
		t.Fatal("enabled registration without an invite code must not report allowed")
	}
	response := doJSON(t, app, http.MethodPost, "/api/admin/auth/register", map[string]any{
		"username": "teacher", "email": "teacher@school.test", "password": "teacher123456", "invite_code": "",
	}, "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("registration without configured code should be forbidden, got %d %s", response.Code, response.Body)
	}
}

func TestRegisterCreatesTeamLeaderWithTeam(t *testing.T) {
	app, store := seedRegistrationFixture(t)
	enableRegistration(t, store, "classroom-invite")

	response := doJSON(t, app, http.MethodPost, "/api/admin/auth/register", map[string]any{
		"username":    "teacher-wang",
		"name":        "Wang Laoshi",
		"email":       "wang@school.test",
		"password":    "teacher123456",
		"invite_code": "classroom-invite",
	}, "")
	if response.Code != http.StatusCreated {
		t.Fatalf("registration failed: %d %s", response.Code, response.Body)
	}
	var payload struct {
		User AdminUser `json:"user"`
	}
	if err := json.Unmarshal([]byte(response.Body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.User.Role != "team_leader" || payload.User.TeamID == "" {
		t.Fatalf("registered user should be a team leader with a team: %+v", payload.User)
	}
	if payload.User.PasswordHash != "" {
		t.Fatal("registration response must not expose the password hash")
	}

	teams := store.ListResources("teams")
	if len(teams) != 1 || teams[0].ID != payload.User.TeamID {
		t.Fatalf("registration should create exactly one owning team: %+v", teams)
	}

	login := doJSON(t, app, http.MethodPost, "/api/admin/auth/login", map[string]any{
		"identity": "teacher-wang", "password": "teacher123456",
	}, "")
	if login.Code != http.StatusOK {
		t.Fatalf("registered teacher should be able to log in: %d %s", login.Code, login.Body)
	}
}

func TestRegisterRejectsBadInviteWeakPasswordAndConflicts(t *testing.T) {
	app, store := seedRegistrationFixture(t)
	enableRegistration(t, store, "classroom-invite")

	badCode := doJSON(t, app, http.MethodPost, "/api/admin/auth/register", map[string]any{
		"username": "teacher-li", "email": "li@school.test", "password": "teacher123456", "invite_code": "wrong-code",
	}, "")
	if badCode.Code != http.StatusForbidden {
		t.Fatalf("wrong invite code should be forbidden, got %d %s", badCode.Code, badCode.Body)
	}

	weak := doJSON(t, app, http.MethodPost, "/api/admin/auth/register", map[string]any{
		"username": "teacher-li", "email": "li@school.test", "password": "short", "invite_code": "classroom-invite",
	}, "")
	if weak.Code != http.StatusBadRequest {
		t.Fatalf("weak password should be rejected, got %d %s", weak.Code, weak.Body)
	}

	noDigit := doJSON(t, app, http.MethodPost, "/api/admin/auth/register", map[string]any{
		"username": "teacher-li", "email": "li@school.test", "password": "onlylettershere", "invite_code": "classroom-invite",
	}, "")
	if noDigit.Code != http.StatusBadRequest {
		t.Fatalf("password without digits should be rejected, got %d %s", noDigit.Code, noDigit.Body)
	}

	created := doJSON(t, app, http.MethodPost, "/api/admin/auth/register", map[string]any{
		"username": "teacher-li", "email": "li@school.test", "password": "teacher123456", "invite_code": "classroom-invite",
	}, "")
	if created.Code != http.StatusCreated {
		t.Fatalf("valid registration failed: %d %s", created.Code, created.Body)
	}

	conflict := doJSON(t, app, http.MethodPost, "/api/admin/auth/register", map[string]any{
		"username": "teacher-li", "email": "li@school.test", "password": "teacher123456", "invite_code": "classroom-invite",
	}, "")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("duplicate username should conflict, got %d %s", conflict.Code, conflict.Body)
	}

	// A failed registration must not leak orphan teams: only the successful
	// registration above should own a team.
	teams := store.ListResources("teams")
	if len(teams) != 1 {
		t.Fatalf("expected a single registration team, got %+v", teams)
	}
}

func TestRegistrationRateLimitBrutesInviteCode(t *testing.T) {
	app, store := seedRegistrationFixture(t)
	enableRegistration(t, store, "classroom-invite")

	lastCode := 0
	for i := 0; i < registrationMaxAttemptsPerIP+2; i++ {
		response := doJSON(t, app, http.MethodPost, "/api/admin/auth/register", map[string]any{
			"username": "brute-force", "email": "brute@school.test", "password": "teacher123456", "invite_code": "guess",
		}, "")
		lastCode = response.Code
		if response.Code == http.StatusTooManyRequests {
			break
		}
	}
	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("repeated registration attempts should be rate limited, got %d", lastCode)
	}
}

func TestRegistrationRateLimitUsesForwardedClientIP(t *testing.T) {
	registrationAttempts.mu.Lock()
	registrationAttempts.attempts = map[string][]time.Time{}
	registrationAttempts.mu.Unlock()
	store := NewMemoryStore()
	server := NewWithConfig(store, Config{AdminToken: "registration-proxy-admin", TrustedProxyCIDRs: []string{"127.0.0.0/8", "192.0.2.0/24"}})
	app := server.Handler()
	enableRegistration(t, store, "classroom-invite")

	register := func(xff string) int {
		request := httptest.NewRequest(http.MethodPost, "/api/admin/auth/register", strings.NewReader(`{"username":"proxy-user","email":"proxy@school.test","password":"teacher123456","invite_code":"wrong"}`))
		request.Header.Set("Content-Type", "application/json")
		if xff != "" {
			request.Header.Set("X-Forwarded-For", xff)
		}
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, request)
		return recorder.Code
	}

	// Exhaust the bucket of one forwarded client.
	for i := 0; i < registrationMaxAttemptsPerIP; i++ {
		if code := register("203.0.113.10"); code == http.StatusTooManyRequests {
			t.Fatalf("bucket should not exhaust before the limit")
		}
	}
	if code := register("203.0.113.10"); code != http.StatusTooManyRequests {
		t.Fatalf("exhausted forwarded client should be limited, got %d", code)
	}
	// A different forwarded client keeps its own bucket.
	if code := register("203.0.113.11"); code == http.StatusTooManyRequests {
		t.Fatal("a second forwarded client must not share the first client's bucket")
	}
	// A non-proxied caller (no XFF) uses the socket address bucket.
	if code := register(""); code == http.StatusTooManyRequests {
		t.Fatal("the direct client must not share forwarded clients' buckets")
	}
}
