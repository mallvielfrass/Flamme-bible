package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

func findCatalogs(root string) ([]string, error) {
	found := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || !strings.EqualFold(d.Name(), "Modules") {
			return nil
		}
		parent := filepath.Dir(path)
		if programBeside(parent) || programDataModules(path) {
			addCatalog(found, path)
			if err := addINICatalogs(found, parent); err != nil {
				return err
			}
		}
		return fs.SkipDir
	})
	if err != nil {
		return nil, err
	}
	list := make([]string, 0, len(found))
	for _, path := range found {
		list = append(list, path)
	}
	sort.Strings(list)
	return list, nil
}

func programBeside(parent string) bool {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if name == "visiobible.exe" || name == "visiobible.ini" {
			return true
		}
	}
	return false
}

func programDataModules(modules string) bool {
	version := filepath.Base(filepath.Dir(modules))
	programData := filepath.Base(filepath.Dir(filepath.Dir(modules)))
	if !strings.EqualFold(programData, "ProgramData") {
		return false
	}
	return strings.HasPrefix(strings.ToLower(version), "visiobible v")
}

func addINICatalogs(found map[string]string, programDir string) error {
	var iniPath string
	entries, err := os.ReadDir(programDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), "VisioBible.ini") {
			iniPath = filepath.Join(programDir, entry.Name())
			break
		}
	}
	if iniPath == "" {
		return nil
	}
	text, err := readINI(iniPath)
	if err != nil {
		return err
	}
	inBiblePath := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if strings.HasPrefix(line, "[") {
			inBiblePath = strings.EqualFold(line, "[BiblePath]")
			continue
		}
		if !inBiblePath || line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		rel, ok := quotedINIPath(line)
		if !ok {
			continue
		}
		rel = strings.ReplaceAll(rel, `\`, string(os.PathSeparator))
		abs := rel
		if !filepath.IsAbs(rel) {
			abs = filepath.Join(programDir, rel)
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !info.IsDir() {
			abs = filepath.Dir(abs)
		}
		addCatalog(found, catalogOf(abs))
	}
	return nil
}

func quotedINIPath(line string) (string, bool) {
	start := strings.IndexByte(line, '"')
	if start < 0 {
		return "", false
	}
	rest := line[start+1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

func catalogOf(path string) string {
	cur := filepath.Clean(path)
	for {
		if strings.EqualFold(filepath.Base(cur), "Modules") {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(path)
		}
		cur = parent
	}
}

func addCatalog(found map[string]string, path string) {
	path = filepath.Clean(path)
	key := path
	if runtime.GOOS == "windows" {
		key = strings.ToLower(path)
	}
	found[key] = path
}

func readINI(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	if utf8.Valid(data) {
		return string(data), nil
	}
	decoded, err := charmap.Windows1251.NewDecoder().Bytes(data)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func selectCatalog(found []string, dir string) (string, error) {
	if dir != "" {
		dir = filepath.Clean(dir)
		if len(found) == 0 {
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() {
				return "", fmt.Errorf("папка не найдена")
			}
			return dir, nil
		}
		for _, catalog := range found {
			if sameCatalog(catalog, dir) {
				return catalog, nil
			}
		}
		return "", fmt.Errorf("папка не из списка")
	}
	if len(found) == 1 {
		return found[0], nil
	}
	var buf strings.Builder
	buf.WriteString("нужен выбор")
	for _, catalog := range found {
		fmt.Fprintf(&buf, "\n%s", catalog)
	}
	return "", fmt.Errorf("%s", buf.String())
}

func sameCatalog(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func insideCatalog(path, catalog string) bool {
	rel, err := filepath.Rel(catalog, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func moduleFiles(catalog string) ([]string, error) {
	entries, err := os.ReadDir(catalog)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".vbm") {
			continue
		}
		path := filepath.Join(catalog, entry.Name())
		if !insideCatalog(path, catalog) {
			continue
		}
		files = append(files, path)
	}
	return files, nil
}
