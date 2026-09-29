package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type pathPair struct {
	old string
	new string
}

func rewriteConfigPath(current, oldPath, newPath string) (string, bool) {
	if current == "" || oldPath == "" || oldPath == newPath {
		return current, false
	}
	cur := filepath.ToSlash(current)
	old := filepath.ToSlash(oldPath)
	if cur == old {
		return styleConfigPath(current, newPath), true
	}
	if strings.HasPrefix(cur, old+"/") {
		next := filepath.ToSlash(newPath) + cur[len(old):]
		return styleConfigPath(current, next), true
	}
	return current, false
}

func styleConfigPath(original, next string) string {
	if strings.Contains(original, `\`) && !strings.Contains(original, "/") {
		return filepath.FromSlash(filepath.ToSlash(next))
	}
	return filepath.ToSlash(next)
}

func rewriteConfigPathPairs(current string, pairs []pathPair) (string, bool) {
	for _, pair := range pairs {
		if next, ok := rewriteConfigPath(current, pair.old, pair.new); ok {
			return next, true
		}
	}
	return current, false
}

func rewriteJSONTree(v any, pairs []pathPair) (any, bool) {
	switch n := v.(type) {
	case map[string]any:
		return rewriteJSONStringMap(n, pairs)
	case []any:
		changed := false
		for i, item := range n {
			next, ok := rewriteJSONTree(item, pairs)
			if !ok {
				continue
			}
			n[i] = next
			changed = true
		}
		return n, changed
	case string:
		return rewriteConfigPathPairs(n, pairs)
	default:
		return v, false
	}
}

func rewriteJSONStringMap(m map[string]any, pairs []pathPair) (map[string]any, bool) {
	changed := false
	next := make(map[string]any, len(m))
	for key, val := range m {
		newKey, keyChanged := rewriteConfigPathPairs(key, pairs)
		newVal, valChanged := rewriteJSONTree(val, pairs)
		if keyChanged || valChanged {
			changed = true
		}
		next[newKey] = newVal
	}
	if !changed {
		return m, false
	}
	return next, true
}

func rewriteJSONFile(path string, pairs []pathPair) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	next, changed := rewriteJSONTree(doc, pairs)
	if !changed {
		return nil
	}
	return writeJSONFile(path, next)
}

func findWorkspacesAlong(path string) []string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, store := range listedWorkspaceStores() {
		ws := store.Path
		if ws == abs || strings.HasPrefix(abs, ws+string(os.PathSeparator)) {
			out = append(out, ws)
		}
	}
	return out
}

func configPathPairs(oldPath, newPath, workspace string) []pathPair {
	pairs := []pathPair{{old: oldPath, new: newPath}}
	if workspace == "" {
		return pairs
	}
	oldKey, err := lockKey(workspace, oldPath)
	if err != nil {
		return pairs
	}
	newKey, err := lockKey(workspace, newPath)
	if err != nil {
		return pairs
	}
	if oldKey == filepath.ToSlash(oldPath) && newKey == filepath.ToSlash(newPath) {
		return pairs
	}
	return append(pairs, pathPair{old: oldKey, new: newKey})
}

func visitVedaJSON(configDir, workspace string, oldPath, newPath string, seen map[string]struct{}) error {
	entries, err := os.ReadDir(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	pairs := configPathPairs(oldPath, newPath, workspace)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		full := filepath.Join(configDir, entry.Name())
		if _, ok := seen[full]; ok {
			continue
		}
		seen[full] = struct{}{}
		if err := rewriteJSONFile(full, pairs); err != nil {
			return err
		}
	}
	return nil
}

func rewriteVedaConfigs(oldPath, newPath string, extraWorkspaces []string) error {
	oldAbs, err := filepath.Abs(oldPath)
	if err != nil {
		return nil
	}
	newAbs, err := filepath.Abs(newPath)
	if err != nil {
		return nil
	}
	if oldAbs == newAbs {
		return nil
	}

	seen := map[string]struct{}{}
	for _, store := range listedWorkspaceStores() {
		if err := visitVedaJSON(store.Dir, store.Path, oldAbs, newAbs, seen); err != nil {
			return err
		}
	}
	for _, ws := range extraWorkspaces {
		dir, err := workspaceDataDir(ws)
		if err != nil {
			continue
		}
		if err := visitVedaJSON(dir, ws, oldAbs, newAbs, seen); err != nil {
			return err
		}
	}
	for _, path := range []string{oldAbs, newAbs} {
		for _, ws := range findWorkspacesAlong(path) {
			dir, err := workspaceDataDir(ws)
			if err != nil {
				continue
			}
			if err := visitVedaJSON(dir, ws, oldAbs, newAbs, seen); err != nil {
				return err
			}
		}
	}
	if dir, err := dataDir(); err == nil {
		if err := visitVedaJSON(dir, "", oldAbs, newAbs, seen); err != nil {
			return err
		}
	}
	return nil
}
