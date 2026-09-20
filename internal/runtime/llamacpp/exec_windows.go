//go:build windows

package llamacpp

import (
	"os"
	"path/filepath"
	"strings"
)

// withExecExtensions appends the extensions Windows treats as executable, so a
// path configured without ".exe" still resolves. PATHEXT is honoured when set.
func withExecExtensions(p string) []string {
	if filepath.Ext(p) != "" {
		return nil
	}
	exts := strings.Split(os.Getenv("PATHEXT"), string(os.PathListSeparator))
	var out []string
	for _, e := range exts {
		if e = strings.TrimSpace(e); e == "" || !strings.HasPrefix(e, ".") {
			continue
		}
		// PATHEXT is conventionally upper case; prefer the lower-case spelling
		// so logged paths read naturally, and fall back to it as written.
		if lower := strings.ToLower(e); lower != e {
			out = append(out, p+lower)
		}
		out = append(out, p+e)
	}
	if len(out) == 0 {
		out = []string{p + ".exe", p + ".bat", p + ".cmd"}
	}
	return out
}
