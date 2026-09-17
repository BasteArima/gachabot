package httpapi

import (
	"testing"
	"time"
)

func TestParseEndsAtTakesTheWholeDay(t *testing.T) {
	got, err := parseEndsAt("2026-12-31")
	if err != nil {
		t.Fatalf("дата не разобралась: %v", err)
	}
	if got == nil {
		t.Fatal("дата разобралась в nil")
	}
	// The panel sends a bare date; a season ending "31 декабря" must not end at
	// midnight when that day starts.
	if h := got.In(msk).Hour(); h != 23 {
		t.Errorf("сезон должен кончаться в конце дня, а кончается в %d ч", h)
	}
	if d := got.In(msk).Day(); d != 31 {
		t.Errorf("день съехал: %d", d)
	}
}

func TestParseEndsAtEmptyMeansNoTarget(t *testing.T) {
	got, err := parseEndsAt("")
	if err != nil || got != nil {
		t.Fatalf("пустая дата должна дать (nil, nil), а дала (%v, %v)", got, err)
	}
}

func TestParseEndsAtRejectsGarbage(t *testing.T) {
	if _, err := parseEndsAt("31.12.2026"); err == nil {
		t.Error("непонятная дата должна отвергаться, а прошла")
	}
}

func TestParseEndsAtAcceptsTimestamps(t *testing.T) {
	want := time.Date(2026, 12, 31, 18, 0, 0, 0, time.UTC)
	got, err := parseEndsAt(want.Format(time.RFC3339))
	if err != nil || got == nil || !got.Equal(want) {
		t.Fatalf("RFC3339 должен проходить как есть: %v, %v", got, err)
	}
}
