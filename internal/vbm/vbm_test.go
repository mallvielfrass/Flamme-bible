package vbm

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func sampleBook() *Book {
	return &Book{
		Header: Header{
			FormatVersion: "0x102",
			Flags:         0,
			Charset:       "0x4b0",
			HeaderShort:   "FB",
			HeaderTitle:   "FlammeBible Test",
		},
		Title:        "Синтетический сборник",
		ShortName:    "SYNTH",
		Publisher:    "testdata",
		RevisionDate: "2026-11-20",
		Comment:      "synthetic fixture, not a third-party module",
		Songs: []Song{
			{
				Title:      "Тестовая песня",
				Properties: "$TUNE$=\r\n$ID$=1\r\n$FBID$=fba56871-f5d6-4b32-8b42-4bca03278ec1\r\n",
				Parts:      []Part{{Kind: "Куплет", Text: "тестовый куплет", Align: 0}},
			},
			{
				Title:      "Вторая тестовая",
				Properties: "$TUNE$=\r\n$ID$=2\r\n",
				Parts:      []Part{{Kind: "Куплет", Text: "ещё одна строка\nвторая", Align: 0}},
			},
		},
		Groups: []Group{},
	}
}

func packVBM(t *testing.T, book *Book) []byte {
	t.Helper()
	raw, err := encodeStandalone(book, []byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func encodeStandalone(book *Book, extension, tail []byte) ([]byte, error) {
	var body bytes.Buffer
	for _, value := range []string{book.Title, book.ShortName, book.Publisher, book.RevisionDate, book.Comment} {
		if err := writeText(&body, value); err != nil {
			return nil, err
		}
	}
	writeU32(&body, uint32(len(book.Songs)))
	for _, song := range book.Songs {
		for _, value := range []string{song.Title, song.AltTitle, song.Author, song.Composer, song.Note, song.Properties} {
			if err := writeText(&body, value); err != nil {
				return nil, err
			}
		}
		writeU32(&body, uint32(len(song.Parts)))
		for _, part := range song.Parts {
			if err := writeText(&body, part.Kind); err != nil {
				return nil, err
			}
			if err := writeText(&body, part.Text); err != nil {
				return nil, err
			}
			writeU32(&body, part.Align)
		}
	}
	writeU32(&body, uint32(len(book.Groups)))
	for _, group := range book.Groups {
		if err := writeText(&body, group.Name); err != nil {
			return nil, err
		}
		writeU32(&body, uint32(len(group.Songs)))
		for _, idx := range group.Songs {
			writeU32(&body, idx)
		}
	}
	compressed, err := compress(body.Bytes())
	if err != nil {
		return nil, err
	}
	header := make([]byte, headerFixed)
	copy(header, magic)
	binary.LittleEndian.PutUint32(header[16:], 0x0102)
	binary.LittleEndian.PutUint32(header[20:], uint32(len(compressed)))
	binary.LittleEndian.PutUint32(header[24:], uint32(len(body.Bytes())))
	binary.LittleEndian.PutUint32(header[28:], book.Header.Flags)
	binary.LittleEndian.PutUint32(header[32:], 0x4b0)
	var out bytes.Buffer
	out.Write(header)
	if err := writeText(&out, book.Header.HeaderShort); err != nil {
		return nil, err
	}
	if err := writeText(&out, book.Header.HeaderTitle); err != nil {
		return nil, err
	}
	out.Write(extension)
	out.Write(compressed)
	if tail == nil {
		tail = []byte{0, 0, 0, 0}
	}
	out.Write(tail)
	return out.Bytes(), nil
}

func TestParseSyntheticRoundTrip(t *testing.T) {
	want := sampleBook()
	raw := packVBM(t, want)
	got, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !sameBook(want, got) {
		t.Fatal("разбор синтетического файла не совпал с исходной книгой")
	}
}

func TestParseRejectsTruncatedAndWrongSize(t *testing.T) {
	raw := packVBM(t, sampleBook())
	if _, err := Parse(raw[:80]); err == nil {
		t.Fatal("обрезанный файл принят")
	}
	wrongUncompressed := append([]byte(nil), raw...)
	wrongUncompressed[24] = 1
	wrongUncompressed[25] = 0
	wrongUncompressed[26] = 0
	wrongUncompressed[27] = 0
	if _, err := Parse(wrongUncompressed); err == nil {
		t.Fatal("неверный распакованный размер принят")
	}
	wrongCompressed := append([]byte(nil), raw...)
	csize := len(raw) - 4 - headerFixed
	wrongCompressed[20] = byte(csize)
	wrongCompressed[21] = byte(csize >> 8)
	wrongCompressed[22] = byte(csize >> 16)
	wrongCompressed[23] = byte(csize >> 24)
	if _, err := Parse(wrongCompressed); err == nil {
		t.Fatal("неверный сжатый размер принят")
	}
	broken := append([]byte(nil), raw...)
	broken[len(broken)/2] ^= 0xff
	if _, err := Parse(broken); err == nil {
		t.Fatal("повреждённый поток принят")
	}
}

func TestParseHonorsSizeLimits(t *testing.T) {
	raw := packVBM(t, sampleBook())
	prevFile := maxFileSize
	maxFileSize = len(raw) - 1
	_, err := Parse(raw)
	maxFileSize = prevFile
	if err == nil {
		t.Fatal("файл сверх лимита принят")
	}
	prevBody := maxUncompressed
	maxUncompressed = 64
	_, err = Parse(raw)
	maxUncompressed = prevBody
	if err == nil {
		t.Fatal("распаковка сверх лимита принята")
	}
}

func TestWriteRoundTrip(t *testing.T) {
	book := sampleBook()
	raw := packVBM(t, book)
	template := append([]byte(nil), raw...)
	parsed, err := Parse(template)
	if err != nil {
		t.Fatal(err)
	}
	template[40] = 0xAB
	template[parsed.Header.ZlibOffset-1] = 0xCD
	out, err := Write(parsed, template)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	written := filepath.Join(dir, "rewritten.vbm")
	if err := os.WriteFile(written, out, 0o644); err != nil {
		t.Fatal(err)
	}
	if out[40] != 0xAB || out[parsed.Header.ZlibOffset-1] != 0xCD {
		t.Fatal("непрочитанные байты заголовка не сохранены")
	}
	if !bytes.Equal(out[:20], template[:20]) || !bytes.Equal(out[28:headerFixed], template[28:headerFixed]) {
		t.Fatal("фиксированный заголовок изменён")
	}
	if !bytes.Equal(out[len(out)-4:], template[len(template)-4:]) {
		t.Fatal("хвост шаблона изменён")
	}
	again, err := Parse(out)
	if err != nil {
		t.Fatalf("повторный разбор: %v", err)
	}
	if !sameBook(parsed, again) {
		t.Fatal("повторный разбор не равен исходному")
	}
}

func TestWriteRejectsGroups(t *testing.T) {
	book := sampleBook()
	book.Groups = []Group{{Name: "группа", Songs: []uint32{0}}}
	grouped, err := encodeStandalone(book, []byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte{0, 0, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(grouped)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Groups) == 0 {
		t.Fatal("копия с группами не разобрана")
	}
	before := append([]byte(nil), grouped...)
	if _, err := Write(parsed, grouped); err == nil {
		t.Fatal("файл с группами записан")
	}
	if !bytes.Equal(grouped, before) {
		t.Fatal("отказ записи изменил исходные байты")
	}
}

func sameBook(a, b *Book) bool {
	if a.Title != b.Title || a.ShortName != b.ShortName || a.Publisher != b.Publisher || a.RevisionDate != b.RevisionDate || a.Comment != b.Comment {
		return false
	}
	if a.Header.HeaderShort != b.Header.HeaderShort || a.Header.HeaderTitle != b.Header.HeaderTitle {
		return false
	}
	if a.BytesLeft != b.BytesLeft || !reflect.DeepEqual(a.Songs, b.Songs) {
		return false
	}
	if len(a.Groups) != len(b.Groups) || (len(a.Groups) > 0 && !reflect.DeepEqual(a.Groups, b.Groups)) {
		return false
	}
	return true
}
