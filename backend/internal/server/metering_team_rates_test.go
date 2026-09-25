package server

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"tokenhub/backend/internal/metering"
)

func teamTestRates(input string) metering.Rates {
	return metering.Rates{Input: input, CacheRead: "1", CacheWrite: "2", CacheWrite5m: "2", CacheWrite1h: "2", Output: "5"}
}

func teamRateFixture(t *testing.T) (*GormStore, string, string) {
	t.Helper()
	store := NewMemoryStore()
	store.CreateResource("teams", AdminResource{ID: "team_rate", Name: "Rate Team", Status: StatusActive})
	store.CreateResource("teams", AdminResource{ID: "team_rate_other", Name: "Other Rate Team", Status: StatusActive})
	return store, "team_rate", "team_rate_other"
}

func TestTenantTeamRateCardValidation(t *testing.T) {
	valid := meteringRateCard{Kind: "tenant_team", Target: "team_rate:retail", Currency: "USD", Source: "test", Rates: teamTestRates("3")}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid tenant_team card rejected: %v", err)
	}
	for name, card := range map[string]meteringRateCard{
		"missing model": {Kind: "tenant_team", Target: "team_rate:", Currency: "USD", Source: "test", Rates: teamTestRates("3")},
		"missing team":  {Kind: "tenant_team", Target: ":retail", Currency: "USD", Source: "test", Rates: teamTestRates("3")},
		"no separator":  {Kind: "tenant_team", Target: "team_rate", Currency: "USD", Source: "test", Rates: teamTestRates("3")},
		"non-usd":       {Kind: "tenant_team", Target: "team_rate:retail", Currency: "CNY", Source: "test", Rates: teamTestRates("3")},
		"unknown kind":  {Kind: "team", Target: "team_rate:retail", Currency: "USD", Source: "test", Rates: teamTestRates("3")},
	} {
		if err := card.validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestTenantTeamRateCardPublishRequiresExistingTeam(t *testing.T) {
	store, teamID, _ := teamRateFixture(t)

	if _, err := store.PublishMeteringCard(meteringRateCard{Kind: "tenant_team", Target: "team_missing:retail", Currency: "USD", Source: "test", Rates: teamTestRates("3")}); err == nil {
		t.Fatal("publishing a tenant_team card for a missing team must fail")
	}
	if _, err := store.PublishMeteringCard(meteringRateCard{Kind: "tenant_team", Target: teamID + ":retail", Currency: "USD", Source: "test", Rates: teamTestRates("3")}); err != nil {
		t.Fatalf("publishing a tenant_team card for an existing team failed: %v", err)
	}
}

func TestResolveTenantMeteringCardPrefersTeamCard(t *testing.T) {
	store, teamID, otherTeamID := teamRateFixture(t)
	// Cards publish as immediately effective; resolve strictly afterwards.
	at := time.Now().UTC().Add(time.Minute)

	globalCard := meteringRateCard{Kind: "tenant", Target: "retail", Currency: "USD", Source: "global", Rates: teamTestRates("10")}
	if _, err := store.PublishMeteringCard(globalCard); err != nil {
		t.Fatal(err)
	}
	teamCard := meteringRateCard{Kind: "tenant_team", Target: tenantTeamRateCardTarget(teamID, "retail"), Currency: "USD", Source: "team", Rates: teamTestRates("4")}
	if _, err := store.PublishMeteringCard(teamCard); err != nil {
		t.Fatal(err)
	}

	resolve := func(teamID string) *meteringRateCard {
		t.Helper()
		var card *meteringRateCard
		if err := store.db.Transaction(func(tx *gorm.DB) error {
			resolved, err := resolveTenantMeteringCard(tx, teamID, "retail", at)
			card = resolved
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return card
	}

	if card := resolve(teamID); card == nil || card.Rates.Input != "4" {
		t.Fatalf("team project should resolve the team card: %+v", card)
	}
	if card := resolve(otherTeamID); card == nil || card.Rates.Input != "10" {
		t.Fatalf("foreign team should fall back to the global tenant card: %+v", card)
	}
	if card := resolve(""); card == nil || card.Rates.Input != "10" {
		t.Fatalf("unteamed project should use the global tenant card: %+v", card)
	}
}
