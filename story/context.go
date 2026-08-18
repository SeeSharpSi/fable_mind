package story

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const recentContextNouns = 8

// BuildModelContext returns a detached state view with only relevant glossary entries.
func BuildModelContext(state *GameState, userAction string) *GameState {
	if state == nil {
		return nil
	}
	contextState := cloneGameState(state)
	if len(state.ProperNouns) <= recentContextNouns {
		return contextState
	}

	searchText := modelContextSearchText(state, userAction)
	searchWords := contextWordSet(searchText)
	nounWordCounts := glossaryWordCounts(state.ProperNouns)
	selected := make(map[int]struct{})
	for i, noun := range state.ProperNouns {
		if containsReference(searchText, searchWords, nounWordCounts, noun.Noun) || containsReference(searchText, searchWords, nounWordCounts, noun.PhraseUsed) {
			selected[i] = struct{}{}
		}
	}
	for i := max(len(state.ProperNouns)-recentContextNouns, 0); i < len(state.ProperNouns); i++ {
		selected[i] = struct{}{}
	}

	contextState.ProperNouns = make([]ProperNoun, 0, len(selected))
	for i, noun := range state.ProperNouns {
		if _, include := selected[i]; include {
			contextState.ProperNouns = append(contextState.ProperNouns, noun)
		}
	}
	return contextState
}

func modelContextSearchText(state *GameState, userAction string) string {
	var text strings.Builder
	writeContextText := func(values ...string) {
		for _, value := range values {
			text.WriteByte(' ')
			text.WriteString(value)
		}
	}

	writeContextText(userAction, state.Environment.LocationName, state.Environment.Description)
	for direction, destination := range state.Environment.Exits {
		writeContextText(direction, destination)
	}
	for _, item := range state.Inventory {
		writeContextText(item.Name, item.Description, item.State, strings.Join(item.Properties, " "))
	}
	for _, object := range state.Environment.WorldObjects {
		writeContextText(object.Name, object.State, strings.Join(object.Properties, " "))
	}
	for _, npc := range state.NPCs {
		writeContextText(npc.Name, npc.Disposition, npc.Goal, strings.Join(npc.Knowledge, " "))
	}
	for _, puzzle := range state.Puzzles {
		writeContextText(puzzle.Name, puzzle.Type, puzzle.Description, puzzle.Status, strings.Join(puzzle.SolutionHints, " "))
	}
	writeContextText(strings.Join(state.PlayerStatus.Conditions, " "))
	writeContextText(strings.Join(state.WinConditions, " "), strings.Join(state.LossConditions, " "))
	writeContextText(strings.Join(state.SolvedPuzzleTypes, " "), state.Rules.ConsequenceModel)
	return strings.ToLower(text.String())
}

func containsReference(haystack string, words map[string]struct{}, glossaryCounts map[string]int, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	if utf8.RuneCountInString(needle) >= 3 && strings.Contains(haystack, needle) {
		return true
	}

	parts := contextWords(needle)
	for _, part := range parts {
		_, exactWord := words[part]
		if !exactWord {
			continue
		}
		if glossaryCounts[part] == 1 && (len(parts) == 1 || utf8.RuneCountInString(part) >= 4) {
			return true
		}
	}
	return false
}

func glossaryWordCounts(nouns []ProperNoun) map[string]int {
	counts := make(map[string]int)
	for _, noun := range nouns {
		words := contextWordSet(noun.Noun + " " + noun.PhraseUsed)
		for word := range words {
			counts[word]++
		}
	}
	return counts
}

func contextWordSet(value string) map[string]struct{} {
	words := contextWords(value)
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		set[word] = struct{}{}
	}
	return set
}

func contextWords(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}
