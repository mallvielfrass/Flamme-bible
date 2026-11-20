package server

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// repairMojibake ищет текст, который уже является валидным UTF-8, но получился
// из кириллицы: байты UTF-8 или windows-1251 прочитали как windows-1251 либо
// latin-1/windows-1252 и сохранили снова. Функция ничего не меняет в файле.
// Починка принимается, только если обратный прогон дал валидный текст и он
// заметно читаемее: доля кириллицы не упала, а пары Ð/Ñ или Р/С исчезли,
// либо строка вида «Ïðèâåò» целиком собралась в кириллическое слово.
func repairMojibake(text string) (string, bool) {
	if text == "" {
		return "", false
	}
	var best string
	found := false
	consider := func(cand string) {
		if !mojibakeBetter(text, cand) {
			return
		}
		if !found || preferMojibake(cand, best) {
			best = cand
			found = true
		}
	}
	for _, enc := range []*charmap.Charmap{charmap.Windows1251, charmap.Windows1252, charmap.ISO8859_1} {
		raw, ok := encodeCharmap(enc, text)
		if !ok {
			continue
		}
		if utf8.Valid(raw) {
			consider(string(raw))
		}
		if enc != charmap.Windows1251 {
			consider(decodeCharmap(charmap.Windows1251, raw))
		}
	}
	if !found || best == text {
		return "", false
	}
	return best, true
}

// encodeWindows1251 кодирует текст без замены символов.
// Непредставимый символ — ошибка, а не «?»: вызывающий не должен писать файл.
func encodeWindows1251(text string) ([]byte, error) {
	raw, ok := encodeCharmap(charmap.Windows1251, text)
	if !ok {
		return nil, errUnmappable
	}
	return raw, nil
}

func encodeCharmap(cm *charmap.Charmap, text string) ([]byte, bool) {
	out := make([]byte, 0, len(text))
	for _, r := range text {
		b, ok := cm.EncodeRune(r)
		if !ok {
			return nil, false
		}
		out = append(out, b)
	}
	return out, true
}

func decodeCharmap(cm *charmap.Charmap, raw []byte) string {
	var buf strings.Builder
	buf.Grow(len(raw))
	for _, b := range raw {
		buf.WriteRune(cm.DecodeByte(b))
	}
	return buf.String()
}

func mojibakeBetter(before, after string) bool {
	if after == "" || after == before || strings.ContainsRune(after, '\uFFFD') || !utf8.ValidString(after) {
		return false
	}
	pairsBefore, pairsAfter := markerPairs(before), markerPairs(after)
	cyrBefore, cyrAfter := cyrillicCount(before), cyrillicCount(after)
	shareBefore, shareAfter := cyrillicShare(before), cyrillicShare(after)
	// UTF-8, прочитанный как однобайтовая кодировка, раздувает строку: «П» становится «ÐŸ» или «Рџ».
	if pairsBefore > 0 && pairsAfter == 0 && cyrAfter > 0 && shareAfter+0.001 >= shareBefore && utf8.RuneCountInString(after) < utf8.RuneCountInString(before) {
		return true
	}
	// windows-1251, прочитанный как latin-1: длина та же, кириллицы становится больше.
	if cyrAfter > cyrBefore && shareAfter > shareBefore && pairsAfter <= pairsBefore && cp1251ReadAsLatin1(before, after) {
		return true
	}
	return false
}

func preferMojibake(a, b string) bool {
	pairsA, pairsB := markerPairs(a), markerPairs(b)
	if pairsA != pairsB {
		return pairsA < pairsB
	}
	shareA, shareB := cyrillicShare(a), cyrillicShare(b)
	if shareA != shareB {
		return shareA > shareB
	}
	return utf8.RuneCountInString(a) < utf8.RuneCountInString(b)
}

// markerPairs считает пары, типичные для UTF-8, прочитанного как latin-1 (Ð/Ñ)
// или как windows-1251 (Р/С), за которыми идёт байт продолжения UTF-8.
func markerPairs(text string) int {
	rs := []rune(text)
	n := 0
	for i := 0; i+1 < len(rs); i++ {
		next := rs[i+1]
		switch rs[i] {
		case '\u00D0', '\u00D1':
			if next >= 0x80 && next <= 0xBF {
				n++
				continue
			}
			if b, ok := charmap.Windows1252.EncodeRune(next); ok && b >= 0x80 && b <= 0xBF {
				n++
			}
		case 'Р', 'С':
			if b, ok := charmap.Windows1251.EncodeRune(next); ok && b >= 0x80 && b <= 0xBF {
				n++
			}
		}
	}
	return n
}

func cp1251ReadAsLatin1(before, after string) bool {
	b := []rune(before)
	a := []rune(after)
	if len(b) != len(a) {
		return false
	}
	changed := 0
	distinctive := false
	vowel := false
	for i := range b {
		if b[i] == a[i] {
			continue
		}
		if b[i] < 0x80 || unicode.Is(unicode.Cyrillic, b[i]) || !isCyrillicLetter(a[i]) {
			return false
		}
		if isCyrillicVowel(a[i]) {
			vowel = true
		}
		if distinctiveLatin1(b[i]) {
			distinctive = true
		}
		changed++
	}
	return changed >= 4 && distinctive && vowel
}

// distinctiveLatin1 — буквы, которые в немецком и французском почти не встречаются,
// а в кириллице windows-1251, прочитанной как latin-1, обычны (р, е, ю, ш, …).
func distinctiveLatin1(r rune) bool {
	switch r {
	case 'ð', 'þ', 'ø', '÷', 'ÿ', 'Ï', 'Ð', 'ý', '¸', '¨', 'å', 'æ', 'õ', 'ã':
		return true
	default:
		return false
	}
}

func isCyrillicLetter(r rune) bool {
	return unicode.IsLetter(r) && unicode.In(r, unicode.Cyrillic)
}

func isCyrillicVowel(r rune) bool {
	switch r {
	case 'а', 'е', 'и', 'о', 'у', 'ы', 'э', 'ю', 'я', 'ё',
		'А', 'Е', 'И', 'О', 'У', 'Ы', 'Э', 'Ю', 'Я', 'Ё':
		return true
	default:
		return false
	}
}

func cyrillicCount(text string) int {
	n := 0
	for _, r := range text {
		if isCyrillicLetter(r) {
			n++
		}
	}
	return n
}

func cyrillicShare(text string) float64 {
	letters := 0
	cyr := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
			if isCyrillicLetter(r) {
				cyr++
			}
		}
	}
	if letters == 0 {
		return 0
	}
	return float64(cyr) / float64(letters)
}
