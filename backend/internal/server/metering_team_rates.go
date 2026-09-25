package server

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// Team-scoped tenant pricing: a tenant_team rate card prices one team's usage
// of one model, overriding the global tenant card for that pair. The target
// format is "team_id:model_name".

const tenantTeamRateCardKind = "tenant_team"

// splitTenantTeamRateTarget parses "team_id:model_name". Model names may
// contain ":", so only the first segment is the team ID.
func splitTenantTeamRateTarget(target string) (string, string, bool) {
	target = strings.TrimSpace(target)
	index := strings.Index(target, ":")
	if index <= 0 || index == len(target)-1 {
		return "", "", false
	}
	return target[:index], target[index+1:], true
}

func tenantTeamRateCardTarget(teamID string, modelName string) string {
	return strings.TrimSpace(teamID) + ":" + strings.TrimSpace(modelName)
}

// resolveTenantMeteringCard returns the price card for a request: the team
// card when the project's team has one for this model, otherwise the global
// tenant card. A nil card keeps the legacy price.
func resolveTenantMeteringCard(tx *gorm.DB, teamID string, modelName string, at time.Time) (*meteringRateCard, error) {
	if teamID != "" {
		card, err := loadMeteringCard(tx, tenantTeamRateCardKind, tenantTeamRateCardTarget(teamID, modelName), at)
		if err != nil || card != nil {
			return card, err
		}
	}
	return loadMeteringCard(tx, "tenant", modelName, at)
}

// tenantTeamRateCardTeamExists reports whether the team referenced by a
// tenant_team target exists, so a typo cannot silently fall back to the
// global tenant price.
func tenantTeamRateCardTeamExists(tx *gorm.DB, teamID string) bool {
	var count int64
	if err := tx.Model(&AdminResource{}).Where("kind = ? AND id = ?", "teams", teamID).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}
