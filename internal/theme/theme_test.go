package theme

import (
	"path/filepath"
	"testing"
)

func valid() *Theme {
	return &Theme{
		Rarities: []Rarity{
			{Name: "common", DropChance: 70, BaseReward: 10},
			{Name: "epic", DropChance: 30, BaseReward: 400},
		},
		Sets: []Set{{Name: "Starter", BuffType: "power_percent", BuffValue: 10, RewardPoints: 500}},
		Cards: []Card{
			{Name: "Card A", Rarity: "common", PowerLevel: 10, ImageURL: "http://x/a.webp"},
			{Name: "Card B", Rarity: "epic", PowerLevel: 400, ImageURL: "http://x/b.webp", Set: "Starter"},
		},
	}
}

func TestValidateOK(t *testing.T) {
	warns, err := valid().Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestValidateUnknownRarity(t *testing.T) {
	th := valid()
	th.Cards[0].Rarity = "ghost"
	if _, err := th.Validate(); err == nil {
		t.Error("expected error for unknown rarity")
	}
}

func TestValidateUnknownSet(t *testing.T) {
	th := valid()
	th.Cards[1].Set = "Nope"
	if _, err := th.Validate(); err == nil {
		t.Error("expected error for unknown set")
	}
}

func TestValidateDuplicateCard(t *testing.T) {
	th := valid()
	th.Cards[1].Name = "Card A"
	if _, err := th.Validate(); err == nil {
		t.Error("expected error for duplicate card name")
	}
}

func TestValidateDropSumWarning(t *testing.T) {
	th := valid()
	th.Rarities[0].DropChance = 10 // sum becomes 40
	warns, err := th.Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) == 0 {
		t.Error("expected a drop_chance sum warning")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := valid().Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cards) != 2 || got.Cards[1].Set != "Starter" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
