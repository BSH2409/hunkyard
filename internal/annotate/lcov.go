package annotate

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// parseLcov reads an lcov tracefile. `DA:line,0` lines become uncovered marks.
func parseLcov(r io.Reader, opts Options) (Annotations, error) {
	out := Annotations{Tool: "coverage", Marks: map[string][]Mark{}}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	file := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "SF:"):
			file = relativePath(strings.TrimPrefix(line, "SF:"), opts)
		case line == "end_of_record":
			file = ""
		case strings.HasPrefix(line, "DA:") && file != "":
			parts := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
			if len(parts) < 2 {
				continue
			}
			n, err1 := strconv.Atoi(parts[0])
			hits, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil || hits > 0 {
				continue
			}
			out.Marks[file] = append(out.Marks[file], Mark{Start: n, End: n, Kind: markUncovered})
		}
	}
	return out, scanner.Err()
}
