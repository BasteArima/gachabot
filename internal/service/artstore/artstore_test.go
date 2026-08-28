package artstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Bunny suit":        "bunny_suit",
		"  Ahegao  ":        "ahegao",
		"Ass bigger THAN 2": "ass_bigger_than_2",
		"Cat-ears":          "cat_ears",
		"a  b":              "a_b",
		"Мифическая":        "", // nothing transliterable is left
		"!!!":               "",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidation(t *testing.T) {
	for _, s := range []string{"Rare", "Mythical", "New Set 2"} {
		if !ValidFolder(s) {
			t.Errorf("folder %q should be valid", s)
		}
	}
	for _, s := range []string{"", "../etc", "a/b", "Rare/", ".hidden", "имя"} {
		if ValidFolder(s) {
			t.Errorf("folder %q must be rejected", s)
		}
	}
	for _, s := range []string{"abs", "bunny_suit", "cat-ears", "a1"} {
		if !ValidSlug(s) {
			t.Errorf("slug %q should be valid", s)
		}
	}
	for _, s := range []string{"", "Abs", "../abs", "a/b", "_abs", "имя"} {
		if ValidSlug(s) {
			t.Errorf("slug %q must be rejected", s)
		}
	}
}

// webp/png bytes only need valid headers here: nothing decodes them.
var fakeArt = []byte("RIFF____WEBPdata")

func dirService(t *testing.T) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	return New(Config{PublicBase: "https://art.example/", LocalDir: root}), root
}

func TestPutWritesBothFilesAndReportsUrls(t *testing.T) {
	svc, root := dirService(t)
	if !svc.Enabled() {
		t.Fatal("service should be enabled with a local dir and a public base")
	}

	plainURL, framedURL, err := svc.Put("Rare", "bunny_suit", ".webp", fakeArt, fakeArt, false)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if plainURL != "https://art.example/cards/Rare/bunny_suit.webp" {
		t.Errorf("plain url = %q", plainURL)
	}
	if framedURL != "https://art.example/cards_framed/Rare/bunny_suit.webp" {
		t.Errorf("framed url = %q", framedURL)
	}

	for _, rel := range []string{"cards/Rare/bunny_suit.webp", "cards_framed/Rare/bunny_suit.webp"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s on disk: %v", rel, err)
		}
	}
	// The staging file must not survive.
	if _, err := os.Stat(filepath.Join(root, "cards", "Rare", "bunny_suit.webp.part")); !os.IsNotExist(err) {
		t.Error("temporary .part file was left behind")
	}
}

func TestPutRefusesToOverwriteUnlessAsked(t *testing.T) {
	svc, _ := dirService(t)
	if _, _, err := svc.Put("Rare", "abs", ".webp", fakeArt, fakeArt, false); err != nil {
		t.Fatalf("first Put: %v", err)
	}

	_, _, err := svc.Put("Rare", "abs", ".webp", []byte("RIFF____WEBPnew!"), fakeArt, false)
	if !errors.Is(err, ErrExists) {
		t.Fatalf("second Put should report ErrExists, got %v", err)
	}

	if _, _, err := svc.Put("Rare", "abs", ".webp", []byte("RIFF____WEBPnew!"), fakeArt, true); err != nil {
		t.Fatalf("overwrite Put: %v", err)
	}
}

func TestExistingListsTakenPaths(t *testing.T) {
	svc, _ := dirService(t)

	taken, err := svc.Existing("Rare", "abs")
	if err != nil || len(taken) != 0 {
		t.Fatalf("Existing on empty store = %v, %v", taken, err)
	}

	if _, _, err := svc.Put("Rare", "abs", ".webp", fakeArt, fakeArt, false); err != nil {
		t.Fatal(err)
	}
	taken, err = svc.Existing("Rare", "abs")
	if err != nil {
		t.Fatal(err)
	}
	if len(taken) != 2 {
		t.Errorf("expected both files reported, got %v", taken)
	}
}

func TestPutRejectsBadInput(t *testing.T) {
	svc, root := dirService(t)

	if _, _, err := svc.Put("../escape", "abs", ".webp", fakeArt, fakeArt, false); err == nil {
		t.Error("traversal in folder must be rejected")
	}
	if _, _, err := svc.Put("Rare", "../abs", ".webp", fakeArt, fakeArt, false); err == nil {
		t.Error("traversal in slug must be rejected")
	}
	if _, _, err := svc.Put("Rare", "abs", ".webp", nil, fakeArt, false); err == nil {
		t.Error("a missing file must be rejected")
	}
	// Nothing may have been written by the rejected calls.
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Errorf("store should still be empty, has %d entries", len(entries))
	}
}

func TestDisabledWithoutDriverOrBase(t *testing.T) {
	if New(Config{PublicBase: "https://art.example"}).Enabled() {
		t.Error("no driver should mean disabled")
	}
	if New(Config{LocalDir: t.TempDir()}).Enabled() {
		t.Error("no public base should mean disabled")
	}
	if _, _, err := New(Config{}).Put("Rare", "abs", ".webp", fakeArt, fakeArt, false); !errors.Is(err, ErrDisabled) {
		t.Errorf("Put on a disabled service = %v, want ErrDisabled", err)
	}
}

func TestHostKeyIsRequired(t *testing.T) {
	// Password auth without a pinned host key would hand the password to
	// whatever answers, so the driver must refuse to even dial.
	if _, _, err := parseHostKey(""); err == nil {
		t.Fatal("an empty host key must be rejected")
	}
	if _, _, err := parseHostKey("SHA256:abcdef"); err != nil {
		t.Errorf("a fingerprint should be accepted: %v", err)
	}
	if _, _, err := parseHostKey("not a key"); err == nil {
		t.Error("garbage must be rejected")
	}
}

// A pinned key must also be the key that gets negotiated. Go prefers ed25519
// last, so without asking for the pinned type a server offering rsa and ecdsa
// too would present a different key and the connection would fail as a
// mismatch — which reads like an attack rather than a configuration detail.
func TestHostKeyPinsItsOwnAlgorithm(t *testing.T) {
	const line = "api.example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAINWBMDJ/Ugt3iiUbsdluGzFqVAl6Snjzky5oOLfdztXO"

	_, algos, err := parseHostKey(line)
	if err != nil {
		t.Fatalf("keyscan line should parse: %v", err)
	}
	if len(algos) != 1 || algos[0] != "ssh-ed25519" {
		t.Errorf("host key algorithms = %v, want [ssh-ed25519]", algos)
	}

	// Without the host field it must behave identically.
	_, algos, err = parseHostKey(strings.SplitN(line, " ", 2)[1])
	if err != nil || len(algos) != 1 || algos[0] != "ssh-ed25519" {
		t.Errorf("bare key line = %v, %v", algos, err)
	}

	// A fingerprint cannot name a type, so nothing is restricted.
	if _, algos, _ := parseHostKey("SHA256:abcdef"); algos != nil {
		t.Errorf("a fingerprint should not restrict algorithms, got %v", algos)
	}
}
