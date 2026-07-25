package restapi

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// VaultEntry represents a file or directory entry.
type VaultEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size,omitempty"`
	ModTime int64  `json:"mtime,omitempty"`
}

// VaultGetResult is the response for GET /vault/*.
type VaultGetResult struct {
	Path    string       `json:"path"`
	IsDir   bool         `json:"isDir"`
	Entries []VaultEntry `json:"entries,omitempty"`
	Content string       `json:"content,omitempty"`
	Size    int64        `json:"size,omitempty"`
	MTime   int64        `json:"mtime,omitempty"`
}

// SearchResult is a single search hit.
type SearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// VaultOps provides file system operations on the vault directory.
type VaultOps struct {
	root string
}

// NewVaultOps creates a new VaultOps.
func NewVaultOps(root string) *VaultOps {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return &VaultOps{root: abs}
}

// safePath resolves a vault-relative path and ensures it stays within the vault root.
func (v *VaultOps) safePath(vpath string) (string, error) {
	clean := filepath.Clean(vpath)
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("path traversal not allowed: %s", vpath)
	}
	full := filepath.Join(v.root, clean)
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, v.root) {
		return "", fmt.Errorf("path traversal not allowed: %s", vpath)
	}
	return abs, nil
}

// Get reads a file or lists a directory.
func (v *VaultOps) Get(vpath string) (*VaultGetResult, error) {
	full, err := v.safePath(vpath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not found: %s", vpath)
		}
		return nil, err
	}

	result := &VaultGetResult{
		Path:  vpath,
		IsDir: info.IsDir(),
		Size:  info.Size(),
		MTime: info.ModTime().UnixMilli(),
	}

	if info.IsDir() {
		entries, err := os.ReadDir(full)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			eInfo, _ := e.Info()
			size := int64(0)
			mtime := int64(0)
			if eInfo != nil {
				size = eInfo.Size()
				mtime = eInfo.ModTime().UnixMilli()
			}
			result.Entries = append(result.Entries, VaultEntry{
				Name:    e.Name(),
				Path:    filepath.Join(vpath, e.Name()),
				IsDir:   e.IsDir(),
				Size:    size,
				ModTime: mtime,
			})
		}
		if result.Entries == nil {
			result.Entries = []VaultEntry{}
		}
		return result, nil
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	result.Content = string(data)
	return result, nil
}

// Put writes a file (creating directories as needed).
func (v *VaultOps) Put(vpath string, content []byte) error {
	full, err := v.safePath(vpath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	return os.WriteFile(full, content, 0644)
}

// Append appends content to a file.
func (v *VaultOps) Append(vpath string, content []byte) error {
	full, err := v.safePath(vpath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(full, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(content, '\n'))
	return err
}

// Delete removes a file.
func (v *VaultOps) Delete(vpath string) error {
	full, err := v.safePath(vpath)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil {
		if os.IsNotExist(err) {
			return nil // idempotent
		}
		return err
	}
	return nil
}

// Patch applies a simple section operation (simplified version).
func (v *VaultOps) Patch(vpath, operation, target, content string) error {
	full, err := v.safePath(vpath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return err
	}

	text := string(data)

	switch operation {
	case "replace":
		if target == "" {
			// Replace entire file
			text = content
		} else {
			// Try to replace a heading section
			text = replaceHeading(text, target, content)
		}
	case "prepend":
		if target == "" {
			text = content + text
		} else {
			text = prependHeading(text, target, content)
		}
	case "append":
		if target == "" {
			text = text + "\n" + content
		} else {
			text = appendHeading(text, target, content)
		}
	case "delete":
		if target == "" {
			text = ""
		} else {
			text = deleteHeading(text, target)
		}
	default:
		return fmt.Errorf("unknown operation: %s", operation)
	}

	return os.WriteFile(full, []byte(text), 0644)
}

// Move moves/renames a file.
func (v *VaultOps) Move(source, dest string, allowOverwrite bool) error {
	srcFull, err := v.safePath(source)
	if err != nil {
		return err
	}
	dstFull, err := v.safePath(dest)
	if err != nil {
		return err
	}

	if !allowOverwrite {
		if _, err := os.Stat(dstFull); err == nil {
			return fmt.Errorf("destination already exists: %s", dest)
		}
	}

	if err := os.MkdirAll(filepath.Dir(dstFull), 0755); err != nil {
		return err
	}
	return os.Rename(srcFull, dstFull)
}

// Copy copies a file.
func (v *VaultOps) Copy(source, dest string, allowOverwrite bool) error {
	srcFull, err := v.safePath(source)
	if err != nil {
		return err
	}
	dstFull, err := v.safePath(dest)
	if err != nil {
		return err
	}

	if !allowOverwrite {
		if _, err := os.Stat(dstFull); err == nil {
			return fmt.Errorf("destination already exists: %s", dest)
		}
	}

	data, err := os.ReadFile(srcFull)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstFull), 0755); err != nil {
		return err
	}
	return os.WriteFile(dstFull, data, 0644)
}

// ListTags scans all markdown files for tags (#tag syntax).
func (v *VaultOps) ListTags() ([]string, error) {
	tagSet := make(map[string]int)

	err := filepath.Walk(v.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		// Simple tag extraction: #tag patterns
		text := string(data)
		for _, word := range strings.Fields(text) {
			if strings.HasPrefix(word, "#") && len(word) > 1 {
				tag := strings.TrimRight(word, ",.;:!?()[]{}")
				tagSet[tag]++
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}

// Search performs a simple substring search across all files.
func (v *VaultOps) Search(query string) ([]SearchResult, error) {
	var results []SearchResult
	queryLower := strings.ToLower(query)

	err := filepath.Walk(v.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(v.root, path)
		rel = filepath.ToSlash(rel)

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), queryLower) {
				results = append(results, SearchResult{
					Path:    rel,
					Line:    i + 1,
					Content: strings.TrimSpace(line),
				})
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- Section patch helpers (simplified markdown heading manipulation) ---

func findHeadingLines(text string, heading string) (start, end int) {
	lines := strings.Split(text, "\n")
	target := strings.ToLower(strings.TrimSpace(heading))

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "#") {
			// Extract heading text (strip # and leading spaces)
			hText := strings.TrimSpace(line)
			for hText != "" && hText[0] == '#' {
				hText = strings.TrimSpace(hText[1:])
			}
			if strings.ToLower(hText) == target {
				start = i
				// Find end: next heading of same or higher level, or EOF
				level := 0
				for _, c := range line {
					if c == '#' {
						level++
					} else {
						break
					}
				}
				for j := i + 1; j < len(lines); j++ {
					nextLine := strings.TrimSpace(lines[j])
					if strings.HasPrefix(nextLine, "#") {
						nextLevel := 0
						for _, c := range nextLine {
							if c == '#' {
								nextLevel++
							} else {
								break
							}
						}
						if nextLevel <= level {
							end = j
							return
						}
					}
				}
				end = len(lines)
				return
			}
		}
	}
	return -1, -1
}

func replaceHeading(text, heading, content string) string {
	start, end := findHeadingLines(text, heading)
	if start < 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	newLines := append([]string{lines[start]}, strings.Split(content, "\n")...)
	result := append(lines[:start], newLines...)
	result = append(result, lines[end:]...)
	return strings.Join(result, "\n")
}

func prependHeading(text, heading, content string) string {
	start, end := findHeadingLines(text, heading)
	if start < 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	insert := strings.Split(content, "\n")
	newLines := append([]string{lines[start]}, insert...)
	newLines = append(newLines, lines[start+1:end]...)
	result := append(lines[:start], newLines...)
	result = append(result, lines[end:]...)
	return strings.Join(result, "\n")
}

func appendHeading(text, heading, content string) string {
	start, end := findHeadingLines(text, heading)
	if start < 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	insert := strings.Split(content, "\n")
	newLines := append([]string{lines[start]}, lines[start+1:end]...)
	newLines = append(newLines, insert...)
	result := append(lines[:start], newLines...)
	result = append(result, lines[end:]...)
	return strings.Join(result, "\n")
}

func deleteHeading(text, heading string) string {
	start, end := findHeadingLines(text, heading)
	if start < 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	result := append(lines[:start], lines[end:]...)
	return strings.Join(result, "\n")
}

var _ = fmt.Sprintf
