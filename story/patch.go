package story

import (
	"fmt"
	"slices"
	"strings"
)

// StringSetPatch applies set-like changes to a string slice.
type StringSetPatch struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// StatePatch contains turn-specific changes to game state.
type StatePatch struct {
	Status      *StatusPatch      `json:"status,omitempty"`
	Inventory   *InventoryPatch   `json:"inv,omitempty"`
	Environment *EnvironmentPatch `json:"env,omitempty"`
	World       *WorldPatch       `json:"world,omitempty"`
	NPCs        *NPCsPatch        `json:"npcs,omitempty"`
	Puzzles     *PuzzlesPatch     `json:"puzzles,omitempty"`
	Nouns       *NounsPatch       `json:"nouns,omitempty"`
	Climax      *bool             `json:"climax,omitempty"`
	GameWon     *bool             `json:"won,omitempty"`
	GameLost    *bool             `json:"lost,omitempty"`
}

type StatusPatch struct {
	Health     *int            `json:"hp,omitempty"`
	Stamina    *int            `json:"sp,omitempty"`
	Conditions *StringSetPatch `json:"conds,omitempty"`
}

type InventoryPatch struct {
	Add    []Item      `json:"add,omitempty"`
	Update []ItemPatch `json:"update,omitempty"`
	Remove []string    `json:"remove,omitempty"`
}

type ItemPatch struct {
	Name        string    `json:"name"`
	Description *string   `json:"desc,omitempty"`
	Properties  *[]string `json:"props,omitempty"`
	State       *string   `json:"state,omitempty"`
}

type EnvironmentPatch struct {
	LocationName *string            `json:"loc,omitempty"`
	Description  *string            `json:"desc,omitempty"`
	Exits        *ExitsPatch        `json:"exits,omitempty"`
	WorldObjects *WorldObjectsPatch `json:"objs,omitempty"`
}

type ExitsPatch struct {
	Set    map[string]string `json:"set,omitempty"`
	Remove []string          `json:"remove,omitempty"`
}

type WorldObjectsPatch struct {
	Add    []WorldObject      `json:"add,omitempty"`
	Update []WorldObjectPatch `json:"update,omitempty"`
	Remove []string           `json:"remove,omitempty"`
}

type WorldObjectPatch struct {
	Name       string    `json:"name"`
	Properties *[]string `json:"props,omitempty"`
	State      *string   `json:"state,omitempty"`
}

type WorldPatch struct {
	WorldTension *int `json:"tension,omitempty"`
}

type NPCsPatch struct {
	Add    []NPC      `json:"add,omitempty"`
	Update []NPCPatch `json:"update,omitempty"`
	Remove []string   `json:"remove,omitempty"`
}

type NPCPatch struct {
	Name        string          `json:"name"`
	Disposition *string         `json:"disp,omitempty"`
	Knowledge   *StringSetPatch `json:"know,omitempty"`
	Goal        *string         `json:"goal,omitempty"`
}

type PuzzlesPatch struct {
	Add    []Puzzle      `json:"add,omitempty"`
	Update []PuzzlePatch `json:"update,omitempty"`
	Remove []string      `json:"remove,omitempty"`
	Solve  []string      `json:"solve,omitempty"`
}

type PuzzlePatch struct {
	Name          string    `json:"name"`
	Type          *string   `json:"type,omitempty"`
	Description   *string   `json:"desc,omitempty"`
	Status        *string   `json:"status,omitempty"`
	SolutionHints *[]string `json:"hints,omitempty"`
}

type NounsPatch struct {
	Add []ProperNoun `json:"add,omitempty"`
}

// ApplyStatePatch validates and atomically applies a turn patch.
func ApplyStatePatch(current *GameState, patch *StatePatch) (*GameState, error) {
	if current == nil {
		return nil, fmt.Errorf("current game state is missing")
	}
	if patch == nil {
		return nil, fmt.Errorf("state patch is missing")
	}

	next := cloneGameState(current)
	if patch.Status != nil {
		if patch.Status.Health != nil {
			next.PlayerStatus.Health = clamp(*patch.Status.Health, MinPlayerAttribute, MaxPlayerAttribute)
		}
		if patch.Status.Stamina != nil {
			next.PlayerStatus.Stamina = clamp(*patch.Status.Stamina, MinPlayerAttribute, MaxPlayerAttribute)
		}
		if patch.Status.Conditions != nil {
			if err := applyStringSetPatch(&next.PlayerStatus.Conditions, patch.Status.Conditions); err != nil {
				return nil, fmt.Errorf("status conditions: %w", err)
			}
		}
	}
	if patch.Inventory != nil {
		if err := applyInventoryPatch(&next.Inventory, patch.Inventory); err != nil {
			return nil, fmt.Errorf("inventory: %w", err)
		}
	}
	if patch.Environment != nil {
		if err := applyEnvironmentPatch(&next.Environment, patch.Environment); err != nil {
			return nil, fmt.Errorf("environment: %w", err)
		}
	}
	if patch.World != nil && patch.World.WorldTension != nil {
		next.World.WorldTension = clamp(*patch.World.WorldTension, MinWorldTension, MaxWorldTension)
	}
	if patch.NPCs != nil {
		if err := applyNPCsPatch(&next.NPCs, patch.NPCs); err != nil {
			return nil, fmt.Errorf("npcs: %w", err)
		}
	}
	if patch.Puzzles != nil {
		if err := applyPuzzlesPatch(&next.Puzzles, &next.SolvedPuzzleTypes, patch.Puzzles); err != nil {
			return nil, fmt.Errorf("puzzles: %w", err)
		}
	}
	if patch.Nouns != nil {
		if err := applyNounsPatch(&next.ProperNouns, patch.Nouns); err != nil {
			return nil, fmt.Errorf("nouns: %w", err)
		}
	}
	if patch.Climax != nil {
		if next.Climax && !*patch.Climax {
			return nil, fmt.Errorf("climax cannot be reversed")
		}
		next.Climax = *patch.Climax
	}
	if patch.GameWon != nil {
		if next.GameWon && !*patch.GameWon {
			return nil, fmt.Errorf("won cannot be reversed")
		}
		next.GameWon = *patch.GameWon
	}
	if patch.GameLost != nil {
		if next.GameLost && !*patch.GameLost {
			return nil, fmt.Errorf("lost cannot be reversed")
		}
		next.GameLost = *patch.GameLost
	}
	if next.PlayerStatus.Health == 0 {
		next.GameLost = true
	}
	if next.GameWon && next.GameLost {
		return nil, fmt.Errorf("game cannot be both won and lost")
	}
	if err := validateCurrentGameState(next); err != nil {
		return nil, err
	}

	return next, nil
}

func applyInventoryPatch(items *[]Item, patch *InventoryPatch) error {
	if err := validateItems(patch.Add); err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if err := validateOperations(
		namesOf(patch.Add, func(item Item) string { return item.Name }),
		namesOf(patch.Update, func(item ItemPatch) string { return item.Name }),
		patch.Remove,
	); err != nil {
		return err
	}
	index, err := indexByName(*items, func(item Item) string { return item.Name })
	if err != nil {
		return err
	}
	for _, item := range patch.Add {
		key, err := requiredKey(item.Name)
		if err != nil {
			return err
		}
		if _, exists := index[key]; exists {
			return fmt.Errorf("cannot add existing item %q", item.Name)
		}
		*items = append(*items, cloneItem(item))
		index[key] = len(*items) - 1
	}
	for _, update := range patch.Update {
		key, err := requiredKey(update.Name)
		if err != nil {
			return err
		}
		position, exists := index[key]
		if !exists {
			return fmt.Errorf("cannot update missing item %q", update.Name)
		}
		item := &(*items)[position]
		if update.Description != nil {
			item.Description = *update.Description
		}
		if update.Properties != nil {
			item.Properties = append([]string(nil), (*update.Properties)...)
		}
		if update.State != nil {
			item.State = *update.State
		}
	}
	return removeByName(items, patch.Remove, func(item Item) string { return item.Name })
}

func applyEnvironmentPatch(environment *Environment, patch *EnvironmentPatch) error {
	if patch.LocationName != nil {
		environment.LocationName = *patch.LocationName
	}
	if patch.Description != nil {
		environment.Description = *patch.Description
	}
	if patch.Exits != nil {
		environment.Exits = normalizeExits(environment.Exits)
		seen := make(map[string]struct{})
		for direction, destination := range patch.Exits.Set {
			key, err := requiredKey(direction)
			if err != nil {
				return fmt.Errorf("exit: %w", err)
			}
			seen[key] = struct{}{}
			environment.Exits[key] = destination
		}
		for _, direction := range patch.Exits.Remove {
			key, err := requiredKey(direction)
			if err != nil {
				return fmt.Errorf("exit: %w", err)
			}
			if _, conflict := seen[key]; conflict {
				return fmt.Errorf("exit %q is both set and removed", direction)
			}
			if _, exists := environment.Exits[key]; !exists {
				return fmt.Errorf("cannot remove missing exit %q", direction)
			}
			delete(environment.Exits, key)
		}
	}
	if patch.WorldObjects != nil {
		if err := applyWorldObjectsPatch(&environment.WorldObjects, patch.WorldObjects); err != nil {
			return fmt.Errorf("objects: %w", err)
		}
	}
	return nil
}

func applyWorldObjectsPatch(objects *[]WorldObject, patch *WorldObjectsPatch) error {
	if err := validateWorldObjects(patch.Add); err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if err := validateOperations(
		namesOf(patch.Add, func(object WorldObject) string { return object.Name }),
		namesOf(patch.Update, func(object WorldObjectPatch) string { return object.Name }),
		patch.Remove,
	); err != nil {
		return err
	}
	index, err := indexByName(*objects, func(object WorldObject) string { return object.Name })
	if err != nil {
		return err
	}
	for _, object := range patch.Add {
		key, err := requiredKey(object.Name)
		if err != nil {
			return err
		}
		if _, exists := index[key]; exists {
			return fmt.Errorf("cannot add existing object %q", object.Name)
		}
		*objects = append(*objects, cloneWorldObject(object))
		index[key] = len(*objects) - 1
	}
	for _, update := range patch.Update {
		key, err := requiredKey(update.Name)
		if err != nil {
			return err
		}
		position, exists := index[key]
		if !exists {
			return fmt.Errorf("cannot update missing object %q", update.Name)
		}
		object := &(*objects)[position]
		if update.Properties != nil {
			object.Properties = append([]string(nil), (*update.Properties)...)
		}
		if update.State != nil {
			object.State = *update.State
		}
	}
	return removeByName(objects, patch.Remove, func(object WorldObject) string { return object.Name })
}

func applyNPCsPatch(npcs *[]NPC, patch *NPCsPatch) error {
	if err := validateNPCs(patch.Add); err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if err := validateOperations(
		namesOf(patch.Add, func(npc NPC) string { return npc.Name }),
		namesOf(patch.Update, func(npc NPCPatch) string { return npc.Name }),
		patch.Remove,
	); err != nil {
		return err
	}
	index, err := indexByName(*npcs, func(npc NPC) string { return npc.Name })
	if err != nil {
		return err
	}
	for _, npc := range patch.Add {
		key, err := requiredKey(npc.Name)
		if err != nil {
			return err
		}
		if _, exists := index[key]; exists {
			return fmt.Errorf("cannot add existing npc %q", npc.Name)
		}
		*npcs = append(*npcs, cloneNPC(npc))
		index[key] = len(*npcs) - 1
	}
	for _, update := range patch.Update {
		key, err := requiredKey(update.Name)
		if err != nil {
			return err
		}
		position, exists := index[key]
		if !exists {
			return fmt.Errorf("cannot update missing npc %q", update.Name)
		}
		npc := &(*npcs)[position]
		if update.Disposition != nil {
			npc.Disposition = *update.Disposition
		}
		if update.Knowledge != nil {
			if err := applyStringSetPatch(&npc.Knowledge, update.Knowledge); err != nil {
				return fmt.Errorf("npc %q knowledge: %w", update.Name, err)
			}
		}
		if update.Goal != nil {
			npc.Goal = *update.Goal
		}
	}
	return removeByName(npcs, patch.Remove, func(npc NPC) string { return npc.Name })
}

func applyPuzzlesPatch(puzzles *[]Puzzle, solvedTypes *[]string, patch *PuzzlesPatch) error {
	if err := validatePuzzles(patch.Add, *solvedTypes); err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if err := validateOperations(
		namesOf(patch.Add, func(puzzle Puzzle) string { return puzzle.Name }),
		namesOf(patch.Update, func(puzzle PuzzlePatch) string { return puzzle.Name }),
		append(append([]string(nil), patch.Remove...), patch.Solve...),
	); err != nil {
		return err
	}
	index, err := indexByName(*puzzles, func(puzzle Puzzle) string { return puzzle.Name })
	if err != nil {
		return err
	}
	for _, puzzle := range patch.Add {
		key, err := requiredKey(puzzle.Name)
		if err != nil {
			return err
		}
		if _, exists := index[key]; exists {
			return fmt.Errorf("cannot add existing puzzle %q", puzzle.Name)
		}
		*puzzles = append(*puzzles, clonePuzzle(puzzle))
		index[key] = len(*puzzles) - 1
	}
	for _, update := range patch.Update {
		key, err := requiredKey(update.Name)
		if err != nil {
			return err
		}
		position, exists := index[key]
		if !exists {
			return fmt.Errorf("cannot update missing puzzle %q", update.Name)
		}
		puzzle := &(*puzzles)[position]
		if update.Type != nil {
			if *update.Type != puzzle.Type {
				return fmt.Errorf("cannot change type of existing puzzle %q", update.Name)
			}
		}
		if update.Description != nil {
			puzzle.Description = *update.Description
		}
		if update.Status != nil {
			puzzle.Status = *update.Status
		}
		if update.SolutionHints != nil {
			if !slices.Equal(*update.SolutionHints, puzzle.SolutionHints) {
				return fmt.Errorf("cannot change solution hints of existing puzzle %q", update.Name)
			}
		}
	}
	for _, name := range patch.Solve {
		key, err := requiredKey(name)
		if err != nil {
			return err
		}
		position, exists := index[key]
		if !exists {
			return fmt.Errorf("cannot solve missing puzzle %q", name)
		}
		if err := applyStringSetPatch(solvedTypes, &StringSetPatch{Add: []string{(*puzzles)[position].Type}}); err != nil {
			return fmt.Errorf("solve puzzle %q: %w", name, err)
		}
	}
	return removeByName(puzzles, append(append([]string(nil), patch.Remove...), patch.Solve...), func(puzzle Puzzle) string { return puzzle.Name })
}

func applyNounsPatch(nouns *[]ProperNoun, patch *NounsPatch) error {
	if err := validateNouns(patch.Add); err != nil {
		return fmt.Errorf("add: %w", err)
	}
	index, err := indexByName(*nouns, func(noun ProperNoun) string { return noun.Noun })
	if err != nil {
		return err
	}
	for _, noun := range patch.Add {
		key, err := requiredKey(noun.Noun)
		if err != nil {
			return err
		}
		if position, exists := index[key]; exists {
			existing := (*nouns)[position]
			if existing.PhraseUsed != noun.PhraseUsed || existing.Description != noun.Description {
				return fmt.Errorf("cannot redefine existing noun %q", noun.Noun)
			}
			continue
		}
		*nouns = append(*nouns, noun)
		index[key] = len(*nouns) - 1
	}
	return nil
}

func applyStringSetPatch(values *[]string, patch *StringSetPatch) error {
	current := make(map[string]int, len(*values))
	for i, value := range *values {
		key, err := requiredKey(value)
		if err != nil {
			return err
		}
		if _, duplicate := current[key]; duplicate {
			return fmt.Errorf("existing value %q is duplicated", value)
		}
		current[key] = i
	}
	operations := make(map[string]string)
	for _, value := range patch.Add {
		key, err := requiredKey(value)
		if err != nil {
			return err
		}
		if operation, duplicate := operations[key]; duplicate {
			return fmt.Errorf("value %q has conflicting %s and add operations", value, operation)
		}
		operations[key] = "add"
		if _, exists := current[key]; !exists {
			*values = append(*values, value)
			current[key] = len(*values) - 1
		}
	}
	remove := make(map[string]struct{})
	for _, value := range patch.Remove {
		key, err := requiredKey(value)
		if err != nil {
			return err
		}
		if operation, duplicate := operations[key]; duplicate {
			return fmt.Errorf("value %q has conflicting %s and remove operations", value, operation)
		}
		operations[key] = "remove"
		if _, exists := current[key]; !exists {
			return fmt.Errorf("cannot remove missing value %q", value)
		}
		remove[key] = struct{}{}
	}
	if len(remove) > 0 {
		filtered := (*values)[:0]
		for _, value := range *values {
			if _, removed := remove[entityKey(value)]; !removed {
				filtered = append(filtered, value)
			}
		}
		*values = filtered
	}
	return nil
}

func validateOperations(add, update, remove []string) error {
	operations := make(map[string]string, len(add)+len(update)+len(remove))
	for operation, names := range map[string][]string{"add": add, "update": update, "remove": remove} {
		for _, name := range names {
			key, err := requiredKey(name)
			if err != nil {
				return err
			}
			if previous, duplicate := operations[key]; duplicate {
				return fmt.Errorf("%q has conflicting %s and %s operations", name, previous, operation)
			}
			operations[key] = operation
		}
	}
	return nil
}

func indexByName[T any](values []T, name func(T) string) (map[string]int, error) {
	index := make(map[string]int, len(values))
	for i, value := range values {
		key, err := requiredKey(name(value))
		if err != nil {
			return nil, err
		}
		if _, duplicate := index[key]; duplicate {
			return nil, fmt.Errorf("existing name %q is duplicated", name(value))
		}
		index[key] = i
	}
	return index, nil
}

func removeByName[T any](values *[]T, names []string, name func(T) string) error {
	if len(names) == 0 {
		return nil
	}
	index, err := indexByName(*values, name)
	if err != nil {
		return err
	}
	remove := make(map[string]struct{}, len(names))
	for _, value := range names {
		key, err := requiredKey(value)
		if err != nil {
			return err
		}
		if _, exists := index[key]; !exists {
			return fmt.Errorf("cannot remove missing %q", value)
		}
		remove[key] = struct{}{}
	}
	filtered := (*values)[:0]
	for _, value := range *values {
		if _, removed := remove[entityKey(name(value))]; !removed {
			filtered = append(filtered, value)
		}
	}
	*values = filtered
	return nil
}

func namesOf[T any](values []T, name func(T) string) []string {
	names := make([]string, 0, len(values))
	for _, value := range values {
		names = append(names, name(value))
	}
	return names
}

func requiredKey(value string) (string, error) {
	key := entityKey(value)
	if key == "" {
		return "", fmt.Errorf("name or value cannot be empty")
	}
	return key, nil
}

func entityKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cloneGameState(current *GameState) *GameState {
	next := *current
	next.PlayerStatus.Conditions = append([]string(nil), current.PlayerStatus.Conditions...)
	next.Inventory = make([]Item, len(current.Inventory))
	for i, item := range current.Inventory {
		next.Inventory[i] = cloneItem(item)
	}
	next.Environment.Exits = make(map[string]string, len(current.Environment.Exits))
	for direction, destination := range current.Environment.Exits {
		next.Environment.Exits[direction] = destination
	}
	next.Environment.WorldObjects = make([]WorldObject, len(current.Environment.WorldObjects))
	for i, object := range current.Environment.WorldObjects {
		next.Environment.WorldObjects[i] = cloneWorldObject(object)
	}
	next.NPCs = make([]NPC, len(current.NPCs))
	for i, npc := range current.NPCs {
		next.NPCs[i] = cloneNPC(npc)
	}
	next.Puzzles = make([]Puzzle, len(current.Puzzles))
	for i, puzzle := range current.Puzzles {
		next.Puzzles[i] = clonePuzzle(puzzle)
	}
	next.ProperNouns = append([]ProperNoun(nil), current.ProperNouns...)
	next.WinConditions = append([]string(nil), current.WinConditions...)
	next.LossConditions = append([]string(nil), current.LossConditions...)
	next.SolvedPuzzleTypes = append([]string(nil), current.SolvedPuzzleTypes...)
	return &next
}

func cloneItem(item Item) Item {
	item.Properties = append([]string(nil), item.Properties...)
	return item
}

func cloneWorldObject(object WorldObject) WorldObject {
	object.Properties = append([]string(nil), object.Properties...)
	return object
}

func cloneNPC(npc NPC) NPC {
	npc.Knowledge = append([]string(nil), npc.Knowledge...)
	return npc
}

func clonePuzzle(puzzle Puzzle) Puzzle {
	puzzle.SolutionHints = append([]string(nil), puzzle.SolutionHints...)
	return puzzle
}
