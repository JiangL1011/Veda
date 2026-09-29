package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	defaultMaxMatches   = 2000
	defaultMaxPerFile   = 200
	defaultContextLines = 1
	maxSearchQueryRunes = 10000
)

// SearchOptions 决定查询词的匹配方式。
type SearchOptions struct {
	CaseSensitive bool `json:"caseSensitive"`
	WholeWord     bool `json:"wholeWord"`
	UseRegex      bool `json:"useRegex"`
	MaxMatches    int  `json:"maxMatches"`
	MaxPerFile    int  `json:"maxPerFile"`
	ContextLines  int  `json:"contextLines"`
}

// SearchMatch 表示文件中的一处命中。
type SearchMatch struct {
	Line      int      `json:"line"`
	Column    int      `json:"column"`
	EndLine   int      `json:"endLine"`
	EndColumn int      `json:"endColumn"`
	StartByte int      `json:"startByte"`
	EndByte   int      `json:"endByte"`
	Match     string   `json:"match"`
	LineText  string   `json:"lineText"`
	Before    []string `json:"before,omitempty"`
	After     []string `json:"after,omitempty"`
}

// FileSearchResult 把同一个路径下的命中结果聚合在一起。
type FileSearchResult struct {
	Path    string        `json:"path"`
	RelPath string        `json:"relPath,omitempty"`
	Kind    string        `json:"kind"`
	Matches []SearchMatch `json:"matches"`
}

// SearchResult 是返回给编辑器的数据结构。
type SearchResult struct {
	Query        string             `json:"query"`
	ScannedFiles int                `json:"scannedFiles"`
	SkippedFiles int                `json:"skippedFiles,omitempty"`
	Truncated    bool               `json:"truncated"`
	Files        []FileSearchResult `json:"files"`
}

func normalizeSearchOptions(opts SearchOptions) SearchOptions {
	if opts.MaxMatches <= 0 {
		opts.MaxMatches = defaultMaxMatches
	}
	if opts.MaxPerFile <= 0 {
		opts.MaxPerFile = defaultMaxPerFile
	}
	if opts.ContextLines < 0 {
		opts.ContextLines = 0
	}
	if opts.ContextLines > 5 {
		opts.ContextLines = 5
	}
	return opts
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func atWordBoundary(content string, start, end int) bool {
	if start < 0 || end > len(content) || start > end {
		return false
	}
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(content[:start])
		if isWordRune(r) {
			return false
		}
	}
	if end < len(content) {
		r, _ := utf8.DecodeRuneInString(content[end:])
		if isWordRune(r) {
			return false
		}
	}
	return true
}

func runesEqualFold(a, b rune) bool {
	return unicode.ToLower(a) == unicode.ToLower(b)
}

func indexLiteral(content, query string, from int, caseSensitive bool) (int, int) {
	if query == "" || from > len(content) {
		return -1, 0
	}
	if caseSensitive {
		idx := strings.Index(content[from:], query)
		if idx < 0 {
			return -1, 0
		}
		return from + idx, len(query)
	}
	qRunes := []rune(query)
	if len(qRunes) == 0 {
		return -1, 0
	}
	for i := from; i < len(content); {
		si := i
		ok := true
		for _, qr := range qRunes {
			if si >= len(content) {
				ok = false
				break
			}
			sr, size := utf8.DecodeRuneInString(content[si:])
			if !runesEqualFold(sr, qr) {
				ok = false
				break
			}
			si += size
		}
		if ok {
			return i, si - i
		}
		_, size := utf8.DecodeRuneInString(content[i:])
		if size <= 0 {
			size = 1
		}
		i += size
	}
	return -1, 0
}

type searchEngine struct {
	re            *regexp.Regexp
	query         string
	caseSensitive bool
	wholeWord     bool
	useRegex      bool
}

func compileSearch(query string, opts SearchOptions) (*searchEngine, error) {
	if utf8.RuneCountInString(query) > maxSearchQueryRunes {
		return nil, errors.New("搜索内容过长")
	}
	eng := &searchEngine{
		query:         query,
		caseSensitive: opts.CaseSensitive,
		wholeWord:     opts.WholeWord,
		useRegex:      opts.UseRegex,
	}
	if !opts.UseRegex {
		return eng, nil
	}
	pattern := query
	if !opts.CaseSensitive && !strings.HasPrefix(pattern, "(?i)") && !strings.HasPrefix(pattern, "(?m)") {
		pattern = "(?i)" + pattern
	} else if !opts.CaseSensitive && strings.HasPrefix(pattern, "(?m)") && !strings.Contains(pattern, "(?i)") {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, errors.New("无效的正则表达式")
	}
	eng.re = re
	return eng, nil
}

func (e *searchEngine) next(content string, from int) (int, int) {
	if from > len(content) {
		return -1, 0
	}
	if e.useRegex {
		loc := e.re.FindStringIndex(content[from:])
		if loc == nil {
			return -1, 0
		}
		start := from + loc[0]
		end := from + loc[1]
		if start == end {
			// 零宽匹配（例如 ^ 和 $）必须向前推进，否则会死循环。
			if end < len(content) {
				_, size := utf8.DecodeRuneInString(content[end:])
				if size <= 0 {
					size = 1
				}
				return e.next(content, end+size)
			}
			return -1, 0
		}
		if e.wholeWord && !atWordBoundary(content, start, end) {
			return e.next(content, start+1)
		}
		return start, end - start
	}
	start, n := indexLiteral(content, e.query, from, e.caseSensitive)
	if start < 0 {
		return -1, 0
	}
	end := start + n
	if e.wholeWord && !atWordBoundary(content, start, end) {
		_, size := utf8.DecodeRuneInString(content[start:])
		if size <= 0 {
			size = 1
		}
		return e.next(content, start+size)
	}
	return start, n
}

type lineIndex struct {
	starts []int
	texts  []string
}

func indexLines(content string) lineIndex {
	starts := []int{0}
	var texts []string
	begin := 0
	for i := 0; i < len(content); i++ {
		if content[i] != '\n' {
			continue
		}
		line := strings.TrimSuffix(content[begin:i], "\r")
		texts = append(texts, line)
		starts = append(starts, i+1)
		begin = i + 1
	}
	texts = append(texts, strings.TrimSuffix(content[begin:], "\r"))
	return lineIndex{starts: starts, texts: texts}
}

func columnAt(text string, byteDelta int) int {
	if byteDelta <= 0 {
		return 1
	}
	if byteDelta >= len(text) {
		return utf8.RuneCountInString(text) + 1
	}
	n := 0
	for i := 0; i < byteDelta && i < len(text); {
		_, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 {
			size = 1
		}
		if i+size > byteDelta {
			break
		}
		i += size
		n++
	}
	return n + 1
}

func (idx lineIndex) locate(offset int) (line, col int) {
	if len(idx.starts) == 0 {
		return 1, 1
	}
	line = 1
	for i := 1; i < len(idx.starts); i++ {
		if idx.starts[i] <= offset {
			line = i + 1
			continue
		}
		break
	}
	text := idx.texts[line-1]
	return line, columnAt(text, offset-idx.starts[line-1])
}

func (idx lineIndex) context(line, n int) (before, after []string) {
	if n <= 0 {
		return nil, nil
	}
	lineIdx := line - 1
	from := max(0, lineIdx-n)
	to := min(len(idx.texts), lineIdx)
	if from < to {
		before = append([]string{}, idx.texts[from:to]...)
	}
	from = min(len(idx.texts), lineIdx+1)
	to = min(len(idx.texts), lineIdx+1+n)
	if from < to {
		after = append([]string{}, idx.texts[from:to]...)
	}
	return before, after
}

func searchContent(content, query string, opts SearchOptions) (*SearchResult, error) {
	opts = normalizeSearchOptions(opts)
	out := &SearchResult{Query: query, Files: []FileSearchResult{}}
	if strings.TrimSpace(query) == "" {
		return out, nil
	}
	eng, err := compileSearch(query, opts)
	if err != nil {
		return nil, err
	}
	idx := indexLines(content)
	file := FileSearchResult{Matches: []SearchMatch{}}
	from := 0
	for len(file.Matches) < opts.MaxPerFile && len(file.Matches) < opts.MaxMatches {
		start, n := eng.next(content, from)
		if start < 0 {
			break
		}
		end := start + n
		line, col := idx.locate(start)
		endLine, endCol := idx.locate(end)
		before, after := idx.context(line, opts.ContextLines)
		lineText := ""
		if line-1 >= 0 && line-1 < len(idx.texts) {
			lineText = idx.texts[line-1]
		}
		file.Matches = append(file.Matches, SearchMatch{
			Line:      line,
			Column:    col,
			EndLine:   endLine,
			EndColumn: endCol,
			StartByte: start,
			EndByte:   end,
			Match:     content[start:end],
			LineText:  lineText,
			Before:    before,
			After:     after,
		})
		from = end
	}
	if from < len(content) {
		start, _ := eng.next(content, from)
		if start >= 0 {
			out.Truncated = true
		}
	}
	if len(file.Matches) > 0 {
		out.Files = []FileSearchResult{file}
	}
	return out, nil
}

func insideRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func resolvedInsideRoot(root, path string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		info, lerr := os.Lstat(path)
		if lerr != nil {
			return false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		return insideRoot(root, path)
	}
	return insideRoot(root, resolved)
}

func searchWorkspace(ctx context.Context, root, query string, opts SearchOptions) (*SearchResult, error) {
	opts = normalizeSearchOptions(opts)
	out := &SearchResult{Query: query, Files: []FileSearchResult{}}
	if strings.TrimSpace(query) == "" {
		return out, nil
	}
	if _, err := compileSearch(query, opts); err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("工作区路径必须是文件夹")
	}

	total := 0
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if walkErr != nil {
			out.SkippedFiles++
			return nil
		}
		if path != absRoot && shouldSkipName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		linfo, lerr := d.Info()
		if lerr == nil && linfo.Mode()&os.ModeSymlink != 0 {
			if !resolvedInsideRoot(absRoot, path) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				target, err := os.Stat(path)
				if err == nil && target.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() {
			return nil
		}
		target, err := os.Stat(path)
		if err != nil {
			out.SkippedFiles++
			return nil
		}
		if target.IsDir() {
			if !resolvedInsideRoot(absRoot, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !insideRoot(absRoot, path) {
			return nil
		}
		kind := sniffUnknownKind(path, classifyFile(path, false))
		if kind != KindMarkdown && kind != KindText {
			return nil
		}
		if target.Size() > maxTextBytes {
			out.SkippedFiles++
			return nil
		}
		text, err := readTextFile(path)
		if err != nil {
			out.SkippedFiles++
			return nil
		}
		out.ScannedFiles++
		fileOpts := opts
		remaining := opts.MaxMatches - total
		if remaining <= 0 {
			out.Truncated = true
			return errors.New("truncated")
		}
		if fileOpts.MaxPerFile > remaining {
			fileOpts.MaxPerFile = remaining
		}
		hit, err := searchContent(text.Content, query, fileOpts)
		if err != nil {
			return err
		}
		if len(hit.Files) == 0 || len(hit.Files[0].Matches) == 0 {
			return nil
		}
		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			rel = path
		}
		file := hit.Files[0]
		file.Path = path
		file.RelPath = filepath.ToSlash(rel)
		file.Kind = kind
		out.Files = append(out.Files, file)
		total += len(file.Matches)
		if hit.Truncated || total >= opts.MaxMatches {
			out.Truncated = true
			return errors.New("truncated")
		}
		return nil
	})
	if err != nil && err.Error() != "truncated" && !errors.Is(err, context.Canceled) {
		return nil, err
	}
	if errors.Is(err, context.Canceled) {
		return nil, err
	}
	return out, nil
}
