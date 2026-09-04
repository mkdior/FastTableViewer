package app

import (
	"strings"
	"unicode"
)

type sepDetector struct {
}

// Fast separator detection algorithm with improved heuristics
// Key improvements:
// 1. Priority-based candidate selection
// 2. Early exit for common separators
// 3. Optimized character counting
// 4. Better validation logic

func (sd *sepDetector) sepDetect(s []string) rune {
	if len(s) < 1 {
		return 0
	}

	// Fast path: a common separator that yields the same number of fields on
	// every line. When several do (a TSV whose one comma per line is a decimal
	// mark), the one that yields the most fields is the delimiter.
	var best rune
	bestCount := 0
	for _, sep := range commonSeparators {
		if sd.isValidSeparator(s, sep) {
			if c := countUnquoted(s[0], sep); c > bestCount {
				best, bestCount = sep, c
			}
		}
	}
	if best != 0 {
		return best
	}

	return sd.detectBestSeparator(s)
}

// Fast validation: Check if a separator is valid for all lines
func (sd *sepDetector) isValidSeparator(lines []string, sep rune) bool {
	if len(lines) == 0 {
		return false
	}

	// Count separator occurrences in first line
	firstCount := countUnquoted(lines[0], sep)
	if firstCount == 0 {
		return false // Separator not found
	}

	// Verify all lines have same count
	for i := 1; i < len(lines); i++ {
		if countUnquoted(lines[i], sep) != firstCount {
			return false
		}
	}

	return true
}

// countUnquoted counts r outside double-quoted fields, so a character that
// only occurs inside quotes ("a;b;c",1) is not mistaken for the delimiter.
// A line with an unbalanced quote (5'10") is counted as plain text.
func countUnquoted(s string, r rune) int {
	if strings.Count(s, "\"")%2 == 1 {
		return countRuneFast(s, r)
	}
	count, quoted := 0, false
	for _, c := range s {
		switch {
		case c == '"':
			quoted = !quoted
		case c == r && !quoted:
			count++
		}
	}
	return count
}

// Optimized rune counter - much faster than strings.Count for single runes
func countRuneFast(s string, r rune) int {
	count := 0
	for _, c := range s {
		if c == r {
			count++
		}
	}
	return count
}

// minSeparatorPresence is the share of sample lines a delimiter must appear
// on. Ragged files (columns pasted onto some lines, a stray note line) keep
// their delimiter on nearly every line even when the field counts differ.
const minSeparatorPresence = 0.8

// detectBestSeparator scores every candidate that appears on nearly every
// line. A delimiter produces many fields per line, so the average number of
// occurrences carries most weight; agreement between lines on that number
// adds up to half again; common delimiters are favoured and spaces
// penalised. Ties fall back to the fixed priority of scoreSeparator.
func (sd *sepDetector) detectBestSeparator(lines []string) rune {
	if len(lines) == 0 {
		return 0
	}

	seen := make(map[rune]bool)
	candidates := make([]rune, 0, 8)
	for _, r := range append(append([]rune{}, commonSeparators...), sd.getCandidates(lines[0])...) {
		if !seen[r] {
			seen[r] = true
			candidates = append(candidates, r)
		}
	}

	var best rune
	bestScore, bestPriority := 0.0, 0
	n := float64(len(lines))
	for _, sep := range candidates {
		present, total := 0, 0
		perCount := make(map[int]int)
		for _, line := range lines {
			c := countUnquoted(line, sep)
			total += c
			if c > 0 {
				present++
				perCount[c]++
			}
		}
		if float64(present)/n < minSeparatorPresence {
			continue
		}
		modal, modalCount := 0, 0
		for c, occurrences := range perCount {
			if occurrences > modal || (occurrences == modal && c > modalCount) {
				modal, modalCount = occurrences, c
			}
		}
		consistency := float64(modal) / float64(present)
		avg := float64(total) / n
		weight := 1.0
		switch sep {
		case ',', '\t', '|', ';':
			weight = 1.5
		case ' ':
			weight = 0.5
		}
		score := avg * (0.5 + 0.5*consistency) * weight
		priority := sd.scoreSeparator(sep, modalCount)
		if score > bestScore || (score == bestScore && priority > bestPriority) {
			best, bestScore, bestPriority = sep, score, priority
		}
	}
	return best
}

// Get candidate separators from first line
func (sd *sepDetector) getCandidates(line string) []rune {
	// Use map for deduplication
	seen := make(map[rune]bool)
	var candidates []rune

	// Priority characters to check first
	priority := []rune{',', '\t', '|', ';', ':', ' '}
	for _, r := range priority {
		if strings.ContainsRune(line, r) && !seen[r] {
			seen[r] = true
			candidates = append(candidates, r)
		}
	}

	// Check other non-alphanumeric characters
	for _, r := range line {
		if seen[r] || unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		// Skip quotes and other problematic chars
		if r == '"' || r == '\'' || r == '\\' {
			continue
		}
		seen[r] = true
		candidates = append(candidates, r)
	}

	return candidates
}

// Score separator quality (higher is better)
func (sd *sepDetector) scoreSeparator(sep rune, count int) int {
	score := 0

	// Prefer common separators
	switch sep {
	case ',':
		score += 1000 // Highest priority
	case '\t':
		score += 900
	case '|':
		score += 800
	case ';':
		score += 700
	case ':':
		score += 600
	case ' ':
		score += 100 // Lowest priority (can be ambiguous)
	default:
		score += 500 // Moderate priority for other chars
	}

	// Prefer separators with reasonable column counts (2-100)
	if count >= 2 && count <= 100 {
		score += count * 10
	} else if count > 100 {
		score -= 100 // Penalize too many columns
	}

	return score
}

// remove duplication item in []rune
func uniqueChar(intSlice []rune) []rune {
	keys := make(map[rune]bool)
	var list []rune
	for _, entry := range intSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// check if all item in []int is equal, false for empty array
func allIntItemEqual(r []int) bool {
	if len(r) == 0 {
		return false
	}
	for i := 1; i < len(r); i++ {
		if r[i] != r[0] {
			return false
		}
	}
	return true
}
