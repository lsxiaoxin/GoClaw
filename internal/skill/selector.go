package skill

import (
	"sort"
	"strings"
)

// Selector picks relevant skills by simple keyword matching.
type Selector struct {
	skills []Skill
}

// NewSelector creates a deterministic selector.
func NewSelector(skills []Skill) *Selector {
	copied := append([]Skill(nil), skills...)
	sort.Slice(copied, func(i, j int) bool {
		return strings.ToLower(copied[i].Name) < strings.ToLower(copied[j].Name)
	})
	return &Selector{skills: copied}
}

// Select returns skills whose name, description, or instructions mention prompt keywords.
func (s *Selector) Select(prompt string) []Skill {
	terms := keywords(prompt)
	if len(terms) == 0 {
		return nil
	}
	var selected []Skill
	for _, candidate := range s.skills {
		haystack := strings.ToLower(candidate.Name + " " + candidate.Description + " " + candidate.Instructions)
		for _, term := range terms {
			if strings.Contains(haystack, term) {
				selected = append(selected, candidate)
				break
			}
		}
	}
	return selected
}

func keywords(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	})
	seen := map[string]struct{}{}
	var result []string
	for _, field := range fields {
		field = strings.Trim(field, "_-")
		if len(field) < 3 {
			continue
		}
		if _, exists := seen[field]; exists {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	return result
}
