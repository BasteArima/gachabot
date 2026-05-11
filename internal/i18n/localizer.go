package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Localizer struct {
	texts       map[string]map[string]string
	defaultLang string
}

func NewLocalizer(localesDir string, defaultLang string) (*Localizer, error) {
	loc := &Localizer{
		texts:       make(map[string]map[string]string),
		defaultLang: defaultLang,
	}

	files, err := os.ReadDir(localesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", localesDir, err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			langCode := strings.TrimSuffix(file.Name(), ".json")

			data, err := os.ReadFile(filepath.Join(localesDir, file.Name()))
			if err != nil {
				return nil, err
			}

			var translations map[string]string
			if err := json.Unmarshal(data, &translations); err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", file.Name(), err)
			}

			loc.texts[langCode] = translations
		}
	}
	return loc, nil
}

func (l *Localizer) Translate(lang, key string, args ...interface{}) string {
	langDict, exists := l.texts[lang]
	if !exists {
		langDict = l.texts[l.defaultLang]
	}

	formatStr, exists := langDict[key]
	if !exists {
		if defaultDict, ok := l.texts[l.defaultLang]; ok {
			formatStr, exists = defaultDict[key]
		}
	}

	if !exists {
		return key
	}

	if len(args) > 0 {
		return fmt.Sprintf(formatStr, args...)
	}

	return formatStr
}
