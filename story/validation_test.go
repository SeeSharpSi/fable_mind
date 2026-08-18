package story

import "testing"

func TestValidateInitialGameState(t *testing.T) {
	state := testGameState()
	state.SolvedPuzzleTypes = nil
	state.Rules.ConsequenceModel = "punishing"

	validated, err := ValidateInitialGameState(state, "exploratory")
	if err != nil {
		t.Fatalf("ValidateInitialGameState() error = %v", err)
	}
	if validated.Rules.ConsequenceModel != "exploratory" {
		t.Errorf("consequence model = %q, want server-selected model", validated.Rules.ConsequenceModel)
	}
	if validated.SolvedPuzzleTypes == nil || validated.Environment.Exits == nil {
		t.Error("validated state collections must be initialized")
	}
	validated.Inventory[0].Properties[0] = "changed"
	if state.Inventory[0].Properties[0] != "metal" {
		t.Error("validated state aliases model state")
	}
}

func TestValidateInitialGameStateRejectsIncompleteState(t *testing.T) {
	tests := []struct {
		name   string
		change func(*GameState)
	}{
		{"missing location", func(state *GameState) { state.Environment.LocationName = "" }},
		{"missing win", func(state *GameState) { state.WinConditions = nil }},
		{"missing loss", func(state *GameState) { state.LossConditions = nil }},
		{"invalid health", func(state *GameState) { state.PlayerStatus.Health = 0 }},
		{"missing puzzle hints", func(state *GameState) { state.Puzzles[0].SolutionHints = nil }},
		{"already complete", func(state *GameState) { state.GameWon = true }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := testGameState()
			state.SolvedPuzzleTypes = nil
			test.change(state)
			if _, err := ValidateInitialGameState(state, "challenging"); err == nil {
				t.Fatal("ValidateInitialGameState() error = nil, want validation error")
			}
		})
	}
}

func TestValidateInitialGameStateRejectsDuplicateActivePuzzleTypes(t *testing.T) {
	state := testGameState()
	state.SolvedPuzzleTypes = nil
	state.Puzzles[1].Type = state.Puzzles[0].Type

	if _, err := ValidateInitialGameState(state, "challenging"); err == nil {
		t.Fatal("ValidateInitialGameState() error = nil, want duplicate puzzle type error")
	}
}

func TestValidateInitialGameStateRejectsUnboundedContext(t *testing.T) {
	state := testGameState()
	state.SolvedPuzzleTypes = nil
	state.NPCs[0].Knowledge = make([]string, MaxNPCKnowledge+1)
	for i := range state.NPCs[0].Knowledge {
		state.NPCs[0].Knowledge[i] = "fact"
		state.NPCs[0].Knowledge[i] += string(rune('a' + i))
	}

	if _, err := ValidateInitialGameState(state, "challenging"); err == nil {
		t.Fatal("ValidateInitialGameState() error = nil, want knowledge bound error")
	}
}

func TestValidateInitialGameStateRejectsLongLocationName(t *testing.T) {
	state := testGameState()
	state.SolvedPuzzleTypes = nil
	state.Environment.LocationName = string(make([]byte, 161))

	if _, err := ValidateInitialGameState(state, "challenging"); err == nil {
		t.Fatal("ValidateInitialGameState() error = nil, want location length error")
	}
}
