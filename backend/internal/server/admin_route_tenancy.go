package server

import (
	"net/http"
	"strings"
)

// Model routes inherit tenancy from the provider they reference: a route may
// only be managed by the team that owns its provider, and the gateway serves
// a team-owned provider only to projects whose primary team matches. These
// helpers centralize both decisions so route writes and the request path
// share one rule.

// routeProviderAllowedForProject is the gateway-side predicate: a provider
// owned by a team only serves projects of that team, while platform-owned
// providers (empty owner) serve every project.
func routeProviderAllowedForProject(provider Provider, project Project) bool {
	owner := strings.TrimSpace(provider.OwnerTeamID)
	if owner == "" {
		return true
	}
	return owner == strings.TrimSpace(project.TeamID)
}

// routeOwnerTeamID resolves the owning team of a route through its provider.
func (s *Server) routeOwnerTeamID(route ModelRoute) string {
	provider, ok := s.store.GetProvider(route.ProviderID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(provider.OwnerTeamID)
}

// canManageRouteOwnedBy reports whether the actor may manage a route whose
// provider is owned by ownerTeamID.
func canManageRouteOwnedBy(user AdminUser, ownerTeamID string) bool {
	return canManageProviderOwnedBy(user, ownerTeamID)
}

// requireRouteWithinActorScope loads the route and enforces ownership through
// its provider. It writes the HTTP error response and returns ok=false when
// the route is missing or outside the actor's scope.
func (s *Server) requireRouteWithinActorScope(w http.ResponseWriter, r *http.Request, user AdminUser, routeID string) (ModelRoute, bool) {
	route, found := modelRouteByID(s.store.ListRoutes(), routeID)
	if !found {
		writeError(w, r, NewHTTPError(http.StatusNotFound, "route_not_found", "Route not found"))
		return ModelRoute{}, false
	}
	if !canManageRouteOwnedBy(user, s.routeOwnerTeamID(route)) {
		writeError(w, r, NewHTTPError(http.StatusForbidden, "route_forbidden", "Route belongs to a provider outside your team"))
		return ModelRoute{}, false
	}
	return route, true
}

// filterRoutesForActor narrows a route list to routes whose provider the
// actor may manage. Platform administrators see everything.
func (s *Server) filterRoutesForActor(user AdminUser, routes []ModelRoute) []ModelRoute {
	if isPlatformAdminRole(normalizeAdminRole(user.Role)) {
		return routes
	}
	out := make([]ModelRoute, 0, len(routes))
	for _, route := range routes {
		if canManageRouteOwnedBy(user, s.routeOwnerTeamID(route)) {
			out = append(out, route)
		}
	}
	return out
}

// validateRouteProviderWithinActorScope enforces that a new or patched route
// references a provider the actor may manage. It returns nil for platform
// administrators or when the referenced provider belongs to the actor's team.
func (s *Server) validateRouteProviderWithinActorScope(user AdminUser, providerID string) *HTTPError {
	if isPlatformAdminRole(normalizeAdminRole(user.Role)) {
		return nil
	}
	provider, found := s.store.GetProvider(strings.TrimSpace(providerID))
	if !found {
		// Missing providers are rejected by adapter validation; nothing to
		// scope-check here.
		return nil
	}
	if !canManageProviderOwnedBy(user, provider.OwnerTeamID) {
		return NewHTTPError(http.StatusForbidden, "route_provider_forbidden", "Route must reference a provider owned by your team")
	}
	return nil
}

// requireRoutingPlatformAdmin gates routing surfaces whose effect is global
// (model-level strategy, routing policy binding) to platform administrators
// even though team leaders hold the routing resource grant.
func requireRoutingPlatformAdmin(w http.ResponseWriter, r *http.Request, user AdminUser) bool {
	if !isPlatformAdminRole(normalizeAdminRole(user.Role)) {
		writeError(w, r, NewHTTPError(http.StatusForbidden, "admin_forbidden", "Admin role is not allowed to perform this action"))
		return false
	}
	return true
}
