package filemanager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxEditableSize is the max file size editable in the browser (5MB)
const MaxEditableSize = 5 * 1024 * 1024

// editableExtensions are file types we allow editing
var editableExtensions = map[string]string{
	// Web
	"html": "html", "htm": "html", "css": "css", "js": "javascript",
	"json": "json", "xml": "xml", "svg": "xml",
	// Config
	"conf": "nginx", "cfg": "ini", "ini": "ini", "env": "shell",
	"yml": "yaml", "yaml": "yaml", "toml": "toml",
	// Code
	"go": "go", "py": "python", "rb": "ruby", "php": "php",
	"sh": "shell", "bash": "shell", "zsh": "shell",
	"sql": "sql", "rs": "rust", "ts": "typescript",
	// Text
	"txt": "text", "md": "markdown", "log": "text",
	"gitignore": "text", "dockerignore": "text",
	"Makefile": "makefile", "Dockerfile": "dockerfile",
	// No extension defaults
	"": "text",
}

// FileContent is the result from reading a file
type FileContent struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Size     int64  `json:"size"`
	Mode     string `json:"mode"`
	Language string `json:"language"`
	ReadOnly bool   `json:"read_only"`
}

// ReadFile reads a file and returns its content for editing
func ReadFile(path string) (*FileContent, error) {
	path, err := sanitizePath(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("cannot read a directory")
	}

	if info.Size() > MaxEditableSize {
		return nil, fmt.Errorf("file too large to edit (%d bytes, max %d)", info.Size(), MaxEditableSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read file: %w", err)
	}

	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	lang, ok := editableExtensions[ext]
	if !ok {
		lang = "text"
	}

	// Check writability
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	readOnly := err != nil
	if f != nil {
		f.Close()
	}

	return &FileContent{
		Path:     path,
		Name:     filepath.Base(path),
		Content:  string(data),
		Size:     info.Size(),
		Mode:     info.Mode().String(),
		Language: lang,
		ReadOnly: readOnly,
	}, nil
}

// WriteFile writes content to a file
func WriteFile(path, content string) error {
	path, err := sanitizePath(path)
	if err != nil {
		return err
	}
	if isRestricted(path) {
		return fmt.Errorf("cannot write to restricted path")
	}

	// Ensure parent dir exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// Get existing permissions or default
	mode := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode()
	}

	return os.WriteFile(path, []byte(content), mode)
}
