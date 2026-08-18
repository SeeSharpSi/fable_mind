package prompts

import (
	"fmt"
	"strings"
	"testing"
)

func TestBasePromptContextBudgetAndIntegrityRules(t *testing.T) {
	prompt := fmt.Sprintf(BasePrompt, "Test Author")
	if len(prompt) > 6000 {
		t.Fatalf("base prompt length = %d characters, want at most 6000", len(prompt))
	}

	required := []string{
		"complete initialized state",
		"inv[].props and env.objs[].props are arrays of strings",
		"env.objs[] objects are {name,props?,state?} with no desc",
		"npcs[] are {name,disp,know?,goal} with no desc",
		"Every change must directly follow",
		"Keep win/loss goals and puzzle hints secret",
		"provide fair clues and opportunities",
		"discoverable solution",
		"never repeat solved_puzzles types",
		"rules.model controls severity",
		"NPC behavior follows disp, goal, and know",
		"Keep env.desc current",
		`<span class="proper-noun tooltip"`,
		"Every noun entry used this turn requires matching story tooltip",
		"relevant/recent glossary subset",
		"Normal turns use 70-110 words",
		"family-friendly and PG",
	}
	for _, rule := range required {
		if !strings.Contains(prompt, rule) {
			t.Errorf("base prompt missing integrity rule %q", rule)
		}
	}
}
