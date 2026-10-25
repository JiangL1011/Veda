package model

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
