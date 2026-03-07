package filemanager

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MaxUploadSize is 100MB
const MaxUploadSize = 100 * 1024 * 1024

// SaveUpload saves an uploaded file to the target directory
func SaveUpload(destDir, filename string, reader io.Reader) (string, error) {
	destDir, err := sanitizePath(destDir)
	if err != nil {
		return "", err
	}
	if isRestricted(destDir) {
		return "", fmt.Errorf("cannot upload to restricted path")
	}

	// Clean filename to prevent path traversal
	filename = filepath.Base(filename)
	if filename == "." || filename == ".." {
		return "", fmt.Errorf("invalid filename")
	}

	fullPath := filepath.Join(destDir, filename)

	// Ensure directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("cannot create directory: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("cannot create file: %w", err)
	}
	defer f.Close()

	// Copy with size limit
	limited := io.LimitReader(reader, MaxUploadSize+1)
	written, err := io.Copy(f, limited)
	if err != nil {
		os.Remove(fullPath)
		return "", fmt.Errorf("upload failed: %w", err)
	}
	if written > MaxUploadSize {
		os.Remove(fullPath)
		return "", fmt.Errorf("file exceeds max upload size (%d MB)", MaxUploadSize/(1024*1024))
	}

	return fullPath, nil
}

// ZipDownload creates a zip archive from a directory for download
func ZipDownload(sourcePath string, writer io.Writer) error {
	sourcePath, err := sanitizePath(sourcePath)
	if err != nil {
		return err
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("path not found: %w", err)
	}

	zipWriter := zip.NewWriter(writer)
	defer zipWriter.Close()

	if !info.IsDir() {
		// Single file
		return addFileToZip(zipWriter, sourcePath, filepath.Base(sourcePath))
	}

	// Directory — walk and add all files
	basePath := filepath.Dir(sourcePath)
	return filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}

		return addFileToZip(zipWriter, path, relPath)
	})
}

func addFileToZip(zw *zip.Writer, filePath, zipPath string) error {
	// Normalize to forward slashes for zip
	zipPath = strings.ReplaceAll(zipPath, "\\", "/")

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = zipPath
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

// GetDownloadInfo returns file info for download (name, size)
func GetDownloadInfo(path string) (string, int64, error) {
	path, err := sanitizePath(path)
	if err != nil {
		return "", 0, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", 0, fmt.Errorf("file not found: %w", err)
	}

	return info.Name(), info.Size(), nil
}
