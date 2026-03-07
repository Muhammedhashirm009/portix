package filemanager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileEntry represents a file or directory in a listing
type FileEntry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	IsDir      bool      `json:"is_dir"`
	Size       int64     `json:"size"`
	Mode       string    `json:"mode"`
	ModTime    time.Time `json:"mod_time"`
	Owner      string    `json:"owner"`
	Group      string    `json:"group"`
	Extension  string    `json:"extension,omitempty"`
	IsSymlink  bool      `json:"is_symlink"`
	LinkTarget string    `json:"link_target,omitempty"`
}

// BrowseResult is the response from listing a directory
type BrowseResult struct {
	Path       string      `json:"path"`
	Parent     string      `json:"parent"`
	Breadcrumb []PathPart  `json:"breadcrumb"`
	Items      []FileEntry `json:"items"`
	TotalFiles int         `json:"total_files"`
	TotalDirs  int         `json:"total_dirs"`
}

// PathPart represents one segment in a breadcrumb
type PathPart struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Restricted paths that cannot be modified
var restrictedPaths = []string{
	"/proc", "/sys", "/dev", "/run",
}

// isRestricted checks if a path is in the restricted list
func isRestricted(path string) bool {
	abs, _ := filepath.Abs(path)
	for _, r := range restrictedPaths {
		if abs == r || strings.HasPrefix(abs, r+"/") {
			return true
		}
	}
	return false
}

// sanitizePath cleans and validates a path
func sanitizePath(path string) (string, error) {
	if path == "" {
		path = "/"
	}

	cleaned := filepath.Clean(path)

	// Must be absolute
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("path must be absolute: %s", path)
	}

	return cleaned, nil
}

// Browse lists the contents of a directory
func Browse(dirPath string) (*BrowseResult, error) {
	dirPath, err := sanitizePath(dirPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("cannot access path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dirPath)
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read directory: %w", err)
	}

	items := make([]FileEntry, 0, len(entries))
	totalFiles, totalDirs := 0, 0

	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		fullPath := filepath.Join(dirPath, e.Name())
		entry := FileEntry{
			Name:    e.Name(),
			Path:    fullPath,
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime(),
		}

		// Check symlink
		if info.Mode()&os.ModeSymlink != 0 {
			entry.IsSymlink = true
			target, err := os.Readlink(fullPath)
			if err == nil {
				entry.LinkTarget = target
			}
		}

		// Extension
		if !e.IsDir() {
			entry.Extension = strings.TrimPrefix(filepath.Ext(e.Name()), ".")
			totalFiles++
		} else {
			totalDirs++
		}

		items = append(items, entry)
	}

	// Sort: directories first, then alphabetically
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	// Build breadcrumb
	breadcrumb := buildBreadcrumb(dirPath)

	// Parent directory
	parent := filepath.Dir(dirPath)
	if parent == dirPath {
		parent = ""
	}

	return &BrowseResult{
		Path:       dirPath,
		Parent:     parent,
		Breadcrumb: breadcrumb,
		Items:      items,
		TotalFiles: totalFiles,
		TotalDirs:  totalDirs,
	}, nil
}

func buildBreadcrumb(path string) []PathPart {
	parts := []PathPart{{Name: "/", Path: "/"}}
	if path == "/" {
		return parts
	}

	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	current := ""
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		current += "/" + seg
		parts = append(parts, PathPart{Name: seg, Path: current})
	}
	return parts
}

// CreateDirectory creates a new directory
func CreateDirectory(path string) error {
	path, err := sanitizePath(path)
	if err != nil {
		return err
	}
	if isRestricted(path) {
		return fmt.Errorf("cannot create in restricted path")
	}
	return os.MkdirAll(path, 0755)
}

// CreateFile creates a new empty file
func CreateFile(path string) error {
	path, err := sanitizePath(path)
	if err != nil {
		return err
	}
	if isRestricted(path) {
		return fmt.Errorf("cannot create in restricted path")
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}

// Rename renames a file or directory
func Rename(oldPath, newName string) error {
	oldPath, err := sanitizePath(oldPath)
	if err != nil {
		return err
	}
	if isRestricted(oldPath) {
		return fmt.Errorf("cannot rename restricted path")
	}

	dir := filepath.Dir(oldPath)
	newPath := filepath.Join(dir, newName)

	return os.Rename(oldPath, newPath)
}

// Move moves a file or directory to a new location
func Move(src, destDir string) error {
	src, err := sanitizePath(src)
	if err != nil {
		return err
	}
	destDir, err = sanitizePath(destDir)
	if err != nil {
		return err
	}
	if isRestricted(src) {
		return fmt.Errorf("cannot move restricted path")
	}

	destPath := filepath.Join(destDir, filepath.Base(src))
	return os.Rename(src, destPath)
}

// Copy copies a file or directory
func Copy(src, destDir string) error {
	src, err := sanitizePath(src)
	if err != nil {
		return err
	}
	destDir, err = sanitizePath(destDir)
	if err != nil {
		return err
	}
	if isRestricted(src) {
		return fmt.Errorf("cannot copy restricted path")
	}

	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	destPath := filepath.Join(destDir, filepath.Base(src))

	if info.IsDir() {
		return copyDir(src, destPath)
	}
	return copyFile(src, destPath)
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	info, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	destFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// Delete removes a file or directory
func Delete(path string) error {
	path, err := sanitizePath(path)
	if err != nil {
		return err
	}
	if isRestricted(path) {
		return fmt.Errorf("cannot delete restricted path")
	}
	if path == "/" {
		return fmt.Errorf("cannot delete root")
	}

	return os.RemoveAll(path)
}

// ChangePermissions changes file mode
func ChangePermissions(path string, mode os.FileMode) error {
	path, err := sanitizePath(path)
	if err != nil {
		return err
	}
	if isRestricted(path) {
		return fmt.Errorf("cannot change permissions on restricted path")
	}
	return os.Chmod(path, mode)
}

// SearchResult holds a search match
type SearchResult struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

// Search recursively searches for files matching a pattern
func Search(root, pattern string, maxResults int) ([]SearchResult, error) {
	root, err := sanitizePath(root)
	if err != nil {
		return nil, err
	}

	if maxResults <= 0 {
		maxResults = 100
	}

	var results []SearchResult
	pattern = strings.ToLower(pattern)

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if len(results) >= maxResults {
			return filepath.SkipAll
		}
		if isRestricted(path) {
			return filepath.SkipDir
		}

		if strings.Contains(strings.ToLower(info.Name()), pattern) {
			results = append(results, SearchResult{
				Path:    path,
				Name:    info.Name(),
				IsDir:   info.IsDir(),
				Size:    info.Size(),
				ModTime: info.ModTime().Format(time.RFC3339),
			})
		}
		return nil
	})

	return results, nil
}
