package app

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// stringInterner provides efficient string deduplication for categorical data
type stringInterner struct {
	pool sync.Map // map[string]string for concurrent access
}

// newStringInterner creates a new string interner
func newStringInterner() *stringInterner {
	return &stringInterner{}
}

// intern returns a canonical version of the string, reducing memory usage
func (si *stringInterner) intern(s string) string {
	// Quick path for empty strings
	if s == "" {
		return s
	}

	// Try to load existing string
	if existing, ok := si.pool.Load(s); ok {
		return existing.(string)
	}

	// Store and return the string
	si.pool.Store(s, s)
	return s
}

// shouldInternColumn determines if a column should use string interning
// based on its cardinality (ratio of unique values to total values)
func shouldInternColumn(values []string, threshold float64) bool {
	if len(values) < 100 {
		return false // Too small to benefit
	}

	// Sample the column to estimate cardinality
	sampleSize := 1000
	if len(values) < sampleSize {
		sampleSize = len(values)
	}

	seen := make(map[string]bool, sampleSize)
	for i := 0; i < sampleSize; i++ {
		seen[values[i]] = true
	}

	cardinality := float64(len(seen)) / float64(sampleSize)
	return cardinality < threshold // Low cardinality = good for interning
}

// errMemoryLimit is returned by contAppendSli when the configured --memory cap would be exceeded.
// Loaders stop reading at this point and keep the rows loaded so far.
var errMemoryLimit = errors.New("memory limit reached")

// Buffer represents a table data structure with concurrent access support
type Buffer struct {
	sep          rune              // Column separator character
	cont         [][]string        // Table content (rows x columns)
	colType      []int             // Column data types (colTypeStr or colTypeFloat)
	rowLen       int               // Number of rows
	colLen       int               // Number of columns
	rowFreeze    int               // Number of frozen header rows (0 or 1)
	colFreeze    int               // Number of frozen columns (0 or 1)
	selectedCell [][]int           // Selected cell coordinates
	mu           sync.RWMutex      // Mutex for concurrent access
	interners    []*stringInterner // String interners per column (nil if not used)
	internCols   []bool            // Track which columns use interning
	memoryUsage  int64             // Current estimated memory usage in bytes
	maxMemory    int64             // Maximum allowed memory in bytes (0 = no limit)
}

const (
	// Pre-allocated capacity for rows (optimized for large files)
	defaultRowCapacity = 10000
	// Cardinality threshold for string interning (30% unique values)
	internCardinalityThreshold = 0.30
	// Default memory limit: 0 = unlimited (users can set custom limit with --memory flag)
	defaultMaxMemoryBytes = 0
	// Estimated overhead per string in bytes (header + pointer + padding)
	stringOverheadBytes = 24
)

// createNewBuffer initializes and returns a new empty Buffer
func createNewBuffer() *Buffer {
	return &Buffer{
		sep:          0,
		cont:         [][]string{},
		colType:      []int{},
		rowLen:       0,
		colLen:       0,
		rowFreeze:    1,
		colFreeze:    1,
		selectedCell: [][]int{},
		interners:    nil,
		internCols:   nil,
		memoryUsage:  0,
		maxMemory:    defaultMaxMemoryBytes,
	}
}

// createNewBufferWithData creates a Buffer from existing data
func createNewBufferWithData(ss [][]string, strict bool) (*Buffer, error) {
	b = createNewBuffer()
	for _, s := range ss {
		if err := b.contAppendSli(s, strict); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// contAppendSli appends a row to the buffer
// strict: if true, enforces consistent column count
func (b *Buffer) contAppendSli(s []string, strict bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Initialize on first row
	if b.rowLen == 0 {
		b.colLen = len(s)
		b.colType = make([]int, b.colLen+1)
		// Pre-allocate capacity to reduce reallocations
		if cap(b.cont) == 0 {
			b.cont = make([][]string, 0, defaultRowCapacity)
		}
	}

	// Check memory limit before adding row
	rowSize := b.estimateRowSize(s)
	if b.maxMemory > 0 && b.memoryUsage+rowSize > b.maxMemory {
		return fmt.Errorf("%w (limit %s, loaded %s)", errMemoryLimit, formatBytes(b.maxMemory), formatBytes(b.memoryUsage))
	}

	// Strict mode: enforce column count
	if strict && len(s) != b.colLen {
		return errors.New("Row " + I2S(b.rowLen+b.rowFreeze) + " lacks some columns")
	}

	b.cont = append(b.cont, s)
	b.memoryUsage += rowSize

	// Keep every row at least colLen wide: a longer row widens the table and
	// pads all earlier rows, a shorter row is padded on its own.
	if len(s) > b.colLen {
		b.resizeColUnsafe(len(s))
	} else if len(s) < b.colLen {
		b.padRowUnsafe(len(b.cont) - 1)
	}
	b.rowLen++

	return nil
}

// estimateRowSize estimates memory usage for a row in bytes
func (b *Buffer) estimateRowSize(row []string) int64 {
	size := int64(len(row) * 8) // Slice overhead (pointers)
	for _, s := range row {
		size += int64(len(s)) + stringOverheadBytes
	}
	return size
}

// getMemoryUsage returns current estimated memory usage
func (b *Buffer) getMemoryUsage() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.memoryUsage
}

// getMemoryLimit returns the configured memory limit
func (b *Buffer) getMemoryLimit() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.maxMemory
}

// setMemoryLimit sets the maximum memory limit in bytes (0 = no limit)
func (b *Buffer) setMemoryLimit(bytes int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maxMemory = bytes
}

// getMemoryStats returns memory usage statistics
func (b *Buffer) getMemoryStats() map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["current_bytes"] = b.memoryUsage
	stats["current_formatted"] = formatBytes(b.memoryUsage)
	stats["limit_bytes"] = b.maxMemory
	stats["limit_formatted"] = formatBytes(b.maxMemory)

	if b.maxMemory > 0 {
		stats["usage_percent"] = float64(b.memoryUsage) / float64(b.maxMemory) * 100.0
		stats["available_bytes"] = b.maxMemory - b.memoryUsage
		stats["available_formatted"] = formatBytes(b.maxMemory - b.memoryUsage)
	} else {
		stats["usage_percent"] = 0.0
		stats["unlimited"] = true
	}

	return stats
}

// formatBytes formats bytes into human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return strconv.FormatInt(bytes, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	return strconv.FormatFloat(float64(bytes)/float64(div), 'f', 2, 64) + " " + units[exp]
}

// resizeColUnsafe grows the column count to n and pads every shorter row with
// "NaN" (must be called with lock held). The column count never shrinks.
func (b *Buffer) resizeColUnsafe(n int) {
	if n <= 0 {
		return
	}

	if n > b.colLen {
		oldColLen := b.colLen
		b.colLen = n

		// Resize colType array if needed
		if len(b.colType) < n+1 {
			newColType := make([]int, n+1)
			copy(newColType, b.colType)
			b.colType = newColType
		}

		// Initialize new column types to colTypeStr (default)
		for i := oldColLen; i < n; i++ {
			b.colType[i] = colTypeStr
		}
	}

	for ii := range b.cont {
		b.padRowUnsafe(ii)
	}
}

// padRowUnsafe appends "NaN" to row i until it is colLen wide (lock must be held)
func (b *Buffer) padRowUnsafe(i int) {
	for len(b.cont[i]) < b.colLen {
		b.cont[i] = append(b.cont[i], "NaN")
	}
}

// resizeCol adjusts the number of columns (thread-safe)
func (b *Buffer) resizeCol(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resizeColUnsafe(n)
}

// sortByStr sorts the buffer by column in string mode
// colIndex: column to sort by
// rev: true for descending, false for ascending
func (b *Buffer) sortByStr(colIndex int, rev bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	hasHeader := I2B(b.rowFreeze)

	if rev {
		// Descending sort
		if hasHeader {
			sort.SliceStable(b.cont[1:], func(i, j int) bool {
				return b.cont[1:][i][colIndex] > b.cont[1:][j][colIndex]
			})
		} else {
			sort.SliceStable(b.cont, func(i, j int) bool {
				return b.cont[i][colIndex] > b.cont[j][colIndex]
			})
		}
	} else {
		// Ascending sort
		if hasHeader {
			sort.SliceStable(b.cont[1:], func(i, j int) bool {
				return b.cont[1:][i][colIndex] < b.cont[1:][j][colIndex]
			})
		} else {
			sort.SliceStable(b.cont, func(i, j int) bool {
				return b.cont[i][colIndex] < b.cont[j][colIndex]
			})
		}
	}
}

// sortByNum sorts column by number format with optimized numeric conversion
func (b *Buffer) sortByNum(colIndex int, rev bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	hasHeader := I2B(b.rowFreeze)
	dataRows := b.cont
	if hasHeader {
		dataRows = b.cont[1:]
	}

	// Create index-value pairs to sort
	type numRow struct {
		row []string
		num float64
	}

	pairs := make([]numRow, len(dataRows))
	for i := range dataRows {
		pairs[i] = numRow{
			row: dataRows[i],
			num: parseNumericValueFast(dataRows[i][colIndex]),
		}
	}

	// Sort the pairs
	if rev {
		sort.SliceStable(pairs, func(i, j int) bool {
			return pairs[i].num > pairs[j].num
		})
	} else {
		sort.SliceStable(pairs, func(i, j int) bool {
			return pairs[i].num < pairs[j].num
		})
	}

	// Copy back sorted rows
	for i := range pairs {
		dataRows[i] = pairs[i].row
	}
}

// sortByDate sorts column by date format with optimized date parsing
func (b *Buffer) sortByDate(colIndex int, rev bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	hasHeader := I2B(b.rowFreeze)
	dataRows := b.cont
	if hasHeader {
		dataRows = b.cont[1:]
	}

	// Create index-value pairs to sort
	type dateRow struct {
		row  []string
		date int64
	}

	pairs := make([]dateRow, len(dataRows))
	for i := range dataRows {
		pairs[i] = dateRow{
			row:  dataRows[i],
			date: parseDateValueFast(dataRows[i][colIndex]),
		}
	}

	// Sort the pairs
	if rev {
		sort.SliceStable(pairs, func(i, j int) bool {
			return pairs[i].date > pairs[j].date
		})
	} else {
		sort.SliceStable(pairs, func(i, j int) bool {
			return pairs[i].date < pairs[j].date
		})
	}

	// Copy back sorted rows
	for i := range pairs {
		dataRows[i] = pairs[i].row
	}
}

// parseNumericValueFast quickly parses a string to float64
// Handles commas, underscores, and returns 0 for invalid values
func parseNumericValueFast(s string) float64 {
	// Remove common separators
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.TrimSpace(s)

	if s == "" || s == "NA" || s == "N/A" || s == "NaN" || s == "null" {
		return 0
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}

// parseDateValueFast parses a date string to a unix timestamp for sorting,
// returning 0 for values that are not dates. Use parseDate where the
// difference between the epoch and an invalid value matters.
func parseDateValueFast(s string) int64 {
	ts, _ := parseDate(s)
	return ts
}

// parseDate parses a date string in one of the supported formats and reports
// whether it was a date at all, so 1970-01-01 (timestamp 0) stays valid.
func parseDate(s string) (int64, bool) {
	s = strings.TrimSpace(s)

	// Fast rejection checks
	if s == "" || s == "NA" || s == "N/A" || s == "null" {
		return 0, false
	}

	// Dates are typically 8-30 characters
	if len(s) < 8 || len(s) > 30 {
		return 0, false
	}

	// Must contain date separators
	if !strings.ContainsAny(s, "-/.:T ") {
		return 0, false
	}

	// Must contain at least one digit
	hasDigit := false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return 0, false
	}

	// Try common date formats (most common first for performance)
	formats := []string{
		"2006-01-02",          // ISO date: 2024-10-17
		"2006-01-02 15:04:05", // ISO datetime: 2024-10-17 15:30:00
		"01/02/2006",          // US date: 10/17/2024
		"02/01/2006",          // EU date: 17/10/2024
		"2006/01/02",          // Alt ISO: 2024/10/17
		time.RFC3339,          // RFC3339: 2024-10-17T15:30:00Z
		"2006-01-02T15:04:05", // ISO8601 without timezone
		"Jan 02, 2006",        // Mon DD, YYYY
		"January 02, 2006",    // Month DD, YYYY
		"02-Jan-2006",         // DD-Mon-YYYY
		"02 Jan 2006",         // DD Mon YYYY
		"2006.01.02",          // Dotted date
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t.Unix(), true
		}
	}

	return 0, false
}

// getCol returns the ith column data as a string slice
// Uses pointer receiver to avoid copying mutex
func (b *Buffer) getCol(i int) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]string, b.rowLen)
	for rowI := 0; rowI < b.rowLen; rowI++ {
		result[rowI] = b.cont[rowI][i]
	}
	return result
}

// set ith column data type
func (b *Buffer) setColType(i int, t int) {
	b.colType[i] = t
}

// get ith column data type
func (b *Buffer) getColType(i int) int {
	return b.colType[i]
}

// autoDetectColumnType intelligently detects if a column contains numeric, date, or string data
// Returns colTypeDate for dates, colTypeFloat for numbers, colTypeStr for strings
func (b *Buffer) autoDetectColumnType(colIndex int) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if colIndex < 0 || colIndex >= b.colLen {
		return colTypeStr
	}

	// Sample size for type detection
	startRow := b.rowFreeze
	endRow := b.rowLen

	// For large datasets, sample smartly (first N rows + some middle + last N)
	sampleSize := 100
	sampleRows := []int{}

	if endRow-startRow > sampleSize {
		// Sample first 50 rows
		for i := startRow; i < startRow+50 && i < endRow; i++ {
			sampleRows = append(sampleRows, i)
		}
		// Sample middle 25 rows
		midPoint := (startRow + endRow) / 2
		for i := midPoint; i < midPoint+25 && i < endRow; i++ {
			sampleRows = append(sampleRows, i)
		}
		// Sample last 25 rows
		for i := endRow - 25; i < endRow; i++ {
			if i > startRow {
				sampleRows = append(sampleRows, i)
			}
		}
	} else {
		// For small datasets, check all rows
		for i := startRow; i < endRow; i++ {
			sampleRows = append(sampleRows, i)
		}
	}

	// Analyze samples
	dateCount := 0
	numericCount := 0
	totalCount := 0

	for _, rowIdx := range sampleRows {
		if rowIdx >= b.rowLen || colIndex >= len(b.cont[rowIdx]) {
			continue
		}

		value := strings.TrimSpace(b.cont[rowIdx][colIndex])

		// Skip empty/null cells
		if value == "" || value == "NA" || value == "N/A" || value == "NaN" || value == "null" {
			continue
		}

		totalCount++

		// Check if it's a date (dates are more specific than numbers)
		if isDateValue(value) {
			dateCount++
		} else if isNumericValue(value) {
			numericCount++
		}
	}

	// If no valid values, treat as string
	if totalCount == 0 {
		return colTypeStr
	}

	// Threshold: 90% of values must match type
	threshold := float64(totalCount) * 0.90

	// Priority: Date > Number > String
	if float64(dateCount) >= threshold {
		return colTypeDate
	} else if float64(numericCount) >= threshold {
		return colTypeFloat
	}

	return colTypeStr
}

// isDateValue checks if a string represents a valid date with fast pre-checks
func isDateValue(s string) bool {
	if len(s) == 0 {
		return false
	}

	// Fast rejection: dates are typically 8-30 characters
	if len(s) < 8 || len(s) > 30 {
		return false
	}

	// Quick heuristic checks before trying to parse
	// Dates typically contain: -, /, :, T, or spaces with commas (for month names)
	hasDateSep := strings.ContainsAny(s, "-/.:T") || (strings.Contains(s, " ") && strings.Contains(s, ","))
	if !hasDateSep {
		return false
	}

	// Must contain at least one digit
	hasDigit := false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return false
	}

	// Common date formats (most common first for performance)
	formats := []string{
		"2006-01-02",          // ISO date: 2024-10-17
		"2006-01-02 15:04:05", // ISO datetime: 2024-10-17 15:30:00
		"01/02/2006",          // US date: 10/17/2024
		"02/01/2006",          // EU date: 17/10/2024
		"2006/01/02",          // Alt ISO: 2024/10/17
		time.RFC3339,          // RFC3339: 2024-10-17T15:30:00Z
		"2006-01-02T15:04:05", // ISO8601 without timezone
		"Jan 02, 2006",        // Mon DD, YYYY
		"January 02, 2006",    // Month DD, YYYY
		"02-Jan-2006",         // DD-Mon-YYYY
		"02 Jan 2006",         // DD Mon YYYY
		"2006.01.02",          // Dotted date
	}

	for _, format := range formats {
		if _, err := time.Parse(format, s); err == nil {
			return true
		}
	}

	return false
}

// isNumericValue checks if a string represents a valid number
// Handles: integers, floats, scientific notation, negative numbers
func isNumericValue(s string) bool {
	if len(s) == 0 {
		return false
	}

	// Quick check for common patterns
	hasDigit := false
	hasDot := false
	hasE := false
	i := 0

	// Handle sign
	if s[i] == '+' || s[i] == '-' {
		i++
		if i >= len(s) {
			return false
		}
	}

	// Parse number
	for i < len(s) {
		c := s[i]

		if c >= '0' && c <= '9' {
			hasDigit = true
		} else if c == '.' {
			if hasDot || hasE {
				return false // Multiple dots or dot after E
			}
			hasDot = true
		} else if c == 'e' || c == 'E' {
			if !hasDigit || hasE {
				return false // E without digits or multiple E
			}
			hasE = true
			hasDigit = false // Reset for exponent part

			// Check for sign after E
			if i+1 < len(s) && (s[i+1] == '+' || s[i+1] == '-') {
				i++
			}
		} else if c == '_' || c == ',' {
			// Allow thousand separators (common in data files)
			// Skip validation, just continue
		} else {
			return false // Invalid character
		}
		i++
	}

	return hasDigit
}

// detectAllColumnTypes automatically detects types for all columns in parallel
func (b *Buffer) detectAllColumnTypes() {
	types := make([]int, b.colLen)
	var wg sync.WaitGroup

	for i := 0; i < b.colLen; i++ {
		wg.Add(1)
		go func(col int) {
			defer wg.Done()
			types[col] = b.autoDetectColumnType(col)
		}(i)
	}

	wg.Wait()

	for i, t := range types {
		b.setColType(i, t)
	}
}

// enableStringInterning analyzes columns and enables interning for low-cardinality string columns
// This can save 30-70% memory for datasets with repeated categorical values
func (b *Buffer) enableStringInterning() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.rowLen < 100 {
		return // Too small to benefit
	}

	// Initialize interning structures
	b.interners = make([]*stringInterner, b.colLen)
	b.internCols = make([]bool, b.colLen)

	// Analyze each column
	for col := 0; col < b.colLen; col++ {
		// Skip non-string columns
		if b.colType[col] != colTypeStr {
			continue
		}

		// Get column data
		colData := make([]string, b.rowLen)
		for row := 0; row < b.rowLen; row++ {
			if col < len(b.cont[row]) {
				colData[row] = b.cont[row][col]
			}
		}

		// Check if column should be interned (low cardinality)
		if shouldInternColumn(colData, internCardinalityThreshold) {
			b.interners[col] = newStringInterner()
			b.internCols[col] = true

			// Intern existing values
			for row := 0; row < b.rowLen; row++ {
				if col < len(b.cont[row]) {
					b.cont[row][col] = b.interners[col].intern(b.cont[row][col])
				}
			}
		}
	}
}

// internValue interns a string value for a specific column if interning is enabled
func (b *Buffer) internValue(col int, value string) string {
	if col < len(b.internCols) && b.internCols[col] && b.interners[col] != nil {
		return b.interners[col].intern(value)
	}
	return value
}

// getInterningStats returns statistics about string interning usage
func (b *Buffer) getInterningStats() map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["enabled"] = len(b.internCols) > 0

	if len(b.internCols) == 0 {
		return stats
	}

	internedCols := 0
	for _, enabled := range b.internCols {
		if enabled {
			internedCols++
		}
	}

	stats["total_columns"] = b.colLen
	stats["interned_columns"] = internedCols
	stats["percentage"] = float64(internedCols) / float64(b.colLen) * 100.0

	return stats
}

//clear selectedCell of buffer
//func (b *Buffer) clearSelection() {
//	b.selectedCell = [][]int{}
//}

// search string and add result to selectedCell of buffer
func (b *Buffer) selectBySearch(s string) {
	for ii, i := range b.cont {
		for ji, j := range i {
			if s == j {
				b.selectedCell = append(b.selectedCell, []int{ii, ji})
			}
		}
	}
}

// cellBlock copies the rectangle of cells rows r1..r2, columns c1..c2 (both
// inclusive, clamped to the table); missing cells in short rows are "".
func (b *Buffer) cellBlock(r1, c1, r2, c2 int) [][]string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if r1 > r2 {
		r1, r2 = r2, r1
	}
	if c1 > c2 {
		c1, c2 = c2, c1
	}
	r1, r2 = clampInt(r1, 0, b.rowLen-1), clampInt(r2, 0, b.rowLen-1)
	c1, c2 = clampInt(c1, 0, b.colLen-1), clampInt(c2, 0, b.colLen-1)
	if b.rowLen == 0 || b.colLen == 0 {
		return nil
	}
	out := make([][]string, 0, r2-r1+1)
	for r := r1; r <= r2; r++ {
		row := make([]string, 0, c2-c1+1)
		for c := c1; c <= c2; c++ {
			if c < len(b.cont[r]) {
				row = append(row, b.cont[r][c])
			} else {
				row = append(row, "")
			}
		}
		out = append(out, row)
	}
	return out
}

// FilterOptions defines the parameters for a column filter.
type FilterOptions struct {
	Query         string
	Operator      string
	CaseSensitive bool
}

// numericOperators are the filter operators that compare values as numbers (or dates on date columns).
var numericOperators = map[string]bool{">": true, "<": true, ">=": true, "<=": true}

// parseNumberStrict parses a cell as a float64, tolerating thousands separators
// and underscores. Unlike parseNumericValueFast it reports failure instead of
// returning 0, so unparseable cells can be excluded rather than treated as zero.
func parseNumberStrict(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if strings.ContainsAny(s, ",_") {
		s = strings.NewReplacer(",", "", "_", "").Replace(s)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// compiledFilter is a FilterOptions prepared once so it can be evaluated
// against every row without recompiling regexes or reparsing thresholds.
type compiledFilter struct {
	operator  string
	query     string // lower-cased when the filter is case-insensitive
	caseSens  bool
	re        *regexp.Regexp // set for the regex operator
	threshold float64        // parsed query for numeric/date operators
	asDate    bool           // compare as dates rather than numbers
	valid     bool           // false when the query is unusable (bad regex, non-numeric threshold)
}

// compileFilter prepares options for repeated evaluation against a column of the given type.
func compileFilter(options FilterOptions, colType int) compiledFilter {
	f := compiledFilter{operator: options.Operator, query: options.Query, caseSens: options.CaseSensitive, valid: true}

	switch {
	case numericOperators[f.operator]:
		if colType == colTypeDate {
			f.asDate = true
			ts, ok := parseDate(options.Query)
			f.threshold, f.valid = float64(ts), ok
		} else {
			f.threshold, f.valid = parseNumberStrict(options.Query)
		}
	case f.operator == "regex":
		pattern := options.Query
		if !options.CaseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		f.re, f.valid = re, err == nil
	default:
		if !options.CaseSensitive {
			f.query = strings.ToLower(f.query)
		}
	}
	return f
}

// match reports whether a single cell satisfies the filter.
func (f *compiledFilter) match(cellValue string) bool {
	if !f.valid {
		return false
	}

	if numericOperators[f.operator] {
		var v float64
		if f.asDate {
			ts, ok := parseDate(cellValue)
			if !ok {
				return false
			}
			v = float64(ts)
		} else {
			var ok bool
			if v, ok = parseNumberStrict(cellValue); !ok {
				return false
			}
		}
		switch f.operator {
		case ">":
			return v > f.threshold
		case "<":
			return v < f.threshold
		case ">=":
			return v >= f.threshold
		default: // "<="
			return v <= f.threshold
		}
	}

	if f.operator == "regex" {
		return f.re.MatchString(cellValue)
	}

	cell := cellValue
	if !f.caseSens {
		cell = strings.ToLower(cell)
	}
	switch f.operator {
	case "equals":
		return cell == f.query
	case "starts with":
		return strings.HasPrefix(cell, f.query)
	case "ends with":
		return strings.HasSuffix(cell, f.query)
	default:
		// "contains", and the historical default for an empty operator
		return strings.Contains(cell, f.query)
	}
}

// filterByColumn filters rows based on column value using the provided options.
// It returns a new buffer containing the filtered rows.
func (b *Buffer) filterByColumn(colIndex int, options FilterOptions) *Buffer {
	b.mu.RLock()
	defer b.mu.RUnlock()

	filtered := createNewBuffer()
	filtered.sep = b.sep
	filtered.colLen = b.colLen
	filtered.rowFreeze = b.rowFreeze
	filtered.colFreeze = b.colFreeze
	filtered.colType = make([]int, len(b.colType))
	copy(filtered.colType, b.colType)

	// Pre-allocate with estimated capacity (assume ~25% match rate)
	estimatedCapacity := (b.rowLen - b.rowFreeze) / 4
	if estimatedCapacity < 100 {
		estimatedCapacity = 100
	}
	filtered.cont = make([][]string, 0, estimatedCapacity)

	// Add header row if present
	if b.rowFreeze > 0 && b.rowLen > 0 {
		filtered.cont = append(filtered.cont, b.cont[0])
		filtered.rowLen = 1
	}

	// Early exit if column index is invalid - but still return buffer with header
	if colIndex >= b.colLen {
		return filtered
	}

	colType := colTypeStr
	if colIndex < len(b.colType) {
		colType = b.colType[colIndex]
	}
	filter := compileFilter(options, colType)

	// Filter data rows
	for i := b.rowFreeze; i < b.rowLen; i++ {
		if colIndex >= len(b.cont[i]) {
			continue
		}
		if filter.match(b.cont[i][colIndex]) {
			filtered.cont = append(filtered.cont, b.cont[i])
			filtered.rowLen++
		}
	}

	return filtered
}

// evaluateFilter checks if a single cell value matches the filter options.
// Prefer compileFilter when evaluating many cells.
func evaluateFilter(cellValue string, options FilterOptions, colType int) bool {
	f := compileFilter(options, colType)
	return f.match(cellValue)
}
