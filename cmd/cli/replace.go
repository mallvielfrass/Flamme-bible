package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flammebible/internal/vbm"
)

func (s *Store) Sync(modulePath string, apply bool) (string, error) {
	changes, err := s.Diff(modulePath)
	if err != nil {
		return "", err
	}
	report := formatChanges(changes)
	if !apply {
		return report, nil
	}
	binding, err := s.Bind(modulePath)
	if err != nil {
		return report, err
	}
	if err := s.SetStage(binding.ID, "writing"); err != nil {
		return report, err
	}
	data, err := os.ReadFile(modulePath)
	if err != nil {
		return report, err
	}
	book, err := vbm.Parse(data)
	if err != nil {
		_ = s.SetStage(binding.ID, "restored")
		return report, err
	}
	if err := putNewFBIDs(book, changes); err != nil {
		_ = s.SetStage(binding.ID, "restored")
		return report, err
	}
	if err := backupModule(modulePath); err != nil {
		_ = s.SetStage(binding.ID, "restored")
		return report, err
	}
	payload, err := vbm.Write(book, data)
	if err != nil {
		_ = restoreModule(modulePath)
		_ = s.SetStage(binding.ID, "restored")
		return report, err
	}
	if err := commitModule(modulePath, payload); err != nil {
		_ = s.SetStage(binding.ID, "restored")
		return report, err
	}
	if err := s.SetStage(binding.ID, "done"); err != nil {
		return report, err
	}
	return report, nil
}

func formatChanges(changes []Change) string {
	if len(changes) == 0 {
		return "изменений нет\n"
	}
	var buf strings.Builder
	for _, change := range changes {
		switch change.Kind {
		case "added":
			fmt.Fprintf(&buf, "добавлена %s %s\n", change.FBID, change.Title)
		case "changed":
			fmt.Fprintf(&buf, "изменена %s %s\n", change.FBID, change.Title)
		case "deleted":
			fmt.Fprintf(&buf, "удалена %s %s\n", change.FBID, change.Title)
		}
	}
	return buf.String()
}

func putNewFBIDs(book *vbm.Book, changes []Change) error {
	assigned := map[int]string{}
	for _, change := range changes {
		if change.Kind == "added" && change.Index >= 0 && change.FBID != "" {
			assigned[change.Index] = change.FBID
		}
	}
	for i := range book.Songs {
		fbid, ok := assigned[i]
		if !ok {
			continue
		}
		_, has, err := songFBID(book.Songs[i].Properties)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		book.Songs[i].Properties = appendFBID(book.Songs[i].Properties, fbid)
	}
	return nil
}

func appendFBID(properties, fbid string) string {
	line := "$FBID$=" + fbid
	if properties == "" {
		return line + "\r\n"
	}
	sep := "\n"
	if strings.Contains(properties, "\r\n") {
		sep = "\r\n"
	}
	if strings.HasSuffix(properties, "\n") {
		return properties + line + sep
	}
	return properties + sep + line + sep
}

func replacePayload(path string, payload []byte) error {
	if err := backupModule(path); err != nil {
		return err
	}
	return commitModule(path, payload)
}

func commitModule(path string, payload []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".flammebible-*.tmp")
	if err != nil {
		_ = restoreModule(path)
		return err
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(payload)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmpName)
		_ = restoreModule(path)
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	if _, err := vbm.ParseFile(tmpName); err != nil {
		os.Remove(tmpName)
		_ = restoreModule(path)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		_ = restoreModule(path)
		return err
	}
	if _, err := vbm.ParseFile(path); err != nil {
		_ = restoreModule(path)
		return err
	}
	return nil
}

func backupModule(path string) error {
	if err := copyFile(path, path+".bak"); err != nil {
		return err
	}
	vbi := siblingVBI(path)
	if _, err := os.Stat(vbi); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return copyFile(vbi, vbi+".bak")
}

func restoreModule(path string) error {
	if err := copyFile(path+".bak", path); err != nil {
		return err
	}
	vbiBak := siblingVBI(path) + ".bak"
	if _, err := os.Stat(vbiBak); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return copyFile(vbiBak, siblingVBI(path))
}

func siblingVBI(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + ".vbi"
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
