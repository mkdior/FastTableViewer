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

// loadSource describes where rows are read from.
type loadSource struct {
	name      string         // file name, or "" when reading from a pipe
	scanner   *bufio.Scanner // line scanner over the (possibly decompressed) input
	closer    io.Closer      // underlying file to release once loading ends, nil for pipes
	totalSize int64          // input size in bytes, 0 when unknown
}

// updateInterval is the number of appended rows between two UI update signals.
const updateInterval = 500

// separatorSampleLines is how many lines are read before the separator is inferred.
const separatorSampleLines = 10

// detectSeparator infers the delimiter for the given source from its first lines.
// File name suffixes .csv and .tsv take priority over content-based detection.
func detectSeparator(name string, lines []string) rune {
	if strings.HasSuffix(name, ".csv") {
		return ','
	}
	if strings.HasSuffix(name, ".tsv") {
		return '\t'
	}
	sd := sepDetecor{}
	return sd.sepDetect(lines)
}

// loadToBuffer reads every row of src into b, honouring the skip/limit/column
// flags in args. When updateChan is non-nil the caller is signalled once the
// first rows are available and then periodically; the first signal blocks so
// the UI can start, later ones are dropped if the channel is full. When
// showProgress is set a progress line is printed to stdout.
func loadToBuffer(src loadSource, b *Buffer, updateChan chan<- bool, showProgress bool) error {
	if src.closer != nil {
		defer src.closer.Close()
	}
	loadProgress.TotalBytes = src.totalSize
	loadProgress.LoadedBytes = 0
	loadProgress.IsComplete = false

	progress := newProgressTracker(src.totalSize, showProgress)
	defer progress.finish()

	skipRemaining := args.SkipNum //lines still to skip before reading data
	totalAddedLN := 0             //the number of lines has been added into buffer

	// nextLine returns the next line that survives the blank/skip/prefix filters.
	nextLine := func() (string, bool) {
		for src.scanner.Scan() {
			line := src.scanner.Text()
			if line == "" {
				continue
			}
			if skipRemaining > 0 {
				skipRemaining--
				continue
			}
			if skipLine(line, args.SkipSymbol) {
				continue
			}
			return line, true
		}
		return "", false
	}

	// Read a small sample up front so the separator can be inferred and so the
	// UI has rows to show when it is first signalled.
	var head []string
	for len(head) < separatorSampleLines {
		line, ok := nextLine()
		if !ok {
			break
		}
		head = append(head, line)
	}
	if b.sep == 0 {
		b.sep = detectSeparator(src.name, head)
	}
	if b.sep == 0 {
		return errors.New(errSeparatorNotDetected)
	}

	batch := 0
	selectColumns := len(args.ShowNum) != 0 || len(args.HideNum) != 0
	var visCol []int // resolved once against the first row when --columns/--hide-columns are set
	// appendLine parses and stores one line; stop reports that --lines was reached.
	appendLine := func(line string) (stop bool, err error) {
		if args.NLine > 0 && totalAddedLN >= args.NLine {
			return true, nil
		}
		fields, err := lineCSVParseFast(line, b.sep)
		if err != nil {
			return true, err
		}
		if selectColumns {
			if visCol == nil {
				if visCol, err = getVisCol(args.ShowNum, args.HideNum, len(fields)); err != nil {
					return true, err
				}
			}
			fields = projectColumns(fields, visCol)
		}
		if err := b.contAppendSli(fields, args.Strict); err != nil {
			return true, err
		}
		totalAddedLN++
		batch++
		bytesRead := int64(len(line) + 1) // +1 for newline
		loadProgress.LoadedBytes += bytesRead
		progress.increment(bytesRead)
		return false, nil
	}

	for _, line := range head {
		stop, err := appendLine(line)
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}

	// Signal that initial data is ready for rendering.
	if updateChan != nil {
		updateChan <- true
	}

	for {
		line, ok := nextLine()
		if !ok {
			break
		}
		stop, err := appendLine(line)
		if err != nil {
			return err
		}
		if stop {
			break
		}
		if updateChan != nil && batch >= updateInterval {
			select {
			case updateChan <- true:
				batch = 0
			default:
				// Non-blocking - skip update if channel is full
			}
		}
	}

	if err := src.scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("line %d exceeds the %d byte line limit", totalAddedLN+args.SkipNum+1, maxScanTokenSize)
		}
		return err
	}

	loadProgress.IsComplete = true

	if updateChan != nil {
		// Async mode: do not hold up the "loaded" signal for post-processing.
		go b.detectAllColumnTypes()
		go b.enableStringInterning()
	} else {
		b.detectAllColumnTypes()
		b.enableStringInterning()
	}
	return nil
}

// openFileSource prepares a loadSource for the named file (gzip-aware).
func openFileSource(fn string) (loadSource, error) {
	fileInfo, err := os.Stat(fn)
	if err != nil {
		return loadSource{}, err
	}
	var fileSize int64
	// The uncompressed size of a gzip file is unknown, so leave it at 0 and
	// let the UI fall back to a row counter instead of a bogus percentage.
	if !fileInfo.IsDir() && !strings.HasSuffix(fn, ".gz") {
		fileSize = fileInfo.Size()
	}
	scanner, closer, err := getFileScanner(fn)
	if err != nil {
		return loadSource{}, err
	}
	scanner.Split(bufio.ScanLines)
	return loadSource{name: fn, scanner: scanner, closer: closer, totalSize: fileSize}, nil
}

// maxScanTokenSize is the longest single line the loaders accept (bufio default is 64KB).
const maxScanTokenSize = 1024 * 1024

// readerSource prepares a loadSource for an arbitrary reader such as stdin.
func readerSource(r io.Reader) loadSource {
	scanner := bufio.NewScanner(r)
	//increase buffer size for large files and long lines
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)
	return loadSource{scanner: scanner}
}

// load file content to buffer (async version for progressive rendering)
func loadFileToBufferAsync(fn string, b *Buffer, updateChan chan<- bool, doneChan chan<- error) {
	src, err := openFileSource(fn)
	if err != nil {
		doneChan <- err
		return
	}
	doneChan <- loadToBuffer(src, b, updateChan, false)
}

// load file content to buffer (synchronous version for small files or when preferred)
func loadFileToBuffer(fn string, b *Buffer) error {
	src, err := openFileSource(fn)
	if err != nil {
		return err
	}
	return loadToBuffer(src, b, nil, true)
}

// load console pipe content to buffer (async version for progressive rendering)
func loadPipeToBufferAsync(stdin io.Reader, b *Buffer, updateChan chan<- bool, doneChan chan<- error) {
	doneChan <- loadToBuffer(readerSource(stdin), b, updateChan, false)
}

// load console pipe content to buffer (synchronous version)
func loadPipeToBuffer(stdin io.Reader, b *Buffer) error {
	return loadToBuffer(readerSource(stdin), b, nil, true)
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

// get suitable scanner(compressed or not); the returned closer releases the file
func getFileScanner(fn string) (*bufio.Scanner, io.Closer, error) {
	info, err := os.Stat(fn)
	if err != nil {
		return nil, nil, err
	}
	//check if fn is a directory
	if info.IsDir() {
		return nil, nil, errors.New(fn + " is a directory")
	}

	file, err := os.Open(fn)
	if err != nil {
		return nil, nil, err
	}

	var scanner *bufio.Scanner
	//if input is a gzip file
	if strings.HasSuffix(fn, ".gz") {
		gzCont, err := gzip.NewReader(file)
		if err != nil {
			_ = file.Close()
			return nil, nil, err
		}
		scanner = bufio.NewScanner(gzCont)
	} else {
		scanner = bufio.NewScanner(file)
	}

	//increase buffer size for large files and long lines
	//default is 64KB, we set to 1MB for better performance
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	return scanner, file, nil
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
	// Lines without quotes cannot contain escaped separators, so a plain split
	// is exact. strings.Split handles multi-byte separators, which a byte-wise
	// comparison against the rune silently did not.
	if strings.IndexByte(s, '"') < 0 {
		return strings.Split(s, string(sep)), nil
	}

	// Fall back to standard parser for complex cases
	return lineCSVParse(s, sep)
}

// projectColumns keeps only the given column indexes, in order. Rows shorter
// than the header are padded with NaN so a ragged line cannot abort the load.
func projectColumns(fields []string, visCol []int) []string {
	out := make([]string, 0, len(visCol))
	for _, i := range visCol {
		if i < len(fields) {
			out = append(out, fields[i])
		} else {
			out = append(out, "NaN")
		}
	}
	return out
}

// add displayable(according to user's input argument) RowArray(covert line to array) To Buffer
func addDRToBuffer(b *Buffer, line string, showNum, hideNum []int) error {
	fields, err := lineCSVParseFast(line, b.sep)
	if err != nil {
		return err
	}
	if len(showNum) != 0 || len(hideNum) != 0 {
		visCol, err := getVisCol(showNum, hideNum, len(fields))
		if err != nil {
			return err
		}
		fields = projectColumns(fields, visCol)
	}
	return b.contAppendSli(fields, args.Strict)
}
