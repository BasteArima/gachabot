package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

// writeLocales creates base/ and platform/ dirs with the given lang files.
func writeLocales(t *testing.T, base, platform map[string]string) (string, string) {
	t.Helper()
	root := t.TempDir()
	baseDir := filepath.Join(root, "base")
	platDir := filepath.Join(root, "platform")
	for dir, files := range map[string]map[string]string{baseDir: base, platDir: platform} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return baseDir, platDir
}

func TestSubstitutionAndOverlay(t *testing.T) {
	baseDir, platDir := writeLocales(t,
		map[string]string{
			"ru.json": `{"shared":"база","greet":"привет, {name}"}`,
		},
		map[string]string{
			"ru.json": `{"greet":"здравствуй, {name}!"}`,
		},
	)
	loc, err := NewLocalizer(baseDir, platDir, "ru")
	if err != nil {
		t.Fatal(err)
	}

	if got := loc.Translate("ru", "shared"); got != "база" {
		t.Errorf("base-only key: got %q", got)
	}
	// platform overlay must win over base
	if got := loc.Translate("ru", "greet", Args{"name": "Баст"}); got != "здравствуй, Баст!" {
		t.Errorf("overlay+substitution: got %q", got)
	}
	// missing key returns the key itself
	if got := loc.Translate("ru", "nope"); got != "nope" {
		t.Errorf("missing key: got %q", got)
	}
}

func TestPluralization(t *testing.T) {
	baseDir, platDir := writeLocales(t,
		map[string]string{
			"ru.json": `{"days":"{n} {n:день|дня|дней}"}`,
			"en.json": `{"days":"{n} {n:day|days}"}`,
		},
		map[string]string{"ru.json": `{}`, "en.json": `{}`},
	)
	loc, err := NewLocalizer(baseDir, platDir, "ru")
	if err != nil {
		t.Fatal(err)
	}

	ru := map[int]string{1: "1 день", 2: "2 дня", 5: "5 дней", 11: "11 дней", 21: "21 день", 22: "22 дня", 0: "0 дней"}
	for n, want := range ru {
		if got := loc.Translate("ru", "days", Args{"n": n}); got != want {
			t.Errorf("ru n=%d: got %q, want %q", n, got, want)
		}
	}

	en := map[int]string{1: "1 day", 2: "2 days", 5: "5 days"}
	for n, want := range en {
		if got := loc.Translate("en", "days", Args{"n": n}); got != want {
			t.Errorf("en n=%d: got %q, want %q", n, got, want)
		}
	}
}

func TestRarityFallback(t *testing.T) {
	baseDir, platDir := writeLocales(t,
		map[string]string{"ru.json": `{"rarity_mythic":"Мифическая"}`},
		map[string]string{"ru.json": `{}`},
	)
	loc, err := NewLocalizer(baseDir, platDir, "ru")
	if err != nil {
		t.Fatal(err)
	}
	// known code -> localized
	if got := loc.Rarity("ru", "mythic"); got != "Мифическая" {
		t.Errorf("known rarity: got %q", got)
	}
	// unknown code (e.g. pre-migration Russian name) -> returned as-is
	if got := loc.Rarity("ru", "Эпическая"); got != "Эпическая" {
		t.Errorf("unknown rarity fallback: got %q", got)
	}
}

// TestRealLocales loads the project's actual locale files to catch JSON errors
// and verify a couple of representative rendered strings end-to-end.
func TestRealLocales(t *testing.T) {
	root := filepath.Join("..", "..", "locales")
	for _, platform := range []string{"telegram", "discord"} {
		loc, err := NewLocalizer(filepath.Join(root, "base"), filepath.Join(root, platform), "ru")
		if err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
		// streak pluralization should agree with the count
		got := loc.Translate("ru", "streak_kept_alive", Args{"days": 5})
		if got == "streak_kept_alive" {
			t.Fatalf("%s: streak_kept_alive missing", platform)
		}
		if !contains(got, "5") || !contains(got, "дней") {
			t.Errorf("%s: streak ru days=5 unexpected: %q", platform, got)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
