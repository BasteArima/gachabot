package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Localizer struct {
	texts       map[string]map[string]string
	defaultLang string
}

// NewLocalizer читает все .json файлы из папки localesDir
func NewLocalizer(localesDir string, defaultLang string) (*Localizer, error) {
	loc := &Localizer{
		texts:       make(map[string]map[string]string),
		defaultLang: defaultLang,
	}

	files, err := os.ReadDir(localesDir)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения папки %s: %w", localesDir, err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			// Отрезаем ".json" от имени файла (ru.json -> ru)
			langCode := file.Name()[:len(file.Name())-5]

			data, err := os.ReadFile(filepath.Join(localesDir, file.Name()))
			if err != nil {
				return nil, err
			}

			var translations map[string]string
			if err := json.Unmarshal(data, &translations); err != nil {
				return nil, fmt.Errorf("ошибка парсинга %s: %w", file.Name(), err)
			}

			loc.texts[langCode] = translations
		}
	}
	return loc, nil
}

// T (Translate) возвращает переведенную строку, подставляя аргументы
func (l *Localizer) T(lang, key string, args ...interface{}) string {
	// 1. Ищем словарь для языка юзера (если нет, берем дефолтный "ru")
	langDict, exists := l.texts[lang]
	if !exists {
		langDict = l.texts[l.defaultLang]
	}

	// 2. Ищем нужный текст по ключу
	text, exists := langDict[key]
	if !exists {
		// Если ключа нет даже в дефолтном языке, возвращаем сам ключ, чтобы заметить ошибку
		text, exists = l.texts[l.defaultLang][key]
		if !exists {
			return key
		}
	}

	// 3. Подставляем переменные (если они есть)
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}
