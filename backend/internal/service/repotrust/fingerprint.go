package repotrust

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// inventory hashes sorted relative paths and their exact retained contents,
// length framed to avoid ambiguous path/content concatenations. Only files AO
// merges use canonical JSON after exact callback removal (see managed.go).
func inventory(contents map[string][]byte) Scan {
	out := Scan{Files: []File{}, contents: contents}
	paths := make([]string, 0, len(contents))
	for p := range contents {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		data := contents[p]
		_, _ = fmt.Fprintf(h, "%d:%s%d:", len(p), p, len(data))
		_, _ = h.Write(data)
		out.Files = append(out.Files, File{Path: p, Summary: summarize(data)})
	}
	if len(paths) > 0 {
		out.Fingerprint = hex.EncodeToString(h.Sum(nil))
	}
	return out
}
