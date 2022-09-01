package buildinfo

var (
	// Injected with ldflags at build-time.
	tag string
)

func Version() string {
	if tag == "" {
		return "unknown"
	}
	return tag
}
