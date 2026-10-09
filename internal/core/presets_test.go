package core

import (
	"strings"
	"testing"
)

func TestTablePresetsValidationAndProfileIntegration(t *testing.T) {
	valid := TablePreset{Name: " Relatórios ", Database: "db名", Tables: []string{"sample", "literal,+.[x]"}}
	for _, presets := range [][]TablePreset{nil, {}, {valid}} {
		if err := ValidateTablePresets(presets); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name    string
		presets []TablePreset
	}{
		{"empty name", []TablePreset{{Database: "db", Tables: []string{"t"}}}},
		{"long name", []TablePreset{{Name: strings.Repeat("名", 65), Database: "db", Tables: []string{"t"}}}},
		{"control", []TablePreset{{Name: "bad\n", Database: "db", Tables: []string{"t"}}}},
		{"invalid utf8", []TablePreset{{Name: "\xff", Database: "db", Tables: []string{"t"}}}},
		{"duplicate case", []TablePreset{valid, {Name: "relatórios", Database: "db", Tables: []string{"t"}}}},
		{"invalid db", []TablePreset{{Name: "name", Database: "\xff", Tables: []string{"t"}}}},
		{"empty tables", []TablePreset{{Name: "name", Database: "db", Tables: []string{}}}},
		{"duplicate table", []TablePreset{{Name: "name", Database: "db", Tables: []string{"t", "t"}}}},
		{"invalid table", []TablePreset{{Name: "name", Database: "db", Tables: []string{"t\n"}}}},
		{"limit", make([]TablePreset, 21)},
		{"tables limit", []TablePreset{{Name: "name", Database: "db", Tables: make([]string, MaxCatalogTables+1)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateTablePresets(tc.presets); err == nil {
				t.Fatal("invalid preset accepted")
			}
			p := testProfile()
			p.TablePresets = tc.presets
			if err := ValidateProfile(p); err == nil {
				t.Fatal("profile skipped preset validation")
			}
			p.Database = ""
			if err := ValidateProfile(p); err == nil {
				t.Fatal("profile without default db skipped preset validation")
			}
		})
	}
}
