package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// verifiedBibleID — BibleId, который VisioBible сохранила в шапке модуля rst+.
const verifiedBibleID = "rfwfwqd3dwea"

const verifiedBibleShort = "RST+"

type bibleBook struct {
	Path      string
	FullName  string
	ShortName string
	Chapters  int
	Hash      string
}

type bibleIndex struct {
	Name      string
	ShortName string
	BibleID   string
	Books     []bibleBook
}

func readBible(dir string) (bibleIndex, error) {
	path, text, err := readBibleINI(dir)
	if err != nil {
		return bibleIndex{}, err
	}
	_ = path
	return parseBibleINI(text), nil
}

func writeVerifiedBibleID(dir string) error {
	path, text, err := readBibleINI(dir)
	if err != nil {
		return err
	}
	index := parseBibleINI(text)
	if index.ShortName != verifiedBibleShort {
		return fmt.Errorf("модуль не проверен")
	}
	if index.BibleID != "" {
		return nil
	}
	updated := insertBibleID(text, verifiedBibleID)
	return os.WriteFile(path, []byte(updated), 0o644)
}

func readBibleINI(dir string) (string, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", err
	}
	var name string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(entry.Name(), "bibleqt.ini") {
			name = entry.Name()
			break
		}
	}
	if name == "" {
		return "", "", fmt.Errorf("bibleqt.ini")
	}
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	text, err := decodePlan(data)
	if err != nil {
		return "", "", err
	}
	return path, text, nil
}

func parseBibleINI(text string) bibleIndex {
	var index bibleIndex
	var book *bibleBook
	flush := func() {
		if book == nil {
			return
		}
		index.Books = append(index.Books, *book)
		book = nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, ok := splitINI(line)
		if !ok {
			continue
		}
		if strings.EqualFold(key, "PathName") {
			flush()
			book = &bibleBook{Path: value}
			continue
		}
		if book == nil {
			switch strings.ToLower(key) {
			case "biblename":
				index.Name = value
			case "bibleshortname":
				index.ShortName = value
			case "bibleid":
				index.BibleID = value
			}
			continue
		}
		switch strings.ToLower(key) {
		case "fullname":
			book.FullName = value
		case "shortname":
			book.ShortName = value
		case "chapterqty":
			n, _ := strconv.Atoi(value)
			book.Chapters = n
		case "bookhash":
			book.Hash = value
		}
	}
	flush()
	return index
}

func splitINI(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

func insertBibleID(text, id string) string {
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		key, _, ok := splitINI(strings.TrimSpace(line))
		if ok && strings.EqualFold(key, "BibleShortName") {
			next := append([]string{}, lines[:i+1]...)
			next = append(next, "BibleId="+id)
			next = append(next, lines[i+1:]...)
			return strings.Join(next, nl)
		}
	}
	return "BibleId=" + id + nl + text
}

func formatBible(index bibleIndex) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "%s %s\n", index.ShortName, index.BibleID)
	for _, book := range index.Books {
		fmt.Fprintf(&buf, "%s %s\n", book.Path, book.Hash)
	}
	return buf.String()
}
