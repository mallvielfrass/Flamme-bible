package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"os"
	"testing"
	"unicode/utf16"

	"flammebible/internal/vbm"
)

func synthTemplate() []byte {
	book := &vbm.Book{
		Header: vbm.Header{
			HeaderShort: "FB",
			HeaderTitle: "FlammeBible Test",
		},
		Title:        "Синтетический сборник",
		ShortName:    "SYNTH",
		Publisher:    "testdata",
		RevisionDate: "2026-11-20",
		Comment:      "synthetic fixture, not a third-party module",
		Songs:        []vbm.Song{songWith("Тестовая песня", "тестовый куплет", fbidA, 0)},
	}
	raw, err := packVBM(book)
	if err != nil {
		panic(err)
	}
	return raw
}

func writeSynthBook(t *testing.T, path string, songs []vbm.Song) {
	t.Helper()
	writeSynthBookIdentified(t, path, "", songs)
}

func writeSynthBookIdentified(t *testing.T, path, title string, songs []vbm.Song) {
	t.Helper()
	raw := synthTemplate()
	book, err := vbm.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if title != "" {
		book.Title = title
	}
	if songs != nil {
		book.Songs = songs
	}
	out, err := vbm.Write(book, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func packVBM(book *vbm.Book) ([]byte, error) {
	var body bytes.Buffer
	for _, value := range []string{book.Title, book.ShortName, book.Publisher, book.RevisionDate, book.Comment} {
		if err := writeUTF16(&body, value); err != nil {
			return nil, err
		}
	}
	writeU32(&body, uint32(len(book.Songs)))
	for _, song := range book.Songs {
		for _, value := range []string{song.Title, song.AltTitle, song.Author, song.Composer, song.Note, song.Properties} {
			if err := writeUTF16(&body, value); err != nil {
				return nil, err
			}
		}
		writeU32(&body, uint32(len(song.Parts)))
		for _, part := range song.Parts {
			if err := writeUTF16(&body, part.Kind); err != nil {
				return nil, err
			}
			if err := writeUTF16(&body, part.Text); err != nil {
				return nil, err
			}
			writeU32(&body, part.Align)
		}
	}
	writeU32(&body, uint32(len(book.Groups)))
	for _, group := range book.Groups {
		if err := writeUTF16(&body, group.Name); err != nil {
			return nil, err
		}
		writeU32(&body, uint32(len(group.Songs)))
		for _, idx := range group.Songs {
			writeU32(&body, idx)
		}
	}
	var zbuf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&zbuf, zlib.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(body.Bytes()); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	compressed := zbuf.Bytes()
	header := make([]byte, 72)
	copy(header, "VisioBibleModule")
	binary.LittleEndian.PutUint32(header[16:], 0x0102)
	binary.LittleEndian.PutUint32(header[20:], uint32(len(compressed)))
	binary.LittleEndian.PutUint32(header[24:], uint32(len(body.Bytes())))
	binary.LittleEndian.PutUint32(header[32:], 0x4b0)
	var out bytes.Buffer
	out.Write(header)
	if err := writeUTF16(&out, book.Header.HeaderShort); err != nil {
		return nil, err
	}
	if err := writeUTF16(&out, book.Header.HeaderTitle); err != nil {
		return nil, err
	}
	out.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	out.Write(compressed)
	out.Write([]byte{0, 0, 0, 0})
	return out.Bytes(), nil
}

func writeU32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

func writeUTF16(buf *bytes.Buffer, value string) error {
	units := utf16.Encode([]rune(value))
	var n [2]byte
	binary.LittleEndian.PutUint16(n[:], uint16(len(units)))
	buf.Write(n[:])
	for _, unit := range units {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], unit)
		buf.Write(b[:])
	}
	return nil
}
