package main

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// errSeparatorNotDetected is returned when no delimiter could be inferred from the input.
const errSeparatorNotDetected = "ftv cannot identify the separator; set it manually with -s"

// progressTracker helps display loading progress
type progressTracker struct {
	total        int64
	current      int64
	lastUpdate   time.Time
	updateEvery  int
	lineCount    int
	showProgress bool
	startTime    time.Time
}

func newProgressTracker(total int64, showProgress bool) *progressTracker {
	return &progressTracker{
		total:        total,
		current:      0,
		lastUpdate:   time.Now(),
		updateEvery:  5000, // update every 5000 lines
		lineCount:    0,
		showProgress: showProgress,
		startTime:    time.Now(),
	}
}

func (p *progressTracker) increment(bytes int64) {
	if !p.showProgress {
		return
	}
	p.current += bytes
	p.lineCount++

	// Update display every N lines OR every 0.5 seconds (whichever comes first)
	if p.lineCount%p.updateEvery == 0 || time.Since(p.lastUpdate) > 500*time.Millisecond {
		p.display()
		p.lastUpdate = time.Now()
	}
}

func (p *progressTracker) display() {
	if !p.showProgress {
		return
	}

	elapsed := time.Since(p.startTime).Seconds()
	if elapsed == 0 {
		elapsed = 0.001 // avoid division by zero
	}
	linesPerSec := float64(p.lineCount) / elapsed

	if p.total > 0 {
		percent := float64(p.current) * 100.0 / float64(p.total)
		if percent > 100 {
			percent = 100
		}
		progressBar := makeProgressBar(percent, 20)
		fmt.Printf("\r\033[K📊 Loading: %s | %d lines | %.0f lines/sec", progressBar, p.lineCount, linesPerSec)
	} else {
		// For pipes or when size is unknown
		fmt.Printf("\r\033[K📊 Loading: %d lines | %.0f lines/sec", p.lineCount, linesPerSec)
	}
}

func (p *progressTracker) finish() {
	if !p.showProgress {
		return
	}

	elapsed := time.Since(p.startTime).Seconds()
	if elapsed == 0 {
		elapsed = 0.001
	}
	linesPerSec := float64(p.lineCount) / elapsed

	// Clear the progress line and show final summary
	fmt.Printf("\r\033[K✓ Loaded %d lines in %.2fs (%.0f lines/sec)\n", p.lineCount, elapsed, linesPerSec)
}

// load file content to buffer (async version with concurrent parsing)
func loadFileToBufferAsync(fn string, b *Buffer, updateChan chan<- bool, doneChan chan<- error) {
	totalAddedLN := 0             //the number of lines has been added into buffer
	skipRemaining := args.SkipNum //lines still to skip before reading data

	// Get file size for progress tracking
	fileInfo, err := os.Stat(fn)
	if err != nil {
		doneChan <- err
		return
	}
	var fileSize int64
	if !fileInfo.IsDir() {
		fileSize = fileInfo.Size()
	}

	// Initialize load progress
	loadProgress.TotalBytes = fileSize
	loadProgress.LoadedBytes = 0
	loadProgress.IsComplete = false

	// Create progress tracker (disabled for async loading since UI will show it)
	progress := newProgressTracker(fileSize, false)

	scanner, err := getFileScanner(fn)
	if err != nil {
		doneChan <- err
		return
	}
	scanner.Split(bufio.ScanLines)
	//set separator, if user does not provide it.
	var detectLines []string //lines as detect separator data
	if b.sep == 0 {
		//read 10 lines to detect separator
		lineNumber := 10
		for scanner.Scan() {
			line := scanner.Text()
			//skip empty line
			if line == "" {
				continue
			}
			//ignore first n lines
			if skipRemaining > 0 {
				skipRemaining--
				continue
			}
			//ignore line with specified prefix
			if skipLine(line, args.SkipSymbol) {
				continue
			}
			detectLines = append(detectLines, line)
			if len(detectLines) >= lineNumber {
				break
			}
		}
		//if the suffix of file name is ".csv", set separator to ",".
		//if the suffix of file name is "tsv", set separator to "\t".
		if strings.HasSuffix(fn, ".csv") {
			b.sep = ','
		} else if strings.HasSuffix(fn, ".tsv") {
			b.sep = '\t'
		} else {
			sd := sepDetecor{}
			b.sep = sd.sepDetect(detectLines)
		}

	}
	//check final separator
	if b.sep == 0 {
		doneChan <- errors.New(errSeparatorNotDetected)
		return
	}

	//add detectLines to buffer
	for _, line := range detectLines {
		//parse and add line to buffer
		err = addDRToBuffer(b, line, args.ShowNum, args.HideNum)
		if err != nil {
			progress.finish()
			doneChan <- err
			return
		}
		totalAddedLN++
		bytesRead := int64(len(line) + 1) // +1 for newline
		loadProgress.LoadedBytes += bytesRead
		progress.increment(bytesRead)
		if totalAddedLN >= args.NLine && args.NLine > 0 {
			break
		}
	}

	// Signal that initial data is ready for rendering
	updateChan <- true

	// Parse and append lines sequentially so that row order is preserved.
	batchSize := 0
	const updateInterval = 500 // Update UI every 500 lines
	for scanner.Scan() {
		line := scanner.Text()
		//skip empty line
		if line == "" {
			continue
		}
		//ignore first n lines
		if skipRemaining > 0 {
			skipRemaining--
			continue
		}
		//ignore line with specified prefix
		if skipLine(line, args.SkipSymbol) {
			continue
		}
		if totalAddedLN >= args.NLine && args.NLine > 0 {
			break
		}

		err = addDRToBuffer(b, line, args.ShowNum, args.HideNum)
		if err != nil {
			progress.finish()
			doneChan <- err
			return
		}
		totalAddedLN++
		batchSize++
		bytesRead := int64(len(line) + 1) // +1 for newline
		loadProgress.LoadedBytes += bytesRead
		progress.increment(bytesRead)

		// Update UI periodically (non-blocking: skip if channel is full)
		if batchSize >= updateInterval {
			select {
			case updateChan <- true:
				batchSize = 0
			default:
			}
		}
	}

	loadProgress.IsComplete = true

	// Auto-detect column types after loading (async)
	go b.detectAllColumnTypes()

	// Enable string interning for categorical columns (async)
	go b.enableStringInterning()

	progress.finish()
	doneChan <- nil
}

// load file content to buffer (synchronous version for small files or when preferred)
func loadFileToBuffer(fn string, b *Buffer) error {
	totalAddedLN := 0             //the number of lines has been added into buffer
	skipRemaining := args.SkipNum //lines still to skip before reading data

	// Get file size for progress tracking
	fileInfo, err := os.Stat(fn)
	if err != nil {
		return err
	}
	var fileSize int64
	if !fileInfo.IsDir() {
		fileSize = fileInfo.Size()
	}

	// Create progress tracker
	progress := newProgressTracker(fileSize, true)

	scanner, err := getFileScanner(fn)
	if err != nil {
		return err
	}
	scanner.Split(bufio.ScanLines)
	//set separator, if user does not provide it.
	var detectLines []string //lines as detect separator data
	if b.sep == 0 {
		//read 10 lines to detect separator
		lineNumber := 10
		for scanner.Scan() {
			line := scanner.Text()
			//skip empty line
			if line == "" {
				continue
			}
			//ignore first n lines
			if skipRemaining > 0 {
				skipRemaining--
				continue
			}
			//ignore line with specified prefix
			if skipLine(line, args.SkipSymbol) {
				continue
			}
			detectLines = append(detectLines, line)
			if len(detectLines) >= lineNumber {
				break
			}
		}
		//if the suffix of file name is ".csv", set separator to ",".
		//if the suffix of file name is "tsv", set separator to "\t".
		if strings.HasSuffix(fn, ".csv") {
			b.sep = ','
		} else if strings.HasSuffix(fn, ".tsv") {
			b.sep = '\t'
		} else {
			sd := sepDetecor{}
			b.sep = sd.sepDetect(detectLines)
		}

	}
	//check final separator
	if b.sep == 0 {
		return errors.New(errSeparatorNotDetected)
	}

	//add detectLines to buffer
	for _, line := range detectLines {
		//parse and add line to buffer
		err = addDRToBuffer(b, line, args.ShowNum, args.HideNum)
		if err != nil {
			progress.finish()
			return err

		}
		totalAddedLN++
		progress.increment(int64(len(line) + 1)) // +1 for newline
		if totalAddedLN >= args.NLine && args.NLine > 0 {
			break
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		//skip empty line
		if line == "" {
			continue
		}
		//ignore first n lines
		if skipRemaining > 0 {
			skipRemaining--
			continue
		}
		//ignore line with specified prefix
		if skipLine(line, args.SkipSymbol) {
			continue
		}

		//parse and add line to buffer
		if totalAddedLN >= args.NLine && args.NLine > 0 {
			break
		}
		err = addDRToBuffer(b, line, args.ShowNum, args.HideNum)
		if err != nil {
			progress.finish()
			return err
		}
		totalAddedLN++
		progress.increment(int64(len(line) + 1)) // +1 for newline
	}

	// Auto-detect column types after loading
	b.detectAllColumnTypes()

	// Enable string interning for categorical columns
	b.enableStringInterning()

	progress.finish()
	return nil
}

// load console pipe content to buffer (async version for progressive rendering)
func loadPipeToBufferAsync(stdin io.Reader, b *Buffer, updateChan chan<- bool, doneChan chan<- error) {
	totalAddedLN := 0             //the number of lines has been added into buffer
	skipRemaining := args.SkipNum //lines still to skip before reading data
	var err error

	// For pipes, we don't know the total size
	loadProgress.TotalBytes = 0
	loadProgress.LoadedBytes = 0
	loadProgress.IsComplete = false

	// Create progress tracker (disabled for async loading)
	progress := newProgressTracker(0, false)

	scanner := bufio.NewScanner(stdin)
	//increase buffer size for large files and long lines
	const maxScanTokenSize = 1024 * 1024
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)
	//read 10 lines to detect separator
	lineNumber := 10
	var detectLines []string //lines as detect separator data
	if b.sep == 0 {
		for scanner.Scan() {
			line := scanner.Text()
			//skip empty line
			if line == "" {
				continue
			}
			//ignore first n lines
			if skipRemaining > 0 {
				skipRemaining--
				continue
			}
			//ignore line with specified prefix
			if skipLine(line, args.SkipSymbol) {
				continue
			}
			detectLines = append(detectLines, line)
			if len(detectLines) >= lineNumber {
				break
			}
		}
		sd := sepDetecor{}
		b.sep = sd.sepDetect(detectLines)
	}
	//check final separator
	if b.sep == 0 {
		doneChan <- errors.New(errSeparatorNotDetected)
		return
	}

	//add detectLines to buffer
	for _, line := range detectLines {
		//parse and add line to buffer
		err = addDRToBuffer(b, line, args.ShowNum, args.HideNum)
		if err != nil {
			progress.finish()
			doneChan <- err
			return
		}
		totalAddedLN++
		bytesRead := int64(len(line) + 1)
		loadProgress.LoadedBytes += bytesRead
		progress.increment(bytesRead)
		if totalAddedLN >= args.NLine && args.NLine > 0 {
			break
		}
	}

	// Signal that initial data is ready for rendering
	updateChan <- true

	// Parse and append lines sequentially so that row order is preserved.
	batchSize := 0
	const updateInterval = 500 // Update UI every 500 lines
	for scanner.Scan() {
		line := scanner.Text()
		//skip empty line
		if line == "" {
			continue
		}
		//ignore first n lines
		if skipRemaining > 0 {
			skipRemaining--
			continue
		}
		//ignore line with specified prefix
		if skipLine(line, args.SkipSymbol) {
			continue
		}
		if totalAddedLN >= args.NLine && args.NLine > 0 {
			break
		}

		err = addDRToBuffer(b, line, args.ShowNum, args.HideNum)
		if err != nil {
			progress.finish()
			doneChan <- err
			return
		}
		totalAddedLN++
		batchSize++
		bytesRead := int64(len(line) + 1) // +1 for newline
		loadProgress.LoadedBytes += bytesRead
		progress.increment(bytesRead)

		// Update UI periodically (non-blocking: skip if channel is full)
		if batchSize >= updateInterval {
			select {
			case updateChan <- true:
				batchSize = 0
			default:
			}
		}
	}

	loadProgress.IsComplete = true

	// Auto-detect column types after loading (async)
	go b.detectAllColumnTypes()

	// Enable string interning for categorical columns (async)
	go b.enableStringInterning()

	progress.finish()
	doneChan <- nil
}

// load console pipe content to buffer (synchronous version)
func loadPipeToBuffer(stdin io.Reader, b *Buffer) error {
	totalAddedLN := 0             //the number of lines has been added into buffer
	skipRemaining := args.SkipNum //lines still to skip before reading data
	var err error

	// Create progress tracker (no file size for pipes)
	progress := newProgressTracker(0, true)

	scanner := bufio.NewScanner(stdin)
	//increase buffer size for large files and long lines
	const maxScanTokenSize = 1024 * 1024
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)
	//read 10 lines to detect separator
	lineNumber := 10
	var detectLines []string //lines as detect separator data
	if b.sep == 0 {
		for scanner.Scan() {
			line := scanner.Text()
			//skip empty line
			if line == "" {
				continue
			}
			//ignore first n lines
			if skipRemaining > 0 {
				skipRemaining--
				continue
			}
			//ignore line with specified prefix
			if skipLine(line, args.SkipSymbol) {
				continue
			}
			detectLines = append(detectLines, line)
			if len(detectLines) >= lineNumber {
				break
			}
		}
		sd := sepDetecor{}
		b.sep = sd.sepDetect(detectLines)
	}
	//check final separator
	if b.sep == 0 {
		return errors.New(errSeparatorNotDetected)
	}

	//add detectLines to buffer
	for _, line := range detectLines {
		//parse and add line to buffer
		err = addDRToBuffer(b, line, args.ShowNum, args.HideNum)
		if err != nil {
			progress.finish()
			return err
		}
		totalAddedLN++
		progress.increment(int64(len(line) + 1))
		if totalAddedLN >= args.NLine && args.NLine > 0 {
			break
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		//skip empty line
		if line == "" {
			continue
		}
		//ignore first n lines
		if skipRemaining > 0 {
			skipRemaining--
			continue
		}
		//ignore line with specified prefix
		if skipLine(line, args.SkipSymbol) {
			continue
		}

		//parse and add line to buffer
		if totalAddedLN >= args.NLine && args.NLine > 0 {
			break
		}
		err = addDRToBuffer(b, line, args.ShowNum, args.HideNum)
		if err != nil {
			progress.finish()
			return err
		}
		totalAddedLN++
		progress.increment(int64(len(line) + 1))
	}

	// Auto-detect column types after loading
	b.detectAllColumnTypes()

	// Enable string interning for categorical columns
	b.enableStringInterning()

	progress.finish()
	return nil
}

// check a line whether should bu skip, according to prefix
func skipLine(line string, sy []string) bool {
	for _, sy := range sy {
		if strings.HasPrefix(line, sy) {
			return true
		}

	}
	return false
}

// get suitable scanner(compressed or not)
func getFileScanner(fn string) (*bufio.Scanner, error) {
	info, err := os.Stat(fn)
	if err != nil {
		return nil, err
	}
	//check if fn is a directory
	if info.IsDir() {
		return nil, errors.New(fn + " is a directory")
	}

	file, err := os.Open(fn)
	if err != nil {
		return nil, err
	}

	var scanner *bufio.Scanner
	//if input is a gzip file
	if strings.HasSuffix(fn, ".gz") {
		gzCont, err := gzip.NewReader(file)
		if err != nil {
			return nil, err
		}
		scanner = bufio.NewScanner(gzCont)
	} else {
		scanner = bufio.NewScanner(file)
	}

	//increase buffer size for large files and long lines
	//default is 64KB, we set to 1MB for better performance
	const maxScanTokenSize = 1024 * 1024
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	return scanner, nil
}

// check columns that should be displayed
func getVisCol(showNumL, hideNumL []int, colLen int) ([]int, error) {
	for _, i := range showNumL {
		if i > colLen || i <= 0 {
			return nil, errors.New("Column number " + I2S(i) + " does not exist")
		}
	}

	for _, i := range hideNumL {
		if i > colLen || i <= 0 {
			return nil, errors.New("Column number " + I2S(i) + " does not exist")
		}
	}

	var visCol []int
	for i := 0; i < colLen; i++ {
		flag, err := checkVisible(showNumL, hideNumL, i)
		if err != nil {
			return nil, err
		}
		if flag {
			visCol = append(visCol, i)
		}
	}
	return visCol, nil

}

// check ith column should be displayed or not
func checkVisible(showNumL, hideNumL []int, col int) (bool, error) {
	if len(showNumL) != 0 && len(hideNumL) != 0 {
		return false, errors.New("you can only set visible column or hidden column")
	}

	if len(showNumL) != 0 {
		for _, colTestS := range showNumL {
			if col+1 == colTestS {
				return true, nil
			}
		}
		return false, nil
	}
	if len(hideNumL) != 0 {
		for _, colTestH := range hideNumL {
			if col+1 == colTestH {
				return false, nil
			}
		}
	}
	return true, nil
}

// use go csv library to parse a string line into csv format
// Optimized version with reusable reader
func lineCSVParse(s string, sep rune) ([]string, error) {
	r := csv.NewReader(strings.NewReader(s))
	r.Comma = sep
	r.LazyQuotes = true
	r.ReuseRecord = true //reuse backing array for performance
	//r.TrimLeadingSpace = true //disable, because it will remove NULL item and cause issue.
	record, err := r.Read()
	if err != nil {
		return nil, err
	}
	//make a copy since ReuseRecord=true reuses the backing array
	result := make([]string, len(record))
	copy(result, record)
	return result, err
}

// Fast CSV parser for simple cases (no quotes, no escaping)
// Falls back to standard parser if needed
func lineCSVParseFast(s string, sep rune) ([]string, error) {
	// Quick check if line contains quotes (needs full parser)
	hasQuotes := false
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			hasQuotes = true
			break
		}
	}

	// Use fast path for simple CSV lines
	if !hasQuotes {
		// Count separators to pre-allocate slice
		sepCount := 0
		for i := 0; i < len(s); i++ {
			if rune(s[i]) == sep {
				sepCount++
			}
		}

		result := make([]string, 0, sepCount+1)
		start := 0
		for i := 0; i < len(s); i++ {
			if rune(s[i]) == sep {
				result = append(result, s[start:i])
				start = i + 1
			}
		}
		// Add last field
		result = append(result, s[start:])
		return result, nil
	}

	// Fall back to standard parser for complex cases
	return lineCSVParse(s, sep)
}

// add displayable(according to user's input argument) RowArray(covert line to array) To Buffer
func addDRToBuffer(b *Buffer, line string, showNum, hideNum []int) error {
	var err error
	lineCSVParts, err := lineCSVParseFast(line, b.sep)
	if err != nil {
		return err
	}
	if len(showNum) != 0 || len(hideNum) != 0 {
		// Pre-allocate slice with known capacity
		visCol, err := getVisCol(showNum, hideNum, len(lineCSVParts))
		if err != nil {
			return err
		}
		lineSli := make([]string, 0, len(visCol))
		for _, i := range visCol {
			lineSli = append(lineSli, lineCSVParts[i])
		}
		err = b.contAppendSli(lineSli, args.Strict)
		if err != nil {
			return err
		}

	} else {
		err := b.contAppendSli(lineCSVParts, args.Strict)
		if err != nil {
			return err
		}
	}
	return err
}
