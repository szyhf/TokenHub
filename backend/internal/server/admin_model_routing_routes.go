package server

import (
	"net/http"
	"strings"
)

type adminModelItemHandler func(http.ResponseWriter, *http.Request, AdminUser, string)

func (s *Server) handleAdminModelsGet(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAdmin(w, r, "model", r.Method)
	if !ok {
		return
	}
	s.serveAdminModelsGet(w, user)
}

func (s *Server) handleAdminModelsPost(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAdmin(w, r, "model", r.Method)
	if !ok {
		return
	}
	s.serveAdminModelsPost(w, r, user)
}

func (s *Server) handleAdminModelRoute(w http.ResponseWriter, r *http.Request, resource string, serve adminModelItemHandler) {
	user, ok := s.requireAdmin(w, r, resource, r.Method)
	if !ok {
		return
	}
	modelName := strings.TrimSpace(r.PathValue("model_name"))
	if modelName == "" {
		writeError(w, r, NewHTTPError(http.StatusNotFound, "not_found", "Not found"))
		return
	}
	serve(w, r, user, modelName)
}

func (s *Server) handleAdminModelPatch(w http.ResponseWriter, r *http.Request) {
	s.handleAdminModelRoute(w, r, "model", s.serveAdminModelPatch)
}

func (s *Server) handleAdminModelDelete(w http.ResponseWriter, r *http.Request) {
	s.handleAdminModelRoute(w, r, "model", s.serveAdminModelDelete)
}

func (s *Server) handleAdminModelRoutingPolicyPatch(w http.ResponseWriter, r *http.Request) {
	s.handleAdminModelRoute(w, r, "routing", s.serveAdminModelRoutingPolicyPatch)
}

type adminRoutingRuleItemHandler func(http.ResponseWriter, *http.Request, AdminUser, string)

func (s *Server) handleAdminRoutesGet(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAdmin(w, r, "routing", r.Method)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": s.filterRoutesForActor(user, s.store.ListRoutes())})
}

func (s *Server) handleAdminRoutesPost(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAdmin(w, r, "routing", r.Method)
	if !ok {
		return
	}
	s.serveAdminRoutesPost(w, r, user)
}

func (s *Server) handleAdminRoutingRuleRoute(w http.ResponseWriter, r *http.Request, serve adminRoutingRuleItemHandler) {
	user, ok := s.requireAdmin(w, r, "routing", r.Method)
	if !ok {
		return
	}
	routeID := r.PathValue("route_id")
	if routeID == "" {
		writeError(w, r, NewHTTPError(http.StatusNotFound, "not_found", "Not found"))
		return
	}
	// Explain (GET) accepts a placeholder route ID and only needs the model
	// query, so existence-based ownership applies to mutations only; the
	// explain output itself already runs through the team-scoped planner.
	if r.Method != http.MethodGet {
		if _, ok := s.requireRouteWithinActorScope(w, r, user, routeID); !ok {
			return
		}
	}
	serve(w, r, user, routeID)
}

func (s *Server) handleAdminRoutePatch(w http.ResponseWriter, r *http.Request) {
	s.handleAdminRoutingRuleRoute(w, r, s.serveAdminRoutePatch)
}

func (s *Server) handleAdminRouteDelete(w http.ResponseWriter, r *http.Request) {
	s.handleAdminRoutingRuleRoute(w, r, s.serveAdminRouteDelete)
}

func (s *Server) handleAdminRouteExplainGet(w http.ResponseWriter, r *http.Request) {
	s.handleAdminRoutingRuleRoute(w, r, s.serveAdminRouteExplain)
}
