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
		if count > 0 {
			continue
		}
		if prefix != "" && strings.HasPrefix(file, prefix) {
			file = strings.TrimPrefix(file, prefix)
		}
		file = relativePath(file, opts)
		out.Marks[file] = append(out.Marks[file], Mark{Start: start, End: end, Kind: markUncovered})
	}
	return out, scanner.Err()
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
