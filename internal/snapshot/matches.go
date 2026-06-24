package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// Matches is the persisted result of resolving unique tracks to the catalog.
type Matches struct {
	ResolvedAt time.Time      `json:"resolved_at"`
	Matches    []domain.Match `json:"matches"`
}

// SaveMatches writes match results to path.
func SaveMatches(path string, m Matches) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal matches: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write matches %s: %w", path, err)
	}
	return nil
}

// LoadMatches reads previously saved match results.
func LoadMatches(path string) (Matches, error) {
	var m Matches
	data, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("read matches %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse matches %s: %w", path, err)
	}
	return m, nil
}
