package core

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func ValidateTablePresets(presets []TablePreset) error {
	if len(presets) > 20 {
		return fmt.Errorf("perfil permite no máximo 20 presets de tabelas")
	}
	names := make([]string, 0, len(presets))
	for _, preset := range presets {
		name := strings.TrimSpace(preset.Name)
		if !utf8.ValidString(preset.Name) || name == "" || len([]rune(preset.Name)) > 64 || strings.IndexFunc(preset.Name, unicode.IsControl) >= 0 {
			return fmt.Errorf("nome do preset deve conter entre 1 e 64 caracteres sem controles")
		}
		for _, existing := range names {
			if strings.EqualFold(existing, name) {
				return fmt.Errorf("nomes de presets devem ser únicos, sem distinguir maiúsculas")
			}
		}
		names = append(names, name)
		if err := ValidateDatabase(preset.Database); err != nil {
			return fmt.Errorf("preset %s: %w", name, err)
		}
		if len(preset.Tables) == 0 || len(preset.Tables) > MaxCatalogTables {
			return fmt.Errorf("preset %s deve conter entre 1 e 10000 tabelas", name)
		}
		seen := make(map[string]bool, len(preset.Tables))
		for _, table := range preset.Tables {
			if err := ValidateTableName(table); err != nil {
				return fmt.Errorf("preset %s: %w", name, err)
			}
			if seen[table] {
				return fmt.Errorf("preset %s contém tabela repetida: %s", name, table)
			}
			seen[table] = true
		}
	}
	return nil
}
