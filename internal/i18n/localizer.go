package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Args holds named arguments for a translation, e.g.
//
//	Translate(lang, "roll_success", Args{"name": card.Name, "reward": 10})
//
// Placeholders in the text use {name} for substitution and {count:one|few|many}
// for pluralization (the word form is selected by the integer value of "count").
type Args = map[string]any

type Localizer struct {
	texts       map[string]map[string]string
	defaultLang string
}

// placeholderRe matches {name} and {name:form1|form2|...}.
var placeholderRe = regexp.MustCompile(`\{(\w+)(?::([^}]*))?\}`)

// NewLocalizer loads base translations and then overlays platform-specific ones
// on top (the platform value wins on conflict). Each directory contains
// <lang>.json files of flat key -> text maps. Pass an empty platformDir to load
// only the base layer.
func NewLocalizer(baseDir, platformDir, defaultLang string) (*Localizer, error) {
	loc := &Localizer{
		texts:       make(map[string]map[string]string),
		defaultLang: defaultLang,
	}
	for _, dir := range []string{baseDir, platformDir} {
		if dir == "" {
			continue
		}
		if err := loc.loadDir(dir); err != nil {
			return nil, err
		}
	}
	return loc, nil
}

func (l *Localizer) loadDir(dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dir, err)
	}
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}
		lang := strings.TrimSuffix(file.Name(), ".json")

		data, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			return err
		}

		var translations map[string]string
		if err := json.Unmarshal(data, &translations); err != nil {
			return fmt.Errorf("failed to parse %s: %w", file.Name(), err)
		}

		if l.texts[lang] == nil {
			l.texts[lang] = make(map[string]string)
		}
		for k, v := range translations {
			l.texts[lang][k] = v // overlay: platform overwrites base
		}
	}
	return nil
}

func (l *Localizer) lookup(lang, key string) (string, bool) {
	if dict, ok := l.texts[lang]; ok {
		if v, ok := dict[key]; ok {
			return v, true
		}
	}
	if dict, ok := l.texts[l.defaultLang]; ok {
		if v, ok := dict[key]; ok {
			return v, true
		}
	}
	return "", false
}

// Translate returns the text for key in the given language with placeholders
// substituted. Call it without args for static text, or with a single Args map
// for substitution/pluralization. Missing keys return the key itself.
func (l *Localizer) Translate(lang, key string, args ...Args) string {
	format, ok := l.lookup(lang, key)
	if !ok {
		return key
	}
	if len(args) == 0 || len(args[0]) == 0 {
		return format
	}
	return l.substitute(lang, format, args[0])
}

func (l *Localizer) substitute(lang, format string, args Args) string {
	return placeholderRe.ReplaceAllStringFunc(format, func(m string) string {
		sub := placeholderRe.FindStringSubmatch(m)
		name, forms := sub[1], sub[2]

		val, ok := args[name]
		if !ok {
			return m // leave unknown placeholder as-is
		}
		if forms == "" {
			return fmt.Sprint(val)
		}
		return selectPlural(lang, val, strings.Split(forms, "|"))
	})
}

func selectPlural(lang string, val any, forms []string) string {
	if len(forms) == 0 {
		return ""
	}
	return forms[pluralIndex(lang, toInt(val), len(forms))]
}

func toInt(val any) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		n, _ := strconv.Atoi(fmt.Sprint(v))
		return n
	}
}

// pluralIndex returns the index of the plural form to use. East-Slavic languages
// (ru/uk/be) use 3 forms (one/few/many); everything else uses 2 (one/other). The
// result is clamped so a 2-form string still works under a 3-form language.
func pluralIndex(lang string, n, nforms int) int {
	if n < 0 {
		n = -n
	}
	var idx int
	switch lang {
	case "ru", "uk", "be":
		m10, m100 := n%10, n%100
		switch {
		case m10 == 1 && m100 != 11:
			idx = 0 // one
		case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
			idx = 1 // few
		default:
			idx = 2 // many
		}
	default:
		if n == 1 {
			idx = 0 // one
		} else {
			idx = 1 // other
		}
	}
	if idx > nforms-1 {
		idx = nforms - 1
	}
	return idx
}

// Rarity returns the localized display name for a rarity code (e.g. "mythic" ->
// "Мифическая"). It looks up the key "rarity_<code>" (lowercased) and falls back
// to the code itself if no translation exists, so display stays correct both
// before and after rarity names are migrated to English codes in the database.
func (l *Localizer) Rarity(lang, code string) string {
	key := "rarity_" + strings.ToLower(code)
	if v := l.Translate(lang, key); v != key {
		return v
	}
	return code
}
