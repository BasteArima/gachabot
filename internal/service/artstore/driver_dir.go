package artstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dirStore writes into a local directory. Used for development, where there is
// no art host to reach, and as a fallback if SFTP is unavailable.
type dirStore struct{ root string }

func newDir(root string) (driver, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("не удалось открыть папку %s: %w", abs, err)
	}
	return &dirStore{root: abs}, nil
}

// resolve joins a relative path to the root and refuses anything that would
// escape it. The caller already validated the folder and slug, so this is the
// second lock on the same door.
func (d *dirStore) resolve(rel string) (string, error) {
	full := filepath.Join(d.root, filepath.FromSlash(rel))
	if r, err := filepath.Rel(d.root, full); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("путь вне корня: %s", rel)
	}
	return full, nil
}

func (d *dirStore) exists(rel string) (bool, error) {
	full, err := d.resolve(rel)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(full)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (d *dirStore) put(rel string, data []byte) error {
	full, err := d.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	// Write beside the target and rename, so a half-written file is never served.
	tmp := full + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, full); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (d *dirStore) describe() string { return "папка " + d.root }
func (d *dirStore) close() error     { return nil }
