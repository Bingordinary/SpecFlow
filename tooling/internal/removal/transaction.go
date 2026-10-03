package removal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type backup struct {
	path, saved string
	data        []byte
	mode        os.FileMode
}

// transact keeps every original file until the tentative state has passed
// its post-check. Ordinary rename/write failures restore specs and metadata.
func transact(root string, p *plan, rename func(string, string) error) (err error) {
	var backups []backup
	var temps []string
	defer func() {
		if err != nil {
			for i := len(backups) - 1; i >= 0; i-- {
				b := backups[i]
				restoreErr := os.Rename(b.saved, b.path)
				if os.IsNotExist(restoreErr) {
					// A later backup-cleanup failure can occur after earlier
					// backups were removed. Keep their original bytes until commit.
					restoreErr = os.WriteFile(b.path, b.data, b.mode)
					if restoreErr == nil {
						restoreErr = os.Chmod(b.path, b.mode)
					}
				}
				if restoreErr != nil {
					err = errors.Join(err, fmt.Errorf("restore %s: %w", b.path, restoreErr))
				}
			}
		}
		for _, path := range temps {
			if cleanupErr := os.Remove(path); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()
	paths := append(append([]string(nil), p.result.Files...), p.result.Updates...)
	for _, path := range paths {
		abs := filepath.Join(root, filepath.FromSlash(path))
		original, readErr := readFile(root, path)
		if readErr != nil {
			return readErr
		}
		info, statErr := os.Stat(abs)
		if statErr != nil {
			return statErr
		}
		file, createErr := os.CreateTemp(filepath.Dir(abs), ".sf-remove-*")
		if createErr != nil {
			return createErr
		}
		saved := file.Name()
		temps = append(temps, saved)
		if err = file.Close(); err != nil {
			return err
		}
		if err = rename(abs, saved); err != nil {
			return fmt.Errorf("stage removal %s: %w", path, err)
		}
		backups = append(backups, backup{abs, saved, original, info.Mode().Perm()})
		if data, ok := p.writes[path]; ok {
			if err = os.WriteFile(abs, data, info.Mode().Perm()); err != nil {
				return fmt.Errorf("update %s: %w", path, err)
			}
			if err = os.Chmod(abs, info.Mode().Perm()); err != nil {
				return fmt.Errorf("preserve permissions %s: %w", path, err)
			}
		}
	}
	for _, path := range p.result.Files {
		if _, statErr := os.Lstat(filepath.Join(root, filepath.FromSlash(path))); !os.IsNotExist(statErr) {
			return fmt.Errorf("removal incomplete: %s remains (%v)", path, statErr)
		}
	}
	docs, err := inventory(root)
	if err != nil {
		return err
	}
	if blockers := referenceBlockers(root, docs, p.deleted); len(blockers) != 0 {
		return fmt.Errorf("post-removal reference check: %v", blockers)
	}
	for path, expected := range p.writes {
		actual, readErr := readFile(root, path)
		if readErr != nil || !bytes.Equal(actual, expected) {
			return fmt.Errorf("record cleanup incomplete: %s (%v)", path, readErr)
		}
	}
	for _, saved := range temps {
		if err = os.Remove(saved); err != nil {
			return fmt.Errorf("commit removal: %w", err)
		}
	}
	temps = nil
	return nil
}
