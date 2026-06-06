package tool

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/skill"
)

// LoadSkill returns the full instructions for one loaded skill.
type LoadSkill struct {
	skills map[string]skill.Skill
	info   *schema.ToolInfo
}

// NewLoadSkill creates a load_skill tool from already loaded skills.
func NewLoadSkill(skills []skill.Skill) (*LoadSkill, error) {
	index := make(map[string]skill.Skill, len(skills))
	for _, loaded := range skills {
		if err := loaded.Validate(); err != nil {
			return nil, err
		}
		key := strings.ToLower(loaded.Name)
		if _, exists := index[key]; exists {
			return nil, fmt.Errorf("duplicate skill name %q", loaded.Name)
		}
		index[key] = loaded
	}
	return &LoadSkill{
		skills: index,
		info: &schema.ToolInfo{
			Name: "load_skill",
			Desc: "Load the full instructions for a configured skill by name after the system prompt lists a relevant skill.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"name": {
					Type:     schema.String,
					Desc:     "Skill name to load.",
					Required: true,
				},
			}),
		},
	}, nil
}

func (t *LoadSkill) Info() *schema.ToolInfo { return t.info }

func (t *LoadSkill) ConcurrencySafe() bool { return true }

func (t *LoadSkill) Validate(arguments string) error {
	input, err := loadSkillInputFrom(arguments)
	if err != nil {
		return err
	}
	if _, exists := t.skills[strings.ToLower(input.Name)]; !exists {
		return fmt.Errorf("skill %q is not available", input.Name)
	}
	return nil
}

func (t *LoadSkill) Run(ctx context.Context, arguments string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	input, err := loadSkillInputFrom(arguments)
	if err != nil {
		return "", err
	}
	loaded, exists := t.skills[strings.ToLower(input.Name)]
	if !exists {
		return "", fmt.Errorf("skill %q is not available", input.Name)
	}
	return fmt.Sprintf(
		"Skill: %s\nDescription: %s\nInstructions:\n%s",
		loaded.Name,
		loaded.Description,
		loaded.Instructions,
	), nil
}

// SkillNames returns configured skill names in deterministic order.
func (t *LoadSkill) SkillNames() []string {
	names := make([]string, 0, len(t.skills))
	for _, loaded := range t.skills {
		names = append(names, loaded.Name)
	}
	sort.Strings(names)
	return names
}

type loadSkillInput struct {
	Name string `json:"name"`
}

func loadSkillInputFrom(arguments string) (loadSkillInput, error) {
	var input loadSkillInput
	if err := decodeArguments(arguments, &input); err != nil {
		return loadSkillInput{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return loadSkillInput{}, fmt.Errorf("name is required")
	}
	return input, nil
}
