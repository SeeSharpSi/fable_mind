package story

import (
	"encoding/json"
	"reflect"
	"testing"
)

func pointer[T any](value T) *T { return &value }

func testGameState() *GameState {
	return &GameState{
		PlayerStatus: PlayerStatus{Health: 90, Stamina: 80, Conditions: []string{"wet"}},
		Inventory: []Item{
			{Name: "key", Description: "a brass key", Properties: []string{"metal"}, State: "unused"},
			{Name: "bread", Description: "a stale loaf", Properties: []string{"food"}, State: "whole"},
		},
		Environment: Environment{
			LocationName: "Cell",
			Description:  "A damp cell",
			Exits:        map[string]string{"north": "Hall", "south": "Yard"},
			WorldObjects: []WorldObject{
				{Name: "door", Properties: []string{"wood"}, State: "locked"},
				{Name: "stool", Properties: []string{"wood"}, State: "intact"},
			},
		},
		World: World{WorldTension: 20},
		NPCs: []NPC{
			{Name: "Guard", Disposition: "neutral", Knowledge: []string{"saw_player"}, Goal: "guard cell"},
			{Name: "Cook", Disposition: "friendly", Goal: "serve dinner"},
		},
		Puzzles: []Puzzle{
			{Name: "Locked Door", Type: "mechanical", Description: "Open the door", Status: "unsolved", SolutionHints: []string{"use_key"}},
			{Name: "Old Riddle", Type: "logic", Description: "Answer it", Status: "unsolved", SolutionHints: []string{"listen"}},
		},
		ProperNouns:       []ProperNoun{{Noun: "North Hall", PhraseUsed: "the hall", Description: "a torchlit stone passage"}},
		Rules:             Rules{ConsequenceModel: "challenging"},
		WinConditions:     []string{"escape"},
		LossConditions:    []string{"remain trapped"},
		SolvedPuzzleTypes: []string{"social"},
	}
}

func TestApplyStatePatch(t *testing.T) {
	current := testGameState()
	emptyProperties := []string{}
	patch := &StatePatch{
		Status: &StatusPatch{
			Health:     pointer(0),
			Stamina:    pointer(70),
			Conditions: &StringSetPatch{Add: []string{"cold"}, Remove: []string{"wet"}},
		},
		Inventory: &InventoryPatch{
			Add:    []Item{{Name: "torch", Description: "a resin torch", Properties: []string{"flammable"}, State: "lit"}},
			Update: []ItemPatch{{Name: "key", State: pointer("used"), Properties: &emptyProperties}},
			Remove: []string{"bread"},
		},
		Environment: &EnvironmentPatch{
			LocationName: pointer("Hall"),
			Description:  pointer("A torchlit hall"),
			Exits:        &ExitsPatch{Set: map[string]string{"east": "Kitchen"}, Remove: []string{"south"}},
			WorldObjects: &WorldObjectsPatch{
				Add:    []WorldObject{{Name: "banner", Properties: []string{"cloth"}, State: "hanging"}},
				Update: []WorldObjectPatch{{Name: "door", State: pointer("open")}},
				Remove: []string{"stool"},
			},
		},
		World: &WorldPatch{WorldTension: pointer(35)},
		NPCs: &NPCsPatch{
			Add: []NPC{{Name: "Warden", Disposition: "hostile", Goal: "stop escape"}},
			Update: []NPCPatch{{
				Name:        "Guard",
				Disposition: pointer("friendly"),
				Knowledge:   &StringSetPatch{Add: []string{"received_bribe"}, Remove: []string{"saw_player"}},
			}},
			Remove: []string{"Cook"},
		},
		Puzzles: &PuzzlesPatch{
			Add:    []Puzzle{{Name: "Banner Code", Type: "visual", Description: "Read the pattern", Status: "unsolved", SolutionHints: []string{"colors"}}},
			Remove: []string{"Old Riddle"},
			Solve:  []string{"Locked Door"},
		},
		Nouns:    &NounsPatch{Add: []ProperNoun{{Noun: "Warden", PhraseUsed: "the Warden", Description: "a severe prison commander"}}},
		Climax:   pointer(true),
		GameWon:  pointer(false),
		GameLost: pointer(true),
	}

	next, err := ApplyStatePatch(current, patch)
	if err != nil {
		t.Fatalf("ApplyStatePatch() error = %v", err)
	}
	if next.PlayerStatus.Health != 0 || next.PlayerStatus.Stamina != 70 || !reflect.DeepEqual(next.PlayerStatus.Conditions, []string{"cold"}) {
		t.Errorf("status = %#v", next.PlayerStatus)
	}
	if len(next.Inventory) != 2 || next.Inventory[0].State != "used" || len(next.Inventory[0].Properties) != 0 || next.Inventory[1].Name != "torch" {
		t.Errorf("inventory = %#v", next.Inventory)
	}
	if next.Environment.LocationName != "Hall" || next.Environment.Exits["east"] != "Kitchen" || next.Environment.WorldObjects[0].State != "open" {
		t.Errorf("environment = %#v", next.Environment)
	}
	if len(next.NPCs) != 2 || next.NPCs[0].Disposition != "friendly" || !reflect.DeepEqual(next.NPCs[0].Knowledge, []string{"received_bribe"}) {
		t.Errorf("npcs = %#v", next.NPCs)
	}
	if len(next.Puzzles) != 1 || next.Puzzles[0].Name != "Banner Code" {
		t.Errorf("puzzles = %#v", next.Puzzles)
	}
	if next.World.WorldTension != 35 || !next.Climax || next.GameWon || !next.GameLost {
		t.Errorf("world/flags = %#v, climax=%v won=%v lost=%v", next.World, next.Climax, next.GameWon, next.GameLost)
	}
	if !reflect.DeepEqual(next.SolvedPuzzleTypes, []string{"social", "mechanical"}) || len(next.ProperNouns) != 2 {
		t.Errorf("history/nouns = %#v / %#v", next.SolvedPuzzleTypes, next.ProperNouns)
	}
	if current.PlayerStatus.Health != 90 || current.Inventory[0].State != "unused" || current.Environment.WorldObjects[0].State != "locked" {
		t.Errorf("current state mutated: %#v", current)
	}
}

func TestApplyStatePatchRejectsConflictsAtomically(t *testing.T) {
	current := testGameState()
	patch := &StatePatch{Inventory: &InventoryPatch{
		Update: []ItemPatch{{Name: "key", State: pointer("used")}},
		Remove: []string{"key"},
	}}

	if _, err := ApplyStatePatch(current, patch); err == nil {
		t.Fatal("ApplyStatePatch() error = nil, want conflicting operation error")
	}
	if current.Inventory[0].State != "unused" || len(current.Inventory) != 2 {
		t.Errorf("current inventory mutated after rejected patch: %#v", current.Inventory)
	}
}

func TestApplyStatePatchRejectsMissingEntity(t *testing.T) {
	current := testGameState()
	patch := &StatePatch{NPCs: &NPCsPatch{Update: []NPCPatch{{Name: "Unknown", Goal: pointer("escape")}}}}

	if _, err := ApplyStatePatch(current, patch); err == nil {
		t.Fatal("ApplyStatePatch() error = nil, want missing NPC error")
	}
}

func TestPatchJSONDistinguishesOmittedAndEmptySlice(t *testing.T) {
	var patch StatePatch
	if err := json.Unmarshal([]byte(`{"inv":{"update":[{"name":"key","props":[]}]}}`), &patch); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if patch.Inventory.Update[0].Properties == nil || len(*patch.Inventory.Update[0].Properties) != 0 {
		t.Fatalf("properties = %#v, want present empty slice", patch.Inventory.Update[0].Properties)
	}
}

func TestApplyStatePatchEnforcesAuthority(t *testing.T) {
	current := testGameState()
	patch := &StatePatch{
		Status: &StatusPatch{Health: pointer(-20), Stamina: pointer(180)},
		World:  &WorldPatch{WorldTension: pointer(400)},
	}

	next, err := ApplyStatePatch(current, patch)
	if err != nil {
		t.Fatalf("ApplyStatePatch() error = %v", err)
	}
	if next.PlayerStatus.Health != 0 || next.PlayerStatus.Stamina != 100 || next.World.WorldTension != 250 || !next.GameLost {
		t.Errorf("normalized state = %#v", next)
	}

	changedHints := []string{"different_solution"}
	_, err = ApplyStatePatch(current, &StatePatch{Puzzles: &PuzzlesPatch{Update: []PuzzlePatch{{Name: "Locked Door", SolutionHints: &changedHints}}}})
	if err == nil {
		t.Fatal("ApplyStatePatch() error = nil, want immutable puzzle hints error")
	}

	current.Climax = true
	_, err = ApplyStatePatch(current, &StatePatch{Climax: pointer(false)})
	if err == nil {
		t.Fatal("ApplyStatePatch() error = nil, want climax reversal error")
	}
}

func TestApplyStatePatchRejectsDuplicateActivePuzzleType(t *testing.T) {
	current := testGameState()
	patch := &StatePatch{Puzzles: &PuzzlesPatch{Add: []Puzzle{{
		Name:          "Second Lock",
		Type:          "mechanical",
		Description:   "Open another lock",
		Status:        "unsolved",
		SolutionHints: []string{"find_another_key"},
	}}}}

	if _, err := ApplyStatePatch(current, patch); err == nil {
		t.Fatal("ApplyStatePatch() error = nil, want duplicate active puzzle type error")
	}
	if len(current.Puzzles) != 2 {
		t.Errorf("current puzzles mutated after rejection: %#v", current.Puzzles)
	}
}
