package server

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Invite-code self-registration turns a visitor into a team leader with a
// fresh team. It is the console's first public write endpoint, so it layers
// four guards: the administrator toggle, a constant-time invite-code check, a
// public-password strength policy, and a per-IP attempt window.

const (
	registrationSettingEnabledField = "allow_self_registration"
	registrationSettingCodeField    = "registration_invite_code"

	// registrationAttemptWindow and registrationMaxAttemptsPerIP bound abuse
	// from one client address. The invite code remains the primary gate.
	registrationAttemptWindow     = time.Hour
	registrationMaxAttemptsPerIP  = 10
	registrationMinPasswordLength = 10
	registrationMaxUsernameLength = 64
	registrationMaxNameLength     = 120
)

type registrationSettings struct {
	Allowed    bool
	InviteCode string
}

func (s *Server) registrationSettings() registrationSettings {
	setting, err := s.findResource("settings", gatewaySettingsID)
	if err != nil {
		return registrationSettings{}
	}
	return registrationSettings{
		Allowed:    truthyField(setting.Fields, registrationSettingEnabledField),
		InviteCode: strings.TrimSpace(stringField(setting.Fields, registrationSettingCodeField)),
	}
}

func (s *Server) handleAdminRegistrationStatus(w http.ResponseWriter, r *http.Request) {
	settings := s.registrationSettings()
	writeJSON(w, http.StatusOK, map[string]any{
		"allowed": settings.Allowed && settings.InviteCode != "",
	})
}

type registrationGuard struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

var registrationAttempts = &registrationGuard{attempts: map[string][]time.Time{}}

func (g *registrationGuard) allow(key string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	windowStart := now.Add(-registrationAttemptWindow)
	recent := g.attempts[key][:0]
	allowed := true
	for _, at := range g.attempts[key] {
		if at.Before(windowStart) {
			continue
		}
		recent = append(recent, at)
	}
	if len(recent) >= registrationMaxAttemptsPerIP {
		allowed = false
	} else {
		recent = append(recent, now)
	}
	if len(recent) == 0 {
		delete(g.attempts, key)
	} else {
		g.attempts[key] = recent
	}
	return allowed
}

func registrationInviteCodeMatches(configured string, submitted string) bool {
	if configured == "" || submitted == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(configured), []byte(submitted)) == 1
}

func validateRegistrationPassword(password string) *HTTPError {
	if len(password) < registrationMinPasswordLength {
		return NewHTTPError(http.StatusBadRequest, "weak_password",
			fmt.Sprintf("Password must be at least %d characters long", registrationMinPasswordLength))
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return NewHTTPError(http.StatusBadRequest, "weak_password", "Password must contain at least one letter and one digit")
	}
	return nil
}

func (s *Server) handleAdminRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username   string `json:"username"`
		Name       string `json:"name"`
		Email      string `json:"email"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	if err := s.decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	clientKey := clientIPKey(r)
	if !registrationAttempts.allow(clientKey, time.Now().UTC()) {
		writeError(w, r, NewHTTPError(http.StatusTooManyRequests, "registration_rate_limited",
			"Too many registration attempts; try again later"))
		return
	}
	settings := s.registrationSettings()
	if !settings.Allowed {
		writeError(w, r, NewHTTPError(http.StatusForbidden, "registration_disabled", "Self-registration is disabled"))
		return
	}
	// Fail closed: an enabled toggle without a configured code is a
	// misconfiguration and must not degrade into open registration.
	if !registrationInviteCodeMatches(settings.InviteCode, strings.TrimSpace(req.InviteCode)) {
		writeError(w, r, NewHTTPError(http.StatusForbidden, "registration_invite_invalid", "Invite code is invalid"))
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || len(username) > registrationMaxUsernameLength {
		writeError(w, r, NewHTTPError(http.StatusBadRequest, "invalid_username",
			fmt.Sprintf("Username is required and must be at most %d characters", registrationMaxUsernameLength)))
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = username
	}
	if len(name) > registrationMaxNameLength {
		writeError(w, r, NewHTTPError(http.StatusBadRequest, "invalid_name",
			fmt.Sprintf("Name must be at most %d characters", registrationMaxNameLength)))
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || !strings.Contains(email, "@") || strings.Contains(email, " ") {
		writeError(w, r, NewHTTPError(http.StatusBadRequest, "invalid_email", "A valid email is required"))
		return
	}
	if err := validateRegistrationPassword(req.Password); err != nil {
		writeError(w, r, err)
		return
	}

	team := AdminResource{
		ID:     NewID("team"),
		Name:   username,
		Status: StatusActive,
		Fields: map[string]any{
			"source": "self_registration",
		},
	}
	if _, err := s.store.CreateResourceChecked("teams", team); err != nil {
		writeError(w, r, err)
		return
	}
	user, err := s.store.CreateAdminUser(AdminUser{
		Username: username,
		Name:     name,
		Email:    email,
		Role:     "team_leader",
		TeamID:   team.ID,
		Status:   StatusActive,
	}, req.Password)
	if err != nil {
		// Compensate so a failed user creation does not leave an orphan team.
		if deleteErr := s.store.DeleteResource("teams", team.ID); deleteErr != nil {
			log.Printf("[tokenhub] failed to clean up registration team %s: %v", team.ID, deleteErr)
		}
		writeError(w, r, err)
		return
	}
	s.recordAdminAudit(r, user, "register", "admin_user", user.ID, "", map[string]any{
		"username": user.Username,
		"team_id":  team.ID,
		"role":     user.Role,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func clientIPKey(r *http.Request) string {
	host := r.RemoteAddr
	if index := strings.LastIndex(host, ":"); index > 0 {
		host = host[:index]
	}
	return strings.TrimSpace(host)
}
