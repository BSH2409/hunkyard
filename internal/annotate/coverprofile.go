package annotate

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// parseCoverprofile reads `go test -coverprofile` output. Blocks with a zero
// count become uncovered marks.
func parseCoverprofile(r io.Reader, opts Options) (Annotations, error) {
	out := Annotations{Tool: "coverage", Marks: map[string][]Mark{}}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	var totals coverageTotals
	prefix := ""
	if opts.ModulePath != "" {
		prefix = strings.TrimSuffix(opts.ModulePath, "/") + "/"
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// file.go:start.col,end.col statements count
		colon := strings.LastIndex(line, ":")
		fields := strings.Fields(line[colon+1:])
		if colon < 0 || len(fields) != 3 {
			return Annotations{}, fmt.Errorf("malformed coverprofile line %q", line)
		}
		file := line[:colon]
		start, end, err := coverRange(fields[0])
		if err != nil {
			return Annotations{}, fmt.Errorf("malformed coverprofile line %q", line)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return Annotations{}, fmt.Errorf("malformed coverprofile line %q", line)
		}
		if prefix != "" && strings.HasPrefix(file, prefix) {
			file = strings.TrimPrefix(file, prefix)
		}
		totals.add(relativePath(file, opts), start, end, count)
	}
	totals.markUncovered(out.Marks)
	return out, scanner.Err()
}

// coverageTotals sums hit counts per file range across repeated entries
// (one per test binary with -coverpkg, or merged lcov records), keeping the
// order ranges first appeared in.
type coverageTotals struct {
	order []coverageRange
	hits  map[coverageRange]int
}

type coverageRange struct {
	file       string
	start, end int
}

func (c *coverageTotals) add(file string, start, end, hits int) {
	key := coverageRange{file, start, end}
	if c.hits == nil {
		c.hits = map[coverageRange]int{}
	}
	if _, seen := c.hits[key]; !seen {
		c.order = append(c.order, key)
	}
	c.hits[key] += hits
}

// markUncovered adds a mark for every range no entry covered.
func (c *coverageTotals) markUncovered(marks map[string][]Mark) {
	for _, key := range c.order {
		if c.hits[key] == 0 {
			marks[key.file] = append(marks[key.file], Mark{Start: key.start, End: key.end, Kind: markUncovered})
		}
	}
}

func coverRange(spec string) (int, int, error) {
	from, to, ok := strings.Cut(spec, ",")
	if !ok {
		return 0, 0, fmt.Errorf("bad range")
	}
	start, err := strconv.Atoi(strings.SplitN(from, ".", 2)[0])
	if err != nil {
		return 0, 0, err
	}
	end, err := strconv.Atoi(strings.SplitN(to, ".", 2)[0])
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}
