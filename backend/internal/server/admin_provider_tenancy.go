package server

import (
	"net/http"
	"strings"
)

// Provider channels are either platform-owned (empty OwnerTeamID, manageable
// only by platform administrators) or team-owned (manageable by platform
// administrators and by team leaders whose primary team matches the owner).
// These helpers centralize that decision so every provider and provider
// resource write path shares one ownership rule.

// canManageProviderOwnedBy reports whether the actor may manage providers
// owned by ownerTeamID. An empty ownerTeamID means a platform provider.
func canManageProviderOwnedBy(user AdminUser, ownerTeamID string) bool {
	role := normalizeAdminRole(user.Role)
	if isPlatformAdminRole(role) {
		return true
	}
	if role != "team_leader" {
		return false
	}
	teamID := strings.TrimSpace(user.TeamID)
	return teamID != "" && teamID == strings.TrimSpace(ownerTeamID)
}

// providerOwnerForCreate resolves the owner team stamped on a newly created
// provider. Team leaders can only create providers inside their own team;
// platform administrators may leave the provider platform-owned or assign it
// to an existing team.
func (s *Server) providerOwnerForCreate(user AdminUser, requestedOwner string) (string, *HTTPError) {
	role := normalizeAdminRole(user.Role)
	if isPlatformAdminRole(role) {
		owner := strings.TrimSpace(requestedOwner)
		if owner == "" {
			return "", nil
		}
		if !s.activeTeamIDSet()[owner] {
			return "", NewHTTPError(http.StatusBadRequest, "provider_owner_team_not_found", "Provider owner team does not exist")
		}
		return owner, nil
	}
	teamID := strings.TrimSpace(user.TeamID)
	if role != "team_leader" || teamID == "" {
		return "", NewHTTPError(http.StatusForbidden, "provider_owner_forbidden", "Only team leaders with a team can create team-owned providers")
	}
	return teamID, nil
}

// requireProviderWithinActorScope loads the provider and enforces ownership.
// It writes the HTTP error response and returns ok=false when the provider is
// missing or outside the actor's scope.
func (s *Server) requireProviderWithinActorScope(w http.ResponseWriter, r *http.Request, user AdminUser, providerID string) (Provider, bool) {
	provider, found := s.store.GetProvider(providerID)
	if !found {
		writeError(w, r, NewHTTPError(http.StatusNotFound, "provider_not_found", "Provider not found"))
		return Provider{}, false
	}
	if !canManageProviderOwnedBy(user, provider.OwnerTeamID) {
		writeError(w, r, NewHTTPError(http.StatusForbidden, "provider_forbidden", "Provider is not owned by your team"))
		return Provider{}, false
	}
	return provider, true
}

// requireProviderResourceWithinActorScope loads the resource and enforces
// ownership through its provider. It writes the HTTP error response and
// returns ok=false when the resource is missing or outside the actor's scope.
func (s *Server) requireProviderResourceWithinActorScope(w http.ResponseWriter, r *http.Request, user AdminUser, resourceID string) (ProviderResource, bool) {
	resource, found := s.store.GetProviderResource(resourceID)
	if !found {
		writeError(w, r, NewHTTPError(http.StatusNotFound, "provider_resource_not_found", "Provider resource not found"))
		return ProviderResource{}, false
	}
	provider, found := s.store.GetProvider(resource.ProviderID)
	if !found {
		writeError(w, r, NewHTTPError(http.StatusNotFound, "provider_not_found", "Provider not found"))
		return ProviderResource{}, false
	}
	if !canManageProviderOwnedBy(user, provider.OwnerTeamID) {
		writeError(w, r, NewHTTPError(http.StatusForbidden, "provider_forbidden", "Provider is not owned by your team"))
		return ProviderResource{}, false
	}
	return resource, true
}

// filterProvidersForActor narrows a provider list to the providers the actor
// may see. Platform administrators see everything.
func (s *Server) filterProvidersForActor(user AdminUser, providers []Provider) []Provider {
	if isPlatformAdminRole(normalizeAdminRole(user.Role)) {
		return providers
	}
	out := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if canManageProviderOwnedBy(user, provider.OwnerTeamID) {
			out = append(out, provider)
		}
	}
	return out
}

// filterProviderResourcesForActor narrows a resource list by the ownership of
// each resource's provider.
func (s *Server) filterProviderResourcesForActor(user AdminUser, resources []ProviderResource) []ProviderResource {
	if isPlatformAdminRole(normalizeAdminRole(user.Role)) {
		return resources
	}
	owners := make(map[string]string, len(resources))
	for _, provider := range s.store.ListProviders() {
		owners[provider.ID] = provider.OwnerTeamID
	}
	out := make([]ProviderResource, 0, len(resources))
	for _, resource := range resources {
		if canManageProviderOwnedBy(user, owners[resource.ProviderID]) {
			out = append(out, resource)
		}
	}
	return out
}

// providerResourceIDsOutsideActorScope returns the subset of resourceIDs whose
// provider is not manageable by the actor. An empty result means every ID is
// in scope; missing resources are not reported here so bulk operations keep
// their per-row not-found semantics.
func (s *Server) providerResourceIDsOutsideActorScope(user AdminUser, resourceIDs []string) []string {
	if isPlatformAdminRole(normalizeAdminRole(user.Role)) {
		return nil
	}
	owners := make(map[string]string)
	for _, provider := range s.store.ListProviders() {
		owners[provider.ID] = provider.OwnerTeamID
	}
	var blocked []string
	for _, resourceID := range uniqueStrings(resourceIDs) {
		resource, found := s.store.GetProviderResource(resourceID)
		if !found {
			continue
		}
		if !canManageProviderOwnedBy(user, owners[resource.ProviderID]) {
			blocked = append(blocked, resourceID)
		}
	}
	return blocked
}

// providerIDsOutsideActorScope returns the subset of providerIDs the actor
// may not manage.
func (s *Server) providerIDsOutsideActorScope(user AdminUser, providerIDs []string) []string {
	if isPlatformAdminRole(normalizeAdminRole(user.Role)) {
		return nil
	}
	var blocked []string
	for _, providerID := range uniqueStrings(providerIDs) {
		if provider, found := s.store.GetProvider(providerID); found && !canManageProviderOwnedBy(user, provider.OwnerTeamID) {
			blocked = append(blocked, providerID)
		}
	}
	return blocked
}
