package vbm

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"unicode/utf16"
)

const (
	magic       = "VisioBibleModule"
	headerFixed = 72
	maxCount    = 1 << 20
)

var (
	maxFileSize     = 32 << 20
	maxUncompressed = 64 << 20
)

type Book struct {
	Header       Header  `json:"header"`
	Title        string  `json:"title"`
	ShortName    string  `json:"short_name"`
	Publisher    string  `json:"publisher"`
	RevisionDate string  `json:"revision_date"`
	Comment      string  `json:"comment"`
	Songs        []Song  `json:"songs"`
	Groups       []Group `json:"groups"`
	BytesLeft    int     `json:"bytes_left"`
}

type Header struct {
	FormatVersion string `json:"format_version"`
	Compressed    int    `json:"compressed"`
	Uncompressed  int    `json:"uncompressed"`
	Flags         uint32 `json:"flags"`
	Charset       string `json:"charset"`
	HeaderShort   string `json:"header_short"`
	HeaderTitle   string `json:"header_title"`
	ZlibOffset    int    `json:"zlib_offset"`
}

type Song struct {
	Title      string `json:"title"`
	AltTitle   string `json:"alt_title"`
	Author     string `json:"author"`
	Composer   string `json:"composer"`
	Note       string `json:"note"`
	Properties string `json:"properties"`
	Parts      []Part `json:"parts"`
}

type Part struct {
	Kind  string `json:"kind"`
	Text  string `json:"text"`
	Align uint32 `json:"align"`
}

type Group struct {
	Name  string   `json:"name"`
	Songs []uint32 `json:"songs"`
}

type reader struct {
	b []byte
	o int
}

func ParseFile(path string) (*Book, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > int64(maxFileSize) {
		return nil, fmt.Errorf("файл больше допустимого размера")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Book, error) {
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("файл больше допустимого размера")
	}
	if len(data) < headerFixed || string(data[:16]) != magic {
		return nil, fmt.Errorf("повреждённый файл")
	}
	ver := binary.LittleEndian.Uint32(data[16:])
	csize := binary.LittleEndian.Uint32(data[20:])
	usize := binary.LittleEndian.Uint32(data[24:])
	flags := binary.LittleEndian.Uint32(data[28:])
	charset := binary.LittleEndian.Uint32(data[32:])
	if int(csize) > len(data)-4 {
		return nil, fmt.Errorf("неверный размер")
	}
	start := len(data) - 4 - int(csize)
	head := reader{b: data, o: headerFixed}
	headerShort, err := head.str()
	if err != nil {
		return nil, err
	}
	headerTitle, err := head.str()
	if err != nil {
		return nil, err
	}
	if start < head.o {
		return nil, fmt.Errorf("неверный размер")
	}
	if usize > uint32(maxUncompressed) {
		return nil, fmt.Errorf("распаковка больше допустимого размера")
	}
	body, err := decompress(data[start:len(data)-4], int(usize))
	if err != nil {
		return nil, err
	}
	book, err := parseBody(body)
	if err != nil {
		return nil, err
	}
	book.Header = Header{
		FormatVersion: fmt.Sprintf("0x%x", ver),
		Compressed:    int(csize),
		Uncompressed:  int(usize),
		Flags:         flags,
		Charset:       fmt.Sprintf("0x%x", charset),
		HeaderShort:   headerShort,
		HeaderTitle:   headerTitle,
		ZlibOffset:    start,
	}
	return book, nil
}

func decompress(payload []byte, usize int) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("повреждённый файл: %w", err)
	}
	defer zr.Close()
	body, err := io.ReadAll(io.LimitReader(zr, int64(usize)+1))
	if err != nil {
		return nil, fmt.Errorf("повреждённый файл: %w", err)
	}
	if len(body) != usize {
		return nil, fmt.Errorf("неверный размер")
	}
	return body, nil
}

func parseBody(body []byte) (*Book, error) {
	r := reader{b: body}
	title, err := r.str()
	if err != nil {
		return nil, err
	}
	shortName, err := r.str()
	if err != nil {
		return nil, err
	}
	publisher, err := r.str()
	if err != nil {
		return nil, err
	}
	revisionDate, err := r.str()
	if err != nil {
		return nil, err
	}
	comment, err := r.str()
	if err != nil {
		return nil, err
	}
	songCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	if songCount > maxCount {
		return nil, fmt.Errorf("повреждённый файл")
	}
	songs := []Song{}
	for i := uint32(0); i < songCount; i++ {
		song, err := r.song()
		if err != nil {
			return nil, err
		}
		songs = append(songs, song)
	}
	groups := []Group{}
	if r.o < len(body) {
		groupCount, err := r.u32()
		if err != nil {
			return nil, err
		}
		if groupCount > maxCount {
			return nil, fmt.Errorf("повреждённый файл")
		}
		for i := uint32(0); i < groupCount; i++ {
			group, err := r.group()
			if err != nil {
				return nil, err
			}
			groups = append(groups, group)
		}
	}
	return &Book{
		Title:        title,
		ShortName:    shortName,
		Publisher:    publisher,
		RevisionDate: revisionDate,
		Comment:      comment,
		Songs:        songs,
		Groups:       groups,
		BytesLeft:    len(body) - r.o,
	}, nil
}

func (r *reader) song() (Song, error) {
	var song Song
	var err error
	song.Title, err = r.str()
	if err != nil {
		return Song{}, err
	}
	song.AltTitle, err = r.str()
	if err != nil {
		return Song{}, err
	}
	song.Author, err = r.str()
	if err != nil {
		return Song{}, err
	}
	song.Composer, err = r.str()
	if err != nil {
		return Song{}, err
	}
	song.Note, err = r.str()
	if err != nil {
		return Song{}, err
	}
	song.Properties, err = r.str()
	if err != nil {
		return Song{}, err
	}
	partCount, err := r.u32()
	if err != nil {
		return Song{}, err
	}
	if partCount > maxCount {
		return Song{}, fmt.Errorf("повреждённый файл")
	}
	song.Parts = []Part{}
	for i := uint32(0); i < partCount; i++ {
		part, err := r.part()
		if err != nil {
			return Song{}, err
		}
		song.Parts = append(song.Parts, part)
	}
	return song, nil
}

func (r *reader) part() (Part, error) {
	kind, err := r.str()
	if err != nil {
		return Part{}, err
	}
	text, err := r.str()
	if err != nil {
		return Part{}, err
	}
	align, err := r.u32()
	if err != nil {
		return Part{}, err
	}
	return Part{Kind: kind, Text: text, Align: align}, nil
}

func (r *reader) group() (Group, error) {
	name, err := r.str()
	if err != nil {
		return Group{}, err
	}
	n, err := r.u32()
	if err != nil {
		return Group{}, err
	}
	if n > maxCount {
		return Group{}, fmt.Errorf("повреждённый файл")
	}
	songs := []uint32{}
	for i := uint32(0); i < n; i++ {
		idx, err := r.u32()
		if err != nil {
			return Group{}, err
		}
		songs = append(songs, idx)
	}
	return Group{Name: name, Songs: songs}, nil
}

func (r *reader) u32() (uint32, error) {
	if r.o+4 > len(r.b) {
		return 0, fmt.Errorf("повреждённый файл")
	}
	v := binary.LittleEndian.Uint32(r.b[r.o:])
	r.o += 4
	return v, nil
}

func (r *reader) u16() (uint16, error) {
	if r.o+2 > len(r.b) {
		return 0, fmt.Errorf("повреждённый файл")
	}
	v := binary.LittleEndian.Uint16(r.b[r.o:])
	r.o += 2
	return v, nil
}

func (r *reader) str() (string, error) {
	n, err := r.u16()
	if err != nil {
		return "", err
	}
	nbytes := int(n) * 2
	if r.o+nbytes > len(r.b) {
		return "", fmt.Errorf("повреждённый файл")
	}
	units := make([]uint16, n)
	for i := 0; i < int(n); i++ {
		units[i] = binary.LittleEndian.Uint16(r.b[r.o+2*i:])
	}
	r.o += nbytes
	return string(utf16.Decode(units)), nil
}

func Write(book *Book, template []byte) ([]byte, error) {
	if book == nil {
		return nil, fmt.Errorf("пустая книга")
	}
	if len(book.Groups) > 0 {
		return nil, fmt.Errorf("файл с группами не записывается")
	}
	if len(template) < headerFixed || string(template[:16]) != magic {
		return nil, fmt.Errorf("повреждённый шаблон")
	}
	csize := binary.LittleEndian.Uint32(template[20:])
	if int(csize) > len(template)-4 {
		return nil, fmt.Errorf("повреждённый шаблон")
	}
	start := len(template) - 4 - int(csize)
	head := reader{b: template, o: headerFixed}
	if _, err := head.str(); err != nil {
		return nil, fmt.Errorf("повреждённый шаблон")
	}
	if _, err := head.str(); err != nil {
		return nil, fmt.Errorf("повреждённый шаблон")
	}
	if start < head.o {
		return nil, fmt.Errorf("повреждённый шаблон")
	}
	extension := append([]byte(nil), template[head.o:start]...)
	body, err := encodeBody(book)
	if err != nil {
		return nil, err
	}
	if len(body) > maxUncompressed {
		return nil, fmt.Errorf("распаковка больше допустимого размера")
	}
	compressed, err := compress(body)
	if err != nil {
		return nil, err
	}
	header := append([]byte(nil), template[:headerFixed]...)
	binary.LittleEndian.PutUint32(header[20:], uint32(len(compressed)))
	binary.LittleEndian.PutUint32(header[24:], uint32(len(body)))
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
	out.Write(template[len(template)-4:])
	return out.Bytes(), nil
}

func encodeBody(book *Book) ([]byte, error) {
	var buf bytes.Buffer
	for _, value := range []string{book.Title, book.ShortName, book.Publisher, book.RevisionDate, book.Comment} {
		if err := writeText(&buf, value); err != nil {
			return nil, err
		}
	}
	if len(book.Songs) > maxCount {
		return nil, fmt.Errorf("слишком много песен")
	}
	writeU32(&buf, uint32(len(book.Songs)))
	for _, song := range book.Songs {
		for _, value := range []string{song.Title, song.AltTitle, song.Author, song.Composer, song.Note, song.Properties} {
			if err := writeText(&buf, value); err != nil {
				return nil, err
			}
		}
		if len(song.Parts) > maxCount {
			return nil, fmt.Errorf("слишком много частей")
		}
		writeU32(&buf, uint32(len(song.Parts)))
		for _, part := range song.Parts {
			if err := writeText(&buf, part.Kind); err != nil {
				return nil, err
			}
			if err := writeText(&buf, part.Text); err != nil {
				return nil, err
			}
			writeU32(&buf, part.Align)
		}
	}
	writeU32(&buf, 0)
	return buf.Bytes(), nil
}

func compress(body []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, zlib.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(body); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeU32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

func writeText(buf *bytes.Buffer, value string) error {
	units := utf16.Encode([]rune(value))
	if len(units) > 65535 {
		return fmt.Errorf("строка длиннее 65535")
	}
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
