package i18n

import (
	"fmt"
	"sync"
)

// Lang represents supported languages
type Lang string

const (
	ZH Lang = "zh" // Chinese
	EN Lang = "en" // English
	DE Lang = "de" // German
)

var (
	currentLang Lang = ZH
	mu          sync.RWMutex
	translations = map[Lang]map[string]string{
		ZH: zhTranslations,
		EN: enTranslations,
		DE: deTranslations,
	}
)

// SetLanguage sets the current language
func SetLanguage(lang string) {
	mu.Lock()
	defer mu.Unlock()

	switch lang {
	case "zh", "zh-CN", "zh-TW", "chinese":
		currentLang = ZH
	case "en", "en-US", "en-GB", "english":
		currentLang = EN
	case "de", "de-DE", "german", "deutsch":
		currentLang = DE
	default:
		currentLang = ZH // Default to Chinese
	}
}

// GetLanguage returns the current language
func GetLanguage() string {
	mu.RLock()
	defer mu.RUnlock()
	return string(currentLang)
}

// T translates a key to the current language
func T(key string, args ...interface{}) string {
	mu.RLock()
	lang := currentLang
	mu.RUnlock()

	trans, ok := translations[lang]
	if !ok {
		return key
	}

	str, ok := trans[key]
	if !ok {
		return key
	}

	if len(args) > 0 {
		return fmt.Sprintf(str, args...)
	}
	return str
}
