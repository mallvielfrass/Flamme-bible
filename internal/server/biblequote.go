package server

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var htmlTag = regexp.MustCompile(`(?s)<[^>]*>`)

// readBibleQuote читает каталог модуля BibleQuote: bibleqt.ini и файлы книг.
// Текст перевода в репозиторий не кладётся — каталог передаёт оператор.
func readBibleQuote(dir string) ([]moduleVerse, error) {
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("каталог модуля")
	}
	text, err := readBibleQuoteINI(dir)
	if err != nil {
		return nil, err
	}
	chapterSign, verseSign, books := parseBibleQuoteINI(text)
	if len(books) == 0 {
		return nil, fmt.Errorf("в модуле нет книг")
	}
	var verses []moduleVerse
	for i, book := range books {
		raw, err := readInside(dir, book.path)
		if err != nil {
			return nil, err
		}
		html, err := decodePlanText(raw)
		if err != nil {
			return nil, err
		}
		part, err := versesOfBook(i+1, book, html, chapterSign, verseSign)
		if err != nil {
			return nil, err
		}
		verses = append(verses, part...)
	}
	if len(verses) == 0 {
		return nil, fmt.Errorf("в модуле нет стихов")
	}
	return verses, nil
}

func readBibleQuoteINI(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("bibleqt.ini")
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	return decodePlanText(raw)
}

func parseBibleQuoteINI(text string) (string, string, []bqBook) {
	chapterSign := "<h4>"
	verseSign := "<p>"
	var books []bqBook
	var book *bqBook
	flush := func() {
		if book == nil || book.path == "" {
			book = nil
			return
		}
		books = append(books, *book)
		book = nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, ok := cutINI(line)
		if !ok {
			continue
		}
		if strings.EqualFold(key, "PathName") {
			flush()
			book = &bqBook{path: value}
			continue
		}
		if book == nil {
			switch strings.ToLower(key) {
			case "chaptersign":
				if value != "" {
					chapterSign = value
				}
			case "versesign":
				if value != "" {
					verseSign = value
				}
			}
			continue
		}
		if strings.EqualFold(key, "FullName") {
			book.fullName = value
		}
	}
	flush()
	return chapterSign, verseSign, books
}

type bqBook struct {
	path     string
	fullName string
}

func cutINI(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

func readInside(dir, name string) ([]byte, error) {
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("файл книги вне каталога")
	}
	full := filepath.Join(dir, clean)
	rel, err := filepath.Rel(dir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("файл книги вне каталога")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func versesOfBook(bookIndex int, book bqBook, html, chapterSign, verseSign string) ([]moduleVerse, error) {
	chapters := sectionsAfter(html, chapterSign)
	if len(chapters) == 0 {
		chapters = []string{html}
	}
	var out []moduleVerse
	for c, chapterHTML := range chapters {
		seen := map[int]bool{}
		next := 1
		for _, piece := range sectionsAfter(chapterHTML, verseSign) {
			n, text := splitVerseNumber(plainHTML(piece))
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			if n == 0 {
				n = next
			}
			if seen[n] {
				continue
			}
			if utf8.RuneCountInString(text) > 5000 {
				return nil, fmt.Errorf("слишком длинный стих %s %d:%d", book.fullName, c+1, n)
			}
			seen[n] = true
			if n >= next {
				next = n + 1
			}
			out = append(out, moduleVerse{
				Book:     bookIndex,
				BookName: book.fullName,
				Chapter:  c + 1,
				Verse:    n,
				Body:     text,
			})
		}
	}
	return out, nil
}

func sectionsAfter(text, sign string) []string {
	if sign == "" {
		return nil
	}
	lower := strings.ToLower(text)
	needle := strings.ToLower(sign)
	var starts []int
	from := 0
	for from < len(lower) {
		i := strings.Index(lower[from:], needle)
		if i < 0 {
			break
		}
		starts = append(starts, from+i)
		from += i + len(needle)
	}
	out := make([]string, 0, len(starts))
	for i, start := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		out = append(out, text[start:end])
	}
	return out
}

func plainHTML(raw string) string {
	raw = htmlTag.ReplaceAllString(raw, " ")
	raw = strings.NewReplacer(
		"&nbsp;", " ",
		"&#160;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
	).Replace(raw)
	return strings.Join(strings.Fields(raw), " ")
}

func splitVerseNumber(text string) (int, string) {
	text = strings.TrimSpace(text)
	i := 0
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	if i == 0 || i > 4 {
		return 0, text
	}
	n, err := strconv.Atoi(text[:i])
	if err != nil || n <= 0 {
		return 0, text
	}
	return n, strings.TrimSpace(text[i:])
}
