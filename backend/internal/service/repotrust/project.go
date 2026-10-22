package repotrust

import (
	"context"
	"path/filepath"
	"sort"
)

// Project scans the root and registered workspace child repositories. Relative
// child paths are persisted project data, not caller-supplied filesystem paths.
func Project(ctx context.Context, root string, children []string, branch string) (Scan, error) {
	if branch != "" {
		base, err := Project(ctx, root, children, "")
		if err != nil {
			return base, err
		}
		other, err := projectTree(ctx, root, children, branch)
		if err != nil {
			return other, err
		}
		return Combine(base, other), nil
	}
	return projectTree(ctx, root, children, "")
}
func projectTree(ctx context.Context, root string, children []string, branch string) (Scan, error) {
	paths := append([]string{""}, children...)
	sort.Strings(paths)
	out := Scan{Files: []File{}}
	contents := map[string][]byte{}
	count := 0
	var single Scan
	for _, rel := range paths {
		checkout := root
		if rel != "" {
			checkout = filepath.Join(root, filepath.FromSlash(rel))
		}
		var current Scan
		var err error
		if branch == "" {
			current, err = Detect(ctx, checkout)
		} else {
			current, err = DetectBranch(ctx, checkout, branch)
		}
		if err != nil {
			return out, err
		}
		if current.Fingerprint == "" {
			continue
		}
		count++
		single = current
		for p, data := range current.contents {
			if rel != "" {
				p = rel + "/" + p
			}
			contents[p] = data
		}
	}
	if count == 1 && len(children) == 0 {
		return single, nil
	}
	out = inventory(contents)
	return out, nil
}
