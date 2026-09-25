package server

import "strings"

// scopeStatementQueryForTeamLeader narrows a statement request to the team
// leader's own tenant bill: the side is forced to tenant, provider filters are
// dropped, and the project list is intersected with the projects visible to
// the actor's team. empty reports that the team has no billable projects, in
// which case the caller should answer an empty statement instead of running
// an unscoped query.
func (s *Server) scopeStatementQueryForTeamLeader(user AdminUser, q statementQuery) (statementQuery, bool, error) {
	q.Side = "tenant"
	q.ProviderID = ""
	q.ResourceID = ""

	teamID := strings.TrimSpace(user.TeamID)
	if teamID == "" {
		return statementQuery{}, true, nil
	}
	teamName := teamID
	for _, team := range s.store.ListResources("teams") {
		if team.ID == teamID && strings.TrimSpace(team.Name) != "" {
			teamName = strings.TrimSpace(team.Name)
			break
		}
	}

	visible := map[string]bool{}
	for _, project := range s.store.ListProjects() {
		if strings.TrimSpace(project.TeamID) == teamID {
			visible[project.ID] = true
		}
	}
	if len(visible) == 0 {
		return statementQuery{}, true, nil
	}

	requested := map[string]bool{}
	for _, projectID := range q.ProjectIDs {
		requested[strings.TrimSpace(projectID)] = true
	}
	projectIDs := make([]string, 0, len(visible))
	for projectID := range visible {
		if len(requested) == 0 || requested[projectID] {
			projectIDs = append(projectIDs, projectID)
		}
	}
	if len(projectIDs) == 0 {
		// Every requested project belongs to another team; an empty bill is
		// the honest answer rather than a validation error.
		return statementQuery{}, true, nil
	}

	q.Customer = teamName
	q.ProjectIDs = projectIDs
	return q, false, nil
}
