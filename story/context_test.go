package story

import (
	"fmt"
	"reflect"
	"testing"
)

func TestBuildModelContextSelectsRelevantAndRecentNouns(t *testing.T) {
	state := testGameState()
	state.Environment.Description = "The First Landmark rises nearby"
	state.ProperNouns = make([]ProperNoun, 12)
	for i := range state.ProperNouns {
		state.ProperNouns[i] = ProperNoun{
			Noun:        fmt.Sprintf("Landmark %d", i),
			PhraseUsed:  fmt.Sprintf("the landmark %d", i),
			Description: "a remembered place",
		}
	}
	state.ProperNouns[0].Noun = "First Landmark"
	state.ProperNouns[1].Noun = "Forgotten Captain"

	contextState := BuildModelContext(state, "Ask Forgotten Captain for help")
	if len(contextState.ProperNouns) != 10 {
		t.Fatalf("context noun count = %d, want 10 relevant/recent nouns", len(contextState.ProperNouns))
	}
	gotNames := make(map[string]bool)
	for _, noun := range contextState.ProperNouns {
		gotNames[noun.Noun] = true
	}
	for _, required := range []string{"First Landmark", "Forgotten Captain", "Landmark 4", "Landmark 11"} {
		if !gotNames[required] {
			t.Errorf("context missing noun %q", required)
		}
	}
	if gotNames["Landmark 2"] || gotNames["Landmark 3"] {
		t.Errorf("context retained irrelevant old nouns: %#v", gotNames)
	}
	if len(state.ProperNouns) != 12 {
		t.Errorf("source glossary mutated, length = %d", len(state.ProperNouns))
	}
	if !reflect.DeepEqual(contextState.Puzzles, state.Puzzles) || !reflect.DeepEqual(contextState.WinConditions, state.WinConditions) || !reflect.DeepEqual(contextState.LossConditions, state.LossConditions) {
		t.Error("model context omitted integrity-critical state")
	}
}

func TestBuildModelContextReturnsDetachedState(t *testing.T) {
	state := testGameState()
	contextState := BuildModelContext(state, "wait")
	contextState.Inventory[0].Properties[0] = "changed"
	contextState.Puzzles[0].SolutionHints[0] = "changed"

	if state.Inventory[0].Properties[0] != "metal" || state.Puzzles[0].SolutionHints[0] != "use_key" {
		t.Error("model context aliases persisted state")
	}
}

func TestBuildModelContextMatchesAliasWordsAndShortNames(t *testing.T) {
	state := testGameState()
	state.ProperNouns = make([]ProperNoun, 10)
	for i := range state.ProperNouns {
		state.ProperNouns[i] = ProperNoun{Noun: fmt.Sprintf("Place %d", i), PhraseUsed: fmt.Sprintf("site %d", i), Description: "a place"}
	}
	state.ProperNouns[0] = ProperNoun{Noun: "Old Captain", PhraseUsed: "the old captain", Description: "a retired sailor"}
	state.ProperNouns[1] = ProperNoun{Noun: "Ed", PhraseUsed: "Ed", Description: "a quiet engineer"}

	contextState := BuildModelContext(state, "Ask the captain and Ed")
	got := make(map[string]bool)
	for _, noun := range contextState.ProperNouns {
		got[noun.Noun] = true
	}
	if !got["Old Captain"] || !got["Ed"] {
		t.Errorf("alias/short-name references not selected: %#v", got)
	}
}
