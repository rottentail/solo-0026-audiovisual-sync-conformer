package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	handle, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempPath := handle.Name()
	defer os.Remove(tempPath)
	if err := handle.Chmod(0o600); err != nil {
		handle.Close()
		return fmt.Errorf("set temporary permissions: %w", err)
	}
	if _, err := handle.Write(data); err != nil {
		handle.Close()
		return fmt.Errorf("write staged bytes: %w", err)
	}
	if err := handle.Sync(); err != nil {
		handle.Close()
		return fmt.Errorf("flush staged bytes: %w", err)
	}
	if err := handle.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace destination file: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
