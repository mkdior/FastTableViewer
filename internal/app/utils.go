package app

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/fatih/color"
)

// fatalError restores the terminal, prints err in red and exits with status 1.
// The UI must be stopped before printing or the message lands on the raw screen.
func fatalError(err error) {
	if err != nil {
		if app != nil {
			app.Stop()
		}
		color.Set(color.FgRed)
		fmt.Fprintln(os.Stderr, err)
		color.Unset()
		if !debug {
			os.Exit(1)
		}
	}
}

// print useful info and force quite app
func usefulInfo(s string) {
	color.Set(color.FgHiYellow)
	fmt.Println(s)
	color.Unset()
}

// I2B  covert int to bool, if i >0:true, else false
func I2B(i int) bool {
	return i > 0
}

// F2S covert float64 to bool
func F2S(i float64) string {
	return strconv.FormatFloat(i, 'f', 4, 64)
}

// S2F covert string to float64
func S2F(i string) float64 {
	s, err := strconv.ParseFloat(i, 64)
	if err != nil {
		fatalError(err)
	}
	return s
}

// I2S covert int to string
func I2S(i int) string {
	return strconv.Itoa(i)
}

func getHelpContent() string {
	helpContent := `[` + theme.tag(theme.Dim) + `]━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━[-:-:-]

[::b][` + theme.tag(theme.Text) + `]🚀 ftv - Fast Table Viewer[-:-:-]

[` + theme.tag(theme.Dim) + `]━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━[-:-:-]

[::b][` + theme.tag(theme.Text) + `]📖 Help Navigation[-:-:-]
  [` + theme.tag(theme.Accent) + `]j/k[-]                 Scroll help text
  [` + theme.tag(theme.Accent) + `]gg/G[-]                Jump to top/bottom
  [` + theme.tag(theme.Accent) + `]Ctrl-d/u[-]            Page down/up
  [` + theme.tag(theme.Accent) + `]? or q or Esc[-]       Close help dialog

[::b][` + theme.tag(theme.Text) + `]🚪 Quit[-:-:-]
  [` + theme.tag(theme.Accent) + `]q[-]                   Quit application
  [` + theme.tag(theme.Accent) + `]Esc[-]                 Close dialog or clear search

[::b][` + theme.tag(theme.Text) + `]⬆️ Movement[-:-:-]
  [` + theme.tag(theme.Accent) + `]h[-]                   Move left ⬅️
  [` + theme.tag(theme.Accent) + `]l[-]                   Move right ➡️
  [` + theme.tag(theme.Accent) + `]j[-]                   Move down ⬇️
  [` + theme.tag(theme.Accent) + `]k[-]                   Move up ⬆️

  [` + theme.tag(theme.Accent) + `]w[-]                   Move to next column (word forward)
  [` + theme.tag(theme.Accent) + `]b[-]                   Move to previous column (word backward)

  [` + theme.tag(theme.Accent) + `]gg[-]                  Go to first row (press g twice)
  [` + theme.tag(theme.Accent) + `]G[-]                   Go to last row

  [` + theme.tag(theme.Accent) + `]0[-]                   Go to first column
  [` + theme.tag(theme.Accent) + `]$[-]                   Go to last column

  [` + theme.tag(theme.Accent) + `]Ctrl-d[-]              Page down (half page)
  [` + theme.tag(theme.Accent) + `]Ctrl-u[-]              Page up (half page)

  [` + theme.tag(theme.Accent) + `]N + motion[-]          Repeat a motion N times: 5j, 3l, 2w, 4n
  [` + theme.tag(theme.Accent) + `]NG / Ngg[-]            Jump to row N
  [` + theme.tag(theme.Accent) + `]N Ctrl-d/u[-]          Move N rows

[::b][` + theme.tag(theme.Text) + `]🖱️  Mouse Support[-:-:-]
  [` + theme.tag(theme.Accent) + `]Left Click[-]          Select cell
  [` + theme.tag(theme.Accent) + `]Scroll Wheel[-]        Scroll up/down through rows
  [` + theme.tag(theme.Accent) + `]Click Buttons[-]       Interact with dialogs and forms

[::b][` + theme.tag(theme.Text) + `]🔍 Search[-:-:-]
  [` + theme.tag(theme.Accent) + `]/[-]                   Search for text
                    • Case-insensitive by default
                    • Press [` + theme.tag(theme.Accent) + `]Tab[-] to navigate to checkbox
                    • Press [` + theme.tag(theme.Accent) + `]Space[-] to toggle [` + theme.tag(theme.Accent) + `]Use Regex[-] option
  [` + theme.tag(theme.Accent) + `]n[-]                   Next search result ⏭
  [` + theme.tag(theme.Accent) + `]N[-]                   Previous search result ⏮
  [` + theme.tag(theme.Accent) + `]Esc[-]                 Clear search highlighting

[::b][` + theme.tag(theme.Text) + `]🎯 Regex Search Examples[-:-:-]
  [` + theme.tag(theme.Accent) + `]^start[-]              Match at beginning of cell
  [` + theme.tag(theme.Accent) + `]end$[-]                Match at end of cell
  [` + theme.tag(theme.Accent) + `]\d+[-]                 Match digits (numbers)
  [` + theme.tag(theme.Accent) + `]@.*\.com[-]            Match email pattern
  [` + theme.tag(theme.Accent) + `]word1|word2[-]         Match either word (OR)
  [` + theme.tag(theme.Accent) + `][A-Z]+[-]              Match uppercase letters

[::b][` + theme.tag(theme.Text) + `]🔎 Filter[-:-:-]
  [` + theme.tag(theme.Accent) + `]f[-]                   Filter rows by current column value
                    • Operators: contains, equals, starts/ends with, regex, >, <, >=, <=
                    • Apply filters to multiple columns (combined with AND)
                    • Edit filter: press f on filtered column
  [` + theme.tag(theme.Accent) + `]r[-]                   Remove filter from current column

[::b][` + theme.tag(theme.Text) + `]🏷️  Data Type[-:-:-]
  [` + theme.tag(theme.Accent) + `]t[-]                   Toggle column data type
                    (String → Number → Date → String)

[::b][` + theme.tag(theme.Text) + `]🔃 Sort[-:-:-]
  [` + theme.tag(theme.Accent) + `]s[-]                   Sort data by column (ascending ⬆️)
  [` + theme.tag(theme.Accent) + `]S[-]                   Sort data by column (descending ⬇️)

[::b][` + theme.tag(theme.Text) + `]📏 Text Wrapping[-:-:-]
  [` + theme.tag(theme.Accent) + `]W[-]                   Toggle width limit for current column (50 chars)
                    Long columns (>50 chars) are limited automatically

[::b][` + theme.tag(theme.Text) + `]📊 Stats[-:-:-]
  [` + theme.tag(theme.Accent) + `]i[-]                   Show stats info for current column

[::b][` + theme.tag(theme.Text) + `]❓ Help[-:-:-]
  [` + theme.tag(theme.Accent) + `]?[-]                   Show this help dialog

[` + theme.tag(theme.Dim) + `]━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━[-:-:-]

[::b][` + theme.tag(theme.Text) + `]💡 Pro Tips:[-:-:-]
  • Press [` + theme.tag(theme.Accent) + `]gg[-] to jump to the top of any table
  • Use [` + theme.tag(theme.Accent) + `]/[-] for quick searching across all cells
  • Enable [` + theme.tag(theme.Accent) + `]regex[-] mode for powerful pattern matching
  • Press [` + theme.tag(theme.Accent) + `]i[-] to see detailed statistics for any column
  • Use [` + theme.tag(theme.Accent) + `]f[-] on multiple columns to combine filters
  • Headers are frozen by default for easy navigation

[` + theme.tag(theme.Dim) + `]━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━[-:-:-]
`
	return helpContent
}

// wrapText wraps text to fit within maxWidth characters
// Returns the wrapped text with newlines
func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 || len(text) <= maxWidth {
		return text
	}

	var result []rune
	runes := []rune(text)
	lineStart := 0

	for i := 0; i < len(runes); i++ {
		// Check if we've reached the wrap point
		if i-lineStart >= maxWidth {
			// Find last space before maxWidth for word wrap
			wrapPoint := i
			for j := i; j > lineStart; j-- {
				if runes[j] == ' ' || runes[j] == '\t' || runes[j] == '-' {
					wrapPoint = j + 1
					break
				}
			}

			// If no good wrap point found, hard wrap at maxWidth
			if wrapPoint == i && i > lineStart {
				wrapPoint = lineStart + maxWidth
			}

			// Add the wrapped line
			result = append(result, runes[lineStart:wrapPoint]...)
			result = append(result, '\n')

			// Skip trailing spaces on new line
			for wrapPoint < len(runes) && (runes[wrapPoint] == ' ' || runes[wrapPoint] == '\t') {
				wrapPoint++
			}

			lineStart = wrapPoint
			i = wrapPoint - 1 // -1 because loop will increment
		}
	}

	// Add remaining text
	if lineStart < len(runes) {
		result = append(result, runes[lineStart:]...)
	}

	return string(result)
}

// truncateText truncates text to maxWidth and adds ellipsis if needed
func truncateText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}

	runes := []rune(text)
	if len(runes) <= maxWidth {
		return text
	}

	// Reserve 3 characters for ellipsis
	if maxWidth <= 3 {
		return string(runes[:maxWidth])
	}

	return string(runes[:maxWidth-3]) + "..."
}

// getColumnMaxWidth determines the maximum width for a column
func getColumnMaxWidth(colIndex int) int {
	// Default wrap width (50 characters for long columns)
	defaultWidth := 50

	// Check if custom width is set
	if width, exists := wrappedColumns[colIndex]; exists {
		return width
	}

	return defaultWidth
}

// detectAndWrapLongColumns automatically enables wrapping for columns with long content
// Analyzes first N rows to detect if columns have text longer than threshold
func detectAndWrapLongColumns(b *Buffer, sampleSize int, threshold int) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Determine how many rows to sample
	maxSample := sampleSize
	if b.rowLen < maxSample {
		maxSample = b.rowLen
	}

	// Skip header row in analysis if it exists
	startRow := 0
	if b.rowFreeze > 0 {
		startRow = b.rowFreeze
	}

	// Track maximum length found in each column
	maxLengths := make([]int, b.colLen)

	// Sample rows to find maximum content length per column
	for r := startRow; r < maxSample; r++ {
		for c := 0; c < b.colLen; c++ {
			if c < len(b.cont[r]) {
				cellLen := len(b.cont[r][c])
				if cellLen > maxLengths[c] {
					maxLengths[c] = cellLen
				}
			}
		}
	}

	// Enable wrapping for columns that exceed threshold
	for c := 0; c < b.colLen; c++ {
		if maxLengths[c] > threshold {
			// Only set if not already manually configured
			if _, exists := wrappedColumns[c]; !exists {
				wrappedColumns[c] = getColumnMaxWidth(c)
			}
		}
	}
}

// performSearch searches for a query string in the buffer and stores results
// Supports both plain text and regex search modes with parallel column scanning
func performSearch(b *Buffer, query string, useRegex bool, caseSensitive bool) []SearchResult {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Compile regex if in regex mode
	var re *regexp.Regexp
	var err error
	if useRegex {
		if !caseSensitive {
			query = "(?i)" + query
		}
		re, err = regexp.Compile(query)
		if err != nil {
			return []SearchResult{}
		}
	} else if !caseSensitive {
		query = strings.ToLower(query)
	}

	// Parallel search across columns for better performance
	resultChan := make(chan []SearchResult, b.colLen)
	var wg sync.WaitGroup

	for c := 0; c < b.colLen; c++ {
		wg.Add(1)
		go func(col int) {
			defer wg.Done()
			var colResults []SearchResult

			for r := 0; r < b.rowLen; r++ {
				cellText := b.cont[r][col]

				var matches bool
				if useRegex {
					matches = re.MatchString(cellText)
				} else {
					if caseSensitive {
						matches = strings.Contains(cellText, query)
					} else {
						matches = strings.Contains(strings.ToLower(cellText), query)
					}
				}

				if matches {
					colResults = append(colResults, SearchResult{Row: r, Col: col})
				}
			}

			resultChan <- colResults
		}(c)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results from all columns
	var results []SearchResult
	for colResults := range resultChan {
		results = append(results, colResults...)
	}

	return results
}

// toLower converts a string to lowercase using optimized stdlib
func toLower(s string) string {
	return strings.ToLower(s)
}

// makeProgressBar creates a visual progress bar
// percent should be between 0 and 100
// width is the number of characters for the bar
func makeProgressBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := int(float64(width) * percent / 100.0)
	empty := width - filled

	bar := "["
	for i := 0; i < filled; i++ {
		bar += "█"
	}
	for i := 0; i < empty; i++ {
		bar += "░"
	}
	bar += fmt.Sprintf("] %.1f%%", percent)

	return bar
}
