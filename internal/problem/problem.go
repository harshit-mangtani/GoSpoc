package problem

type Problem struct {
	ID            int64
	Slug          string
	Title         string
	Statement     string
	TimeLimitMS   int
	MemoryLimitKB int
}
