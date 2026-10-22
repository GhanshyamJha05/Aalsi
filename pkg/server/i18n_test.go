package server

import (
	"testing"
)

// TestI18nCompleteness enforces hard constraint:
// "English/Hindi toggle with a test that fails on any untranslated string."
func TestI18nCompleteness(t *testing.T) {
	enDict, hasEn := Translations["en"]
	if !hasEn {
		t.Fatalf("English dictionary 'en' is missing")
	}

	hiDict, hasHi := Translations["hi"]
	if !hasHi {
		t.Fatalf("Hindi dictionary 'hi' is missing")
	}

	// 1. Every English key must exist in Hindi with non-empty translation
	for key, enVal := range enDict {
		hiVal, exists := hiDict[key]
		if !exists {
			t.Errorf("FAIL: Missing Hindi translation for key '%s' (English: %q)", key, enVal)
		} else if hiVal == "" {
			t.Errorf("FAIL: Empty Hindi translation for key '%s'", key)
		}
	}

	// 2. Every Hindi key must exist in English
	for key, hiVal := range hiDict {
		enVal, exists := enDict[key]
		if !exists {
			t.Errorf("FAIL: Missing English translation for key '%s' (Hindi: %q)", key, hiVal)
		} else if enVal == "" {
			t.Errorf("FAIL: Empty English translation for key '%s'", key)
		}
	}
}
