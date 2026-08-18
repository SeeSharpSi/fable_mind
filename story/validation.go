package story

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MinPlayerAttribute = 0
	MaxPlayerAttribute = 100
	MinWorldTension    = 0
	MaxWorldTension    = 250
	MaxInventoryItems  = 32
	MaxWorldObjects    = 32
	MaxNPCs            = 20
	MaxActivePuzzles   = 8
	MaxNPCKnowledge    = 16
	MaxPuzzleHints     = 8
)

// ValidateInitialGameState validates model-created state and preserves server rules.
func ValidateInitialGameState(state *GameState, consequenceModel string) (*GameState, error) {
	if state == nil {
		return nil, fmt.Errorf("initial game state is missing")
	}
	next := cloneGameState(state)
	next.Rules.ConsequenceModel = consequenceModel
	if next.PlayerStatus.Health <= MinPlayerAttribute || next.PlayerStatus.Health > MaxPlayerAttribute {
		return nil, fmt.Errorf("initial health must be from 1 to %d", MaxPlayerAttribute)
	}
	if next.PlayerStatus.Stamina < MinPlayerAttribute || next.PlayerStatus.Stamina > MaxPlayerAttribute {
		return nil, fmt.Errorf("initial stamina must be from %d to %d", MinPlayerAttribute, MaxPlayerAttribute)
	}
	if next.World.WorldTension < MinWorldTension || next.World.WorldTension > MaxWorldTension {
		return nil, fmt.Errorf("initial tension must be from %d to %d", MinWorldTension, MaxWorldTension)
	}
	if strings.TrimSpace(next.Environment.LocationName) == "" || strings.TrimSpace(next.Environment.Description) == "" {
		return nil, fmt.Errorf("initial environment requires location and description")
	}
	if len(next.WinConditions) == 0 || len(next.LossConditions) == 0 {
		return nil, fmt.Errorf("initial state requires win and loss conditions")
	}
	if next.Climax || next.GameWon || next.GameLost {
		return nil, fmt.Errorf("initial state cannot start at climax or game over")
	}
	if len(next.SolvedPuzzleTypes) != 0 {
		return nil, fmt.Errorf("initial state cannot contain solved puzzle types")
	}
	if err := validateStringSet(next.WinConditions); err != nil {
		return nil, fmt.Errorf("win conditions: %w", err)
	}
	if err := validateStringSet(next.LossConditions); err != nil {
		return nil, fmt.Errorf("loss conditions: %w", err)
	}
	if err := validateItems(next.Inventory); err != nil {
		return nil, fmt.Errorf("inventory: %w", err)
	}
	if err := validateWorldObjects(next.Environment.WorldObjects); err != nil {
		return nil, fmt.Errorf("environment objects: %w", err)
	}
	if err := validateNPCs(next.NPCs); err != nil {
		return nil, fmt.Errorf("npcs: %w", err)
	}
	if err := validatePuzzles(next.Puzzles, nil); err != nil {
		return nil, fmt.Errorf("puzzles: %w", err)
	}
	if err := validateNouns(next.ProperNouns); err != nil {
		return nil, fmt.Errorf("nouns: %w", err)
	}

	next.Environment.Exits = normalizeExits(next.Environment.Exits)
	initializeStateCollections(next)
	if err := validateCurrentGameState(next); err != nil {
		return nil, err
	}
	return next, nil
}

func validateCurrentGameState(state *GameState) error {
	if strings.TrimSpace(state.Rules.ConsequenceModel) == "" {
		return fmt.Errorf("consequence model is missing")
	}
	if strings.TrimSpace(state.Environment.LocationName) == "" || strings.TrimSpace(state.Environment.Description) == "" {
		return fmt.Errorf("environment requires location and description")
	}
	if err := validateTextLength("location name", state.Environment.LocationName, 160); err != nil {
		return err
	}
	if state.GameWon && state.GameLost {
		return fmt.Errorf("game cannot be both won and lost")
	}
	if state.PlayerStatus.Health < MinPlayerAttribute || state.PlayerStatus.Health > MaxPlayerAttribute || state.PlayerStatus.Stamina < MinPlayerAttribute || state.PlayerStatus.Stamina > MaxPlayerAttribute {
		return fmt.Errorf("player attributes are out of bounds")
	}
	if state.World.WorldTension < MinWorldTension || state.World.WorldTension > MaxWorldTension {
		return fmt.Errorf("world tension is out of bounds")
	}
	if err := validateTextLength("environment description", state.Environment.Description, 800); err != nil {
		return err
	}
	if len(state.Environment.Exits) > 16 {
		return fmt.Errorf("environment has more than 16 exits")
	}
	for direction, destination := range state.Environment.Exits {
		if err := validateTextLength("exit direction", direction, 40); err != nil {
			return err
		}
		if err := validateTextLength("exit destination", destination, 160); err != nil {
			return err
		}
	}
	if len(state.PlayerStatus.Conditions) > 16 || len(state.WinConditions) > 8 || len(state.LossConditions) > 8 || len(state.SolvedPuzzleTypes) > 64 {
		return fmt.Errorf("state contains too many conditions, goals, or solved puzzle types")
	}
	if err := validateTextList("condition", state.PlayerStatus.Conditions, 120); err != nil {
		return err
	}
	if err := validateTextList("win condition", state.WinConditions, 300); err != nil {
		return err
	}
	if err := validateTextList("loss condition", state.LossConditions, 300); err != nil {
		return err
	}
	if err := validateTextList("solved puzzle type", state.SolvedPuzzleTypes, 80); err != nil {
		return err
	}
	if err := validateStringSet(state.PlayerStatus.Conditions); err != nil {
		return fmt.Errorf("conditions: %w", err)
	}
	if err := validateStringSet(state.WinConditions); err != nil {
		return fmt.Errorf("win conditions: %w", err)
	}
	if err := validateStringSet(state.LossConditions); err != nil {
		return fmt.Errorf("loss conditions: %w", err)
	}
	if err := validateStringSet(state.SolvedPuzzleTypes); err != nil {
		return fmt.Errorf("solved puzzle types: %w", err)
	}
	if err := validateItems(state.Inventory); err != nil {
		return fmt.Errorf("inventory: %w", err)
	}
	if err := validateWorldObjects(state.Environment.WorldObjects); err != nil {
		return fmt.Errorf("environment objects: %w", err)
	}
	if err := validateNPCs(state.NPCs); err != nil {
		return fmt.Errorf("npcs: %w", err)
	}
	if err := validatePuzzles(state.Puzzles, state.SolvedPuzzleTypes); err != nil {
		return fmt.Errorf("puzzles: %w", err)
	}
	if err := validateNouns(state.ProperNouns); err != nil {
		return fmt.Errorf("nouns: %w", err)
	}
	return nil
}

func initializeStateCollections(state *GameState) {
	if state.PlayerStatus.Conditions == nil {
		state.PlayerStatus.Conditions = []string{}
	}
	if state.Inventory == nil {
		state.Inventory = []Item{}
	}
	if state.Environment.Exits == nil {
		state.Environment.Exits = map[string]string{}
	}
	if state.Environment.WorldObjects == nil {
		state.Environment.WorldObjects = []WorldObject{}
	}
	if state.NPCs == nil {
		state.NPCs = []NPC{}
	}
	if state.Puzzles == nil {
		state.Puzzles = []Puzzle{}
	}
	if state.ProperNouns == nil {
		state.ProperNouns = []ProperNoun{}
	}
	if state.SolvedPuzzleTypes == nil {
		state.SolvedPuzzleTypes = []string{}
	}
}

func validateItems(items []Item) error {
	if len(items) > MaxInventoryItems {
		return fmt.Errorf("inventory has more than %d items", MaxInventoryItems)
	}
	if _, err := indexByName(items, func(item Item) string { return item.Name }); err != nil {
		return err
	}
	for _, item := range items {
		if strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("item %q requires description", item.Name)
		}
		if err := validateTextLength("item name", item.Name, 120); err != nil {
			return err
		}
		if err := validateTextLength("item description", item.Description, 240); err != nil {
			return err
		}
		if len(item.Properties) > 12 {
			return fmt.Errorf("item %q has too many properties", item.Name)
		}
		if err := validateTextList("item property", item.Properties, 80); err != nil {
			return err
		}
		if err := validateTextLength("item state", item.State, 120); err != nil {
			return err
		}
	}
	return nil
}

func validateWorldObjects(objects []WorldObject) error {
	if len(objects) > MaxWorldObjects {
		return fmt.Errorf("environment has more than %d objects", MaxWorldObjects)
	}
	if _, err := indexByName(objects, func(object WorldObject) string { return object.Name }); err != nil {
		return err
	}
	for _, object := range objects {
		if err := validateTextLength("object name", object.Name, 120); err != nil {
			return err
		}
		if len(object.Properties) > 12 {
			return fmt.Errorf("object %q has too many properties", object.Name)
		}
		if err := validateTextList("object property", object.Properties, 80); err != nil {
			return err
		}
		if err := validateTextLength("object state", object.State, 120); err != nil {
			return err
		}
	}
	return nil
}

func validateNPCs(npcs []NPC) error {
	if len(npcs) > MaxNPCs {
		return fmt.Errorf("state has more than %d npcs", MaxNPCs)
	}
	if _, err := indexByName(npcs, func(npc NPC) string { return npc.Name }); err != nil {
		return err
	}
	for _, npc := range npcs {
		if strings.TrimSpace(npc.Disposition) == "" || strings.TrimSpace(npc.Goal) == "" {
			return fmt.Errorf("npc %q requires disposition and goal", npc.Name)
		}
		if err := validateTextLength("npc name", npc.Name, 120); err != nil {
			return err
		}
		if err := validateTextLength("npc disposition", npc.Disposition, 80); err != nil {
			return err
		}
		if err := validateTextLength("npc goal", npc.Goal, 240); err != nil {
			return err
		}
		if len(npc.Knowledge) > MaxNPCKnowledge {
			return fmt.Errorf("npc %q has more than %d knowledge facts", npc.Name, MaxNPCKnowledge)
		}
		if err := validateTextList("npc knowledge", npc.Knowledge, 200); err != nil {
			return err
		}
	}
	return nil
}

func validatePuzzles(puzzles []Puzzle, solvedTypes []string) error {
	if len(puzzles) > MaxActivePuzzles {
		return fmt.Errorf("state has more than %d active puzzles", MaxActivePuzzles)
	}
	if _, err := indexByName(puzzles, func(puzzle Puzzle) string { return puzzle.Name }); err != nil {
		return err
	}
	solved := make(map[string]struct{}, len(solvedTypes))
	for _, puzzleType := range solvedTypes {
		solved[entityKey(puzzleType)] = struct{}{}
	}
	active := make(map[string]struct{}, len(puzzles))
	for _, puzzle := range puzzles {
		if strings.TrimSpace(puzzle.Type) == "" || strings.TrimSpace(puzzle.Description) == "" || strings.TrimSpace(puzzle.Status) == "" || len(puzzle.SolutionHints) == 0 {
			return fmt.Errorf("puzzle %q requires type, description, status, and solution hints", puzzle.Name)
		}
		if err := validateTextLength("puzzle name", puzzle.Name, 120); err != nil {
			return err
		}
		if err := validateTextLength("puzzle type", puzzle.Type, 80); err != nil {
			return err
		}
		if err := validateTextLength("puzzle description", puzzle.Description, 400); err != nil {
			return err
		}
		if err := validateTextLength("puzzle status", puzzle.Status, 80); err != nil {
			return err
		}
		if len(puzzle.SolutionHints) > MaxPuzzleHints {
			return fmt.Errorf("puzzle %q has more than %d solution hints", puzzle.Name, MaxPuzzleHints)
		}
		if err := validateTextList("puzzle hint", puzzle.SolutionHints, 160); err != nil {
			return err
		}
		puzzleType := entityKey(puzzle.Type)
		if _, repeated := solved[puzzleType]; repeated {
			return fmt.Errorf("puzzle %q repeats solved type %q", puzzle.Name, puzzle.Type)
		}
		if _, repeated := active[puzzleType]; repeated {
			return fmt.Errorf("active puzzle type %q is duplicated", puzzle.Type)
		}
		active[puzzleType] = struct{}{}
	}
	return nil
}

func validateNouns(nouns []ProperNoun) error {
	if _, err := indexByName(nouns, func(noun ProperNoun) string { return noun.Noun }); err != nil {
		return err
	}
	for _, noun := range nouns {
		if strings.TrimSpace(noun.PhraseUsed) == "" || strings.TrimSpace(noun.Description) == "" {
			return fmt.Errorf("noun %q requires phrase and description", noun.Noun)
		}
		if err := validateTextLength("noun", noun.Noun, 120); err != nil {
			return err
		}
		if err := validateTextLength("noun phrase", noun.PhraseUsed, 120); err != nil {
			return err
		}
		if err := validateTextLength("noun description", noun.Description, 200); err != nil {
			return err
		}
	}
	return nil
}

func validateStringSet(values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key, err := requiredKey(value)
		if err != nil {
			return err
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("value %q is duplicated", value)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func normalizeExits(exits map[string]string) map[string]string {
	normalized := make(map[string]string, len(exits))
	for direction, destination := range exits {
		key := entityKey(direction)
		if key == "" {
			continue
		}
		normalized[key] = destination
	}
	return normalized
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func validateTextList(label string, values []string, maximum int) error {
	for _, value := range values {
		if err := validateTextLength(label, value, maximum); err != nil {
			return err
		}
	}
	return nil
}

func validateTextLength(label, value string, maximum int) error {
	if utf8.RuneCountInString(value) > maximum {
		return fmt.Errorf("%s exceeds %d characters", label, maximum)
	}
	return nil
}
