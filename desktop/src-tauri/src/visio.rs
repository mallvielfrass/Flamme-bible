use std::collections::BTreeMap;
use std::fs;
use std::io::{Read, Write};
use std::path::{Path, PathBuf};

use flate2::read::ZlibDecoder;
use flate2::write::ZlibEncoder;
use flate2::Compression;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

use crate::plans::{self, PlanItem};
use crate::text;

const MAGIC: &[u8] = b"VisioBibleModule";
const HEADER_FIXED: usize = 72;
const MAX_FILE: usize = 32 << 20;
const MAX_UNCOMPRESSED: usize = 64 << 20;
const MAX_COUNT: u32 = 1 << 20;

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
pub struct Header {
    #[serde(default)]
    pub format_version: String,
    #[serde(default)]
    pub compressed: u32,
    #[serde(default)]
    pub uncompressed: u32,
    #[serde(default)]
    pub flags: u32,
    #[serde(default)]
    pub charset: String,
    #[serde(default)]
    pub header_short: String,
    #[serde(default)]
    pub header_title: String,
    #[serde(skip)]
    pub zlib_offset: usize,
}

impl Default for Header {
    fn default() -> Self {
        Self {
            format_version: "0x102".into(),
            compressed: 0,
            uncompressed: 0,
            flags: 0,
            charset: "0x4b0".into(),
            header_short: String::new(),
            header_title: String::new(),
            zlib_offset: 0,
        }
    }
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Part {
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub text: String,
    #[serde(default)]
    pub align: u32,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Song {
    #[serde(default)]
    pub fbid: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub alt_title: String,
    #[serde(default)]
    pub author: String,
    #[serde(default)]
    pub composer: String,
    #[serde(default)]
    pub note: String,
    #[serde(default)]
    pub properties: String,
    #[serde(default)]
    pub parts: Vec<Part>,
    #[serde(default)]
    pub deleted: bool,
}

#[derive(Clone, Debug, PartialEq)]
pub struct Group {
    pub name: String,
    pub songs: Vec<u32>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct Book {
    pub header: Header,
    pub title: String,
    pub short_name: String,
    pub publisher: String,
    pub revision_date: String,
    pub comment: String,
    pub songs: Vec<Song>,
    pub groups: Vec<Group>,
    pub bytes_left: usize,
}

#[derive(Clone, Debug, Serialize)]
pub struct SongbookInfo {
    pub path: String,
    pub title: String,
    pub short_name: String,
    pub song_count: usize,
    pub titles: Vec<String>,
    pub error: String,
}

#[derive(Clone, Debug, Serialize)]
pub struct BibleBookInfo {
    pub short_name: String,
    pub full_name: String,
}

#[derive(Clone, Debug, Serialize)]
pub struct BibleInfo {
    pub path: String,
    pub name: String,
    pub short_name: String,
    pub books: Vec<BibleBookInfo>,
}

#[derive(Clone, Debug, Serialize)]
pub struct PlanInfo {
    pub path: String,
    pub name: String,
    pub kind: String,
    pub items: Vec<PlanItem>,
}

#[derive(Clone, Debug, Serialize)]
pub struct Scan {
    pub root: String,
    pub modules_dir: String,
    pub plans_dir: String,
    pub needs_choice: bool,
    pub catalogs: Vec<String>,
    pub module_refs: Vec<String>,
    pub songbooks: Vec<SongbookInfo>,
    pub bibles: Vec<BibleInfo>,
    pub plans: Vec<PlanInfo>,
    pub plan_def: PlanDefView,
}

#[derive(Clone, Debug, Default, Serialize)]
pub struct PlanDefView {
    pub name: String,
    pub module_short_name: String,
    pub module_short_name_second: String,
    pub journals: Vec<String>,
}

struct Reader<'a> {
    b: &'a [u8],
    o: usize,
}

impl<'a> Reader<'a> {
    fn u32(&mut self) -> Result<u32, String> {
        let end = self.o.checked_add(4).ok_or("повреждённый файл")?;
        if end > self.b.len() {
            return Err("повреждённый файл".into());
        }
        let value = u32::from_le_bytes(self.b[self.o..end].try_into().unwrap());
        self.o = end;
        Ok(value)
    }

    fn u16(&mut self) -> Result<u16, String> {
        let end = self.o.checked_add(2).ok_or("повреждённый файл")?;
        if end > self.b.len() {
            return Err("повреждённый файл".into());
        }
        let value = u16::from_le_bytes(self.b[self.o..end].try_into().unwrap());
        self.o = end;
        Ok(value)
    }

    fn str(&mut self) -> Result<String, String> {
        let n = self.u16()? as usize;
        let nbytes = n.checked_mul(2).ok_or("повреждённый файл")?;
        let end = self.o.checked_add(nbytes).ok_or("повреждённый файл")?;
        if end > self.b.len() {
            return Err("повреждённый файл".into());
        }
        let mut units = Vec::with_capacity(n);
        for index in 0..n {
            let at = self.o + index * 2;
            units.push(u16::from_le_bytes([self.b[at], self.b[at + 1]]));
        }
        self.o = end;
        Ok(String::from_utf16_lossy(&units))
    }
}

fn write_u32(buf: &mut Vec<u8>, value: u32) {
    buf.extend_from_slice(&value.to_le_bytes());
}

fn write_text(buf: &mut Vec<u8>, value: &str) -> Result<(), String> {
    let units: Vec<u16> = value.encode_utf16().collect();
    if units.len() > u16::MAX as usize {
        return Err("строка длиннее 65535".into());
    }
    buf.extend_from_slice(&(units.len() as u16).to_le_bytes());
    for unit in units {
        buf.extend_from_slice(&unit.to_le_bytes());
    }
    Ok(())
}

fn decompress(payload: &[u8], expected: usize) -> Result<Vec<u8>, String> {
    if expected > MAX_UNCOMPRESSED {
        return Err("распаковка больше допустимого размера".into());
    }
    let mut decoder = ZlibDecoder::new(payload);
    let mut body = vec![0u8; expected + 1];
    let mut filled = 0;
    while filled < body.len() {
        match decoder.read(&mut body[filled..]) {
            Ok(0) => break,
            Ok(n) => filled += n,
            Err(_) => return Err("повреждённый файл".into()),
        }
    }
    if filled != expected {
        return Err("неверный размер".into());
    }
    body.truncate(expected);
    Ok(body)
}

fn compress(body: &[u8]) -> Result<Vec<u8>, String> {
    let mut encoder = ZlibEncoder::new(Vec::new(), Compression::default());
    encoder
        .write_all(body)
        .map_err(|_| "не записан".to_string())?;
    encoder.finish().map_err(|_| "не записан".to_string())
}

pub fn parse(data: &[u8]) -> Result<Book, String> {
    if data.len() > MAX_FILE {
        return Err("файл больше допустимого размера".into());
    }
    if data.len() < HEADER_FIXED || &data[..16] != MAGIC {
        return Err("повреждённый файл".into());
    }
    let version = u32::from_le_bytes(data[16..20].try_into().unwrap());
    let csize = u32::from_le_bytes(data[20..24].try_into().unwrap()) as usize;
    let usize_raw = u32::from_le_bytes(data[24..28].try_into().unwrap());
    let flags = u32::from_le_bytes(data[28..32].try_into().unwrap());
    let charset = u32::from_le_bytes(data[32..36].try_into().unwrap());
    if csize > data.len().saturating_sub(4) {
        return Err("неверный размер".into());
    }
    let start = data.len() - 4 - csize;
    let mut head = Reader {
        b: data,
        o: HEADER_FIXED,
    };
    let header_short = head.str()?;
    let header_title = head.str()?;
    if start < head.o {
        return Err("неверный размер".into());
    }
    if usize_raw as usize > MAX_UNCOMPRESSED {
        return Err("распаковка больше допустимого размера".into());
    }
    let body = decompress(&data[start..data.len() - 4], usize_raw as usize)?;
    let mut book = parse_body(&body)?;
    book.header = Header {
        format_version: format!("0x{version:x}"),
        compressed: csize as u32,
        uncompressed: usize_raw,
        flags,
        charset: format!("0x{charset:x}"),
        header_short,
        header_title,
        zlib_offset: start,
    };
    Ok(book)
}

fn parse_body(body: &[u8]) -> Result<Book, String> {
    let mut reader = Reader { b: body, o: 0 };
    let title = reader.str()?;
    let short_name = reader.str()?;
    let publisher = reader.str()?;
    let revision_date = reader.str()?;
    let comment = reader.str()?;
    let song_count = reader.u32()?;
    if song_count > MAX_COUNT {
        return Err("повреждённый файл".into());
    }
    let mut songs = Vec::with_capacity(song_count as usize);
    for _ in 0..song_count {
        songs.push(reader.song()?);
    }
    let mut groups = Vec::new();
    if reader.o < body.len() {
        let group_count = reader.u32()?;
        if group_count > MAX_COUNT {
            return Err("повреждённый файл".into());
        }
        for _ in 0..group_count {
            groups.push(reader.group()?);
        }
    }
    Ok(Book {
        header: Header::default(),
        title,
        short_name,
        publisher,
        revision_date,
        comment,
        songs,
        groups,
        bytes_left: body.len() - reader.o,
    })
}

impl<'a> Reader<'a> {
    fn song(&mut self) -> Result<Song, String> {
        let title = self.str()?;
        let alt_title = self.str()?;
        let author = self.str()?;
        let composer = self.str()?;
        let note = self.str()?;
        let properties = self.str()?;
        let part_count = self.u32()?;
        if part_count > MAX_COUNT {
            return Err("повреждённый файл".into());
        }
        let mut parts = Vec::with_capacity(part_count as usize);
        for _ in 0..part_count {
            parts.push(self.part()?);
        }
        let fbid = property_fbid(&properties)?.unwrap_or_default();
        Ok(Song {
            fbid,
            title,
            alt_title,
            author,
            composer,
            note,
            properties,
            parts,
            deleted: false,
        })
    }

    fn part(&mut self) -> Result<Part, String> {
        Ok(Part {
            kind: self.str()?,
            text: self.str()?,
            align: self.u32()?,
        })
    }

    fn group(&mut self) -> Result<Group, String> {
        let name = self.str()?;
        let count = self.u32()?;
        if count > MAX_COUNT {
            return Err("повреждённый файл".into());
        }
        let mut songs = Vec::with_capacity(count as usize);
        for _ in 0..count {
            songs.push(self.u32()?);
        }
        Ok(Group { name, songs })
    }
}

pub fn parse_file(path: &Path) -> Result<Book, String> {
    let data = fs::read(path).map_err(|err| format!("не прочитан {}: {err}", path.display()))?;
    parse(&data)
}

/// A grouped songbook is refused: rewriting it would drop the groups.
pub fn write(book: &Book, template: &[u8]) -> Result<Vec<u8>, String> {
    if !book.groups.is_empty() {
        return Err("сборник с группами не перезаписывается".into());
    }
    if template.len() < HEADER_FIXED || &template[..16] != MAGIC {
        return Err("повреждённый шаблон".into());
    }
    let csize = u32::from_le_bytes(template[20..24].try_into().unwrap()) as usize;
    if csize > template.len().saturating_sub(4) {
        return Err("повреждённый шаблон".into());
    }
    let start = template.len() - 4 - csize;
    let mut head = Reader {
        b: template,
        o: HEADER_FIXED,
    };
    head.str().map_err(|_| "повреждённый шаблон".to_string())?;
    head.str().map_err(|_| "повреждённый шаблон".to_string())?;
    if start < head.o {
        return Err("повреждённый шаблон".into());
    }
    let extension = &template[head.o..start];
    let body = encode_body(book)?;
    if body.len() > MAX_UNCOMPRESSED {
        return Err("распаковка больше допустимого размера".into());
    }
    let compressed = compress(&body)?;
    let mut header = template[..HEADER_FIXED].to_vec();
    header[20..24].copy_from_slice(&(compressed.len() as u32).to_le_bytes());
    header[24..28].copy_from_slice(&(body.len() as u32).to_le_bytes());
    let mut out = Vec::new();
    out.extend_from_slice(&header);
    write_text(&mut out, &book.header.header_short)?;
    write_text(&mut out, &book.header.header_title)?;
    out.extend_from_slice(extension);
    out.extend_from_slice(&compressed);
    out.extend_from_slice(&template[template.len() - 4..]);
    Ok(out)
}

fn encode_body(book: &Book) -> Result<Vec<u8>, String> {
    let mut buf = Vec::new();
    for value in [
        &book.title,
        &book.short_name,
        &book.publisher,
        &book.revision_date,
        &book.comment,
    ] {
        write_text(&mut buf, value)?;
    }
    if book.songs.len() > MAX_COUNT as usize {
        return Err("слишком много песен".into());
    }
    write_u32(&mut buf, book.songs.len() as u32);
    for song in &book.songs {
        for value in [
            &song.title,
            &song.alt_title,
            &song.author,
            &song.composer,
            &song.note,
            &song.properties,
        ] {
            write_text(&mut buf, value)?;
        }
        if song.parts.len() > MAX_COUNT as usize {
            return Err("слишком много частей".into());
        }
        write_u32(&mut buf, song.parts.len() as u32);
        for part in &song.parts {
            write_text(&mut buf, &part.kind)?;
            write_text(&mut buf, &part.text)?;
            write_u32(&mut buf, part.align);
        }
    }
    write_u32(&mut buf, 0);
    Ok(buf)
}

pub fn blank_template(header_short: &str, header_title: &str) -> Result<Vec<u8>, String> {
    let mut header = vec![0u8; HEADER_FIXED];
    header[..16].copy_from_slice(MAGIC);
    header[16..20].copy_from_slice(&0x0102u32.to_le_bytes());
    header[32..36].copy_from_slice(&0x04b0u32.to_le_bytes());
    write_text(&mut header, header_short)?;
    write_text(&mut header, header_title)?;
    let placeholder = [0u8; 8];
    let csize = placeholder.len() as u32;
    header[20..24].copy_from_slice(&csize.to_le_bytes());
    header.extend_from_slice(&placeholder);
    header.extend_from_slice(&[0, 0, 0, 0]);
    Ok(header)
}

pub fn backup_path(path: &Path) -> PathBuf {
    let mut name = path.file_name().unwrap_or_default().to_os_string();
    name.push(".bak");
    path.with_file_name(name)
}

/// Copy the current file to `name.bak`, then replace it. The copy stays.
pub fn replace_file(path: &Path, bytes: &[u8]) -> Result<(), String> {
    if let Some(parent) = path.parent() {
        if !parent.as_os_str().is_empty() {
            fs::create_dir_all(parent).map_err(|err| err.to_string())?;
        }
    }
    let existed = path.is_file();
    if existed {
        fs::copy(path, backup_path(path))
            .map_err(|err| format!("не сохранена копия .bak: {err}"))?;
    }
    let file_name = path.file_name().ok_or_else(|| "путь".to_string())?;
    let mut tmp_name = std::ffi::OsString::from(".");
    tmp_name.push(file_name);
    tmp_name.push(".tmp");
    let tmp = path.with_file_name(tmp_name);
    if let Err(err) = fs::write(&tmp, bytes) {
        let _ = fs::remove_file(&tmp);
        return Err(err.to_string());
    }
    if existed {
        fs::remove_file(path).map_err(|err| err.to_string())?;
    }
    if let Err(err) = fs::rename(&tmp, path) {
        if existed && !path.exists() {
            let _ = fs::copy(backup_path(path), path);
        }
        let _ = fs::remove_file(&tmp);
        return Err(err.to_string());
    }
    Ok(())
}

pub fn write_book_file(path: &Path, book: &Book) -> Result<(), String> {
    if !book.groups.is_empty() {
        return Err("сборник с группами не перезаписывается".into());
    }
    let template = if path.is_file() {
        let bytes = fs::read(path).map_err(|err| err.to_string())?;
        let existing = parse(&bytes)?;
        if !existing.groups.is_empty() {
            return Err("сборник с группами не перезаписывается".into());
        }
        bytes
    } else {
        blank_template(&book.header.header_short, &book.header.header_title)?
    };
    let bytes = write(book, &template)?;
    replace_file(path, &bytes)
}

pub fn valid_uuid(value: &str) -> bool {
    let bytes = value.as_bytes();
    if bytes.len() != 36 {
        return false;
    }
    let groups = [8usize, 4, 4, 4, 12];
    let mut index = 0;
    for (group, len) in groups.iter().enumerate() {
        if group > 0 {
            if bytes.get(index) != Some(&b'-') {
                return false;
            }
            index += 1;
        }
        for _ in 0..*len {
            if !bytes
                .get(index)
                .is_some_and(|byte| byte.is_ascii_hexdigit())
            {
                return false;
            }
            index += 1;
        }
    }
    index == 36
}

pub fn property_fbid(properties: &str) -> Result<Option<String>, String> {
    let text = properties.replace("\r\n", "\n").replace('\r', "\n");
    let mut found = None;
    for line in text.split('\n') {
        let Some(value) = line.strip_prefix("$FBID$=") else {
            continue;
        };
        if found.is_some() {
            return Err("повтор песни".into());
        }
        if !valid_uuid(value) {
            return Err("повреждён номер песни".into());
        }
        found = Some(value.to_ascii_lowercase());
    }
    Ok(found)
}

pub fn set_fbid(properties: &str, fbid: &str) -> String {
    let newline = if properties.contains("\r\n") {
        "\r\n"
    } else {
        "\n"
    };
    let text = properties.replace("\r\n", "\n").replace('\r', "\n");
    let mut lines = Vec::new();
    let mut seen = false;
    for line in text.split('\n') {
        if line.starts_with("$FBID$=") {
            if !seen {
                lines.push(format!("$FBID$={fbid}"));
                seen = true;
            }
            continue;
        }
        lines.push(line.to_string());
    }
    if lines.last().is_some_and(|line| line.is_empty()) {
        lines.pop();
    }
    if !seen {
        lines.push(format!("$FBID$={fbid}"));
    }
    let mut out = lines.join(newline);
    out.push_str(newline);
    out
}

pub fn normalize_remote_song(song: &Song) -> Result<Song, String> {
    let mut song = song.clone();
    if song.fbid.is_empty() {
        song.fbid = property_fbid(&song.properties)?.unwrap_or_default();
    } else {
        song.fbid = song.fbid.to_ascii_lowercase();
    }
    if !valid_uuid(&song.fbid) {
        return Err("повреждён номер песни".into());
    }
    Ok(song)
}

fn pending_key(path: &str, index: usize, song: &Song) -> String {
    let mut hasher = Sha256::new();
    let line = format!(
        "{path}\0{index}\0{}\0{}\0{}\0{}\0{}\0{}",
        song.title, song.alt_title, song.author, song.composer, song.note, song.properties
    );
    hasher.update(line.as_bytes());
    for part in &song.parts {
        hasher.update(part.kind.as_bytes());
        hasher.update(part.text.as_bytes());
        hasher.update(part.align.to_le_bytes());
    }
    format!("{:x}", hasher.finalize())
}

/// Songs without a number get one stable id, remembered in `pending`.
pub fn prepare_local_songs(
    path: &str,
    songs: &[Song],
    pending: &mut BTreeMap<String, String>,
    mut new_id: impl FnMut() -> String,
) -> Result<Vec<Song>, String> {
    let mut seen = BTreeMap::<String, ()>::new();
    let mut out = Vec::with_capacity(songs.len());
    for (index, source) in songs.iter().enumerate() {
        let mut song = source.clone();
        let from_props = property_fbid(&song.properties)?;
        let id = if let Some(id) = from_props {
            if !song.fbid.is_empty() && song.fbid.to_ascii_lowercase() != id {
                return Err("повреждён номер песни".into());
            }
            id
        } else if !song.fbid.is_empty() {
            let id = song.fbid.to_ascii_lowercase();
            if !valid_uuid(&id) {
                return Err("повреждён номер песни".into());
            }
            id
        } else {
            let key = pending_key(path, index, &song);
            if let Some(id) = pending.get(&key) {
                id.clone()
            } else {
                let id = new_id().to_ascii_lowercase();
                if !valid_uuid(&id) {
                    return Err("повреждён номер песни".into());
                }
                pending.insert(key, id.clone());
                id
            }
        };
        if seen.insert(id.clone(), ()).is_some() {
            return Err("повтор песни".into());
        }
        song.fbid = id.clone();
        if property_fbid(&song.properties)?.as_deref() != Some(id.as_str()) {
            song.properties = set_fbid(&song.properties, &id);
        }
        out.push(song);
    }
    Ok(out)
}

fn normalize_newlines(text: &str) -> String {
    text.replace("\r\n", "\n").replace('\r', "\n")
}

/// Line endings and a trailing newline are not a real edit. Spaces and align are.
fn content_properties(properties: &str) -> String {
    normalize_newlines(properties)
        .lines()
        .filter(|line| !line.starts_with("$ID$=") && !line.starts_with("$FBID$="))
        .collect::<Vec<_>>()
        .join("\n")
}

fn normalize_part(text: &str) -> String {
    normalize_newlines(text).trim_end_matches('\n').to_string()
}

pub fn song_eq(left: &Song, right: &Song) -> bool {
    left.title == right.title
        && left.alt_title == right.alt_title
        && left.author == right.author
        && left.composer == right.composer
        && left.note == right.note
        && content_properties(&left.properties) == content_properties(&right.properties)
        && left.parts.len() == right.parts.len()
        && left.parts.iter().zip(&right.parts).all(|(a, b)| {
            a.kind == b.kind
                && a.align == b.align
                && normalize_part(&a.text) == normalize_part(&b.text)
        })
}

pub fn same_on_disk(left: &[Song], right: &[Song]) -> bool {
    left.len() == right.len()
        && left.iter().zip(right).all(|(a, b)| {
            a.title == b.title
                && a.alt_title == b.alt_title
                && a.author == b.author
                && a.composer == b.composer
                && a.note == b.note
                && a.properties == b.properties
                && a.parts == b.parts
        })
}

pub fn lyrics(song: &Song) -> String {
    song.parts
        .iter()
        .map(|part| part.text.as_str())
        .filter(|text| !text.is_empty())
        .collect::<Vec<_>>()
        .join("\n")
}

/// The dialog edits the whole lyric as one text, not as separate verses.
pub fn song_from_manual(base: &Song, title: &str, lyrics_text: &str) -> Song {
    let mut song = base.clone();
    song.title = title.to_string();
    song.deleted = false;
    song.parts = vec![Part {
        kind: "Текст".into(),
        text: lyrics_text.to_string(),
        align: 0,
    }];
    song
}

pub fn with_fbid(song: &Song, fbid: &str) -> Song {
    let mut song = song.clone();
    song.fbid = fbid.to_string();
    song.deleted = false;
    song.properties = set_fbid(&song.properties, fbid);
    song
}

pub fn open_location(input: &Path) -> Result<Scan, String> {
    if !input.exists() {
        return Err("Папка не найдена. Укажите каталог с VisioBible.ini или папку Modules.".into());
    }
    let input = if input.is_file() {
        if !name_is(input, "VisioBible.ini") {
            return Err("Выберите папку VisioBible, файл VisioBible.ini или папку Modules.".into());
        }
        input
            .parent()
            .ok_or_else(|| "Папка не найдена".to_string())?
            .to_path_buf()
    } else {
        input.to_path_buf()
    };
    if child_modules(&input).is_some() || has_install_marker(&input) {
        return open_root(&input);
    }
    if is_modules_catalog(&input) {
        let root = install_root_of(&input);
        let refs = read_refs(&root);
        return scan_one(&root, &input, refs);
    }
    let mut catalogs = Vec::new();
    if let Ok(entries) = fs::read_dir(&input) {
        let mut entries: Vec<_> = entries.filter_map(|entry| entry.ok()).collect();
        entries.sort_by_key(|entry| entry.file_name());
        for entry in entries {
            let path = entry.path();
            if !path.is_dir() {
                continue;
            }
            if let Some(catalog) = primary_catalog(&path) {
                push_unique(&mut catalogs, catalog);
            }
        }
    }
    if catalogs.is_empty() {
        return Err("Папка VisioBible не найдена. Обычно VisioBible.ini лежит рядом с программой, а сборники — в подпапке Modules.".into());
    }
    if catalogs.len() > 1 {
        return Ok(choice_scan(catalogs, Vec::new()));
    }
    open_location(&catalogs[0])
}

fn open_root(root: &Path) -> Result<Scan, String> {
    let refs = read_refs(root);
    let mut catalogs = Vec::new();
    if let Some(primary) = primary_catalog(root) {
        push_unique(&mut catalogs, primary);
    }
    for reference in &refs {
        if let Some(catalog) = catalog_of(Path::new(reference)) {
            push_unique(&mut catalogs, catalog);
        }
    }
    if catalogs.is_empty() {
        return Err("В этой папке нет каталога Modules. Его можно выбрать отдельно.".into());
    }
    if catalogs.len() > 1 {
        return Ok(choice_scan(catalogs, refs));
    }
    scan_one(root, &catalogs[0], refs)
}

fn choice_scan(catalogs: Vec<PathBuf>, refs: Vec<String>) -> Scan {
    Scan {
        root: String::new(),
        modules_dir: String::new(),
        plans_dir: String::new(),
        needs_choice: true,
        catalogs: catalogs
            .iter()
            .map(|path| path.display().to_string())
            .collect(),
        module_refs: refs,
        songbooks: Vec::new(),
        bibles: Vec::new(),
        plans: Vec::new(),
        plan_def: PlanDefView::default(),
    }
}

fn scan_one(root: &Path, modules: &Path, refs: Vec<String>) -> Result<Scan, String> {
    let plans_dir = plans_directory(root, modules);
    let mut songbooks = Vec::new();
    let mut names = Vec::new();
    if let Ok(entries) = fs::read_dir(modules) {
        for entry in entries.filter_map(|entry| entry.ok()) {
            if entry
                .file_type()
                .map(|kind| kind.is_file())
                .unwrap_or(false)
                && entry
                    .path()
                    .extension()
                    .and_then(|ext| ext.to_str())
                    .is_some_and(|ext| ext.eq_ignore_ascii_case("vbm"))
            {
                names.push(entry.path());
            }
        }
    }
    names.sort();
    for path in names {
        songbooks.push(read_songbook_info(&path));
    }
    let bibles = read_bibles(modules);
    let (plans, plan_def) = read_plans(&plans_dir);
    Ok(Scan {
        root: display_path(root),
        modules_dir: modules.display().to_string(),
        plans_dir: plans_dir.display().to_string(),
        needs_choice: false,
        catalogs: vec![modules.display().to_string()],
        module_refs: refs,
        songbooks,
        bibles,
        plans,
        plan_def,
    })
}

fn display_path(path: &Path) -> String {
    if path.as_os_str().is_empty() {
        String::new()
    } else {
        path.display().to_string()
    }
}

fn read_songbook_info(path: &Path) -> SongbookInfo {
    match parse_file(path) {
        Ok(book) => SongbookInfo {
            path: path.display().to_string(),
            title: book.title.clone(),
            short_name: book.short_name.clone(),
            song_count: book.songs.len(),
            titles: book.songs.into_iter().map(|song| song.title).collect(),
            error: String::new(),
        },
        Err(err) => SongbookInfo {
            path: path.display().to_string(),
            title: path
                .file_stem()
                .and_then(|name| name.to_str())
                .unwrap_or("сборник")
                .to_string(),
            short_name: String::new(),
            song_count: 0,
            titles: Vec::new(),
            error: err,
        },
    }
}

fn read_bibles(modules: &Path) -> Vec<BibleInfo> {
    let mut found = Vec::new();
    let direct = modules.join("bibleqt.ini");
    if direct.is_file() {
        if let Some(info) = read_bible(&direct) {
            found.push(info);
        }
    }
    let Ok(entries) = fs::read_dir(modules) else {
        return found;
    };
    let mut dirs: Vec<_> = entries
        .filter_map(|entry| entry.ok())
        .map(|entry| entry.path())
        .collect();
    dirs.sort();
    for dir in dirs {
        if !dir.is_dir() {
            continue;
        }
        let ini = dir.join("bibleqt.ini");
        if ini.is_file() {
            if let Some(info) = read_bible(&ini) {
                found.push(info);
            }
        }
    }
    found
}

fn read_bible(path: &Path) -> Option<BibleInfo> {
    let bytes = fs::read(path).ok()?;
    let text = text::decode_text(&bytes);
    let index = parse_bible_ini(&text);
    Some(BibleInfo {
        path: path.parent().unwrap_or(path).display().to_string(),
        name: index.name,
        short_name: index.short_name,
        books: index.books,
    })
}

struct BibleIndex {
    name: String,
    short_name: String,
    books: Vec<BibleBookInfo>,
}

/// BookHash and verse HTML stay on disk. A plan only needs the module name.
fn parse_bible_ini(text: &str) -> BibleIndex {
    let mut index = BibleIndex {
        name: String::new(),
        short_name: String::new(),
        books: Vec::new(),
    };
    let mut book: Option<BibleBookInfo> = None;
    let flush = |book: &mut Option<BibleBookInfo>, index: &mut BibleIndex| {
        if let Some(book) = book.take() {
            index.books.push(book);
        }
    };
    for line in text.split('\n') {
        let line = line.trim().trim_end_matches('\r');
        if line.is_empty() || line.starts_with(';') {
            continue;
        }
        let Some((key, value)) = line.split_once('=') else {
            continue;
        };
        let key = key.trim();
        let value = value.trim();
        if key.eq_ignore_ascii_case("PathName") {
            flush(&mut book, &mut index);
            book = Some(BibleBookInfo {
                short_name: String::new(),
                full_name: String::new(),
            });
            continue;
        }
        if book.is_none() {
            if key.eq_ignore_ascii_case("BibleName") {
                index.name = value.to_string();
            } else if key.eq_ignore_ascii_case("BibleShortName") {
                index.short_name = value.to_string();
            }
            continue;
        }
        let Some(current) = book.as_mut() else {
            continue;
        };
        if key.eq_ignore_ascii_case("FullName") {
            current.full_name = value.to_string();
        } else if key.eq_ignore_ascii_case("ShortName") {
            current.short_name = value.to_string();
        }
    }
    flush(&mut book, &mut index);
    index
}

fn read_plans(dir: &Path) -> (Vec<PlanInfo>, PlanDefView) {
    let mut plans = Vec::new();
    let mut def = PlanDefView::default();
    if !dir.is_dir() {
        return (plans, def);
    }
    let plan_def_path = dir.join("PlanDef.ini");
    if plan_def_path.is_file() {
        if let Ok(parsed) = plans::read_plan_def_file(&plan_def_path) {
            def = PlanDefView {
                name: parsed.name.clone(),
                module_short_name: parsed.module_short_name,
                module_short_name_second: parsed.module_short_name_second,
                journals: parsed.journals,
            };
            if !parsed.items.is_empty() {
                plans.push(PlanInfo {
                    path: plan_def_path.display().to_string(),
                    name: if parsed.name.is_empty() {
                        "PlanDef".into()
                    } else {
                        parsed.name
                    },
                    kind: "plandef".into(),
                    items: parsed.items,
                });
            }
        }
    }
    let history = dir.join("History.ini");
    if history.is_file() {
        if let Ok(items) = plans::read_history_file(&history) {
            plans.push(PlanInfo {
                path: history.display().to_string(),
                name: "История".into(),
                kind: "history".into(),
                items,
            });
        }
    }
    if let Ok(entries) = fs::read_dir(dir) {
        let mut files: Vec<_> = entries
            .filter_map(|entry| entry.ok())
            .map(|entry| entry.path())
            .collect();
        files.sort();
        for path in files {
            if path
                .extension()
                .and_then(|ext| ext.to_str())
                .is_some_and(|ext| ext.eq_ignore_ascii_case("vplt"))
            {
                let name = path
                    .file_stem()
                    .and_then(|stem| stem.to_str())
                    .unwrap_or("план")
                    .to_string();
                let items = plans::read_vplt_file(&path).unwrap_or_default();
                plans.push(PlanInfo {
                    path: path.display().to_string(),
                    name,
                    kind: "vplt".into(),
                    items,
                });
            }
        }
    }
    (plans, def)
}

fn install_root_of(modules: &Path) -> PathBuf {
    let mut current = modules.to_path_buf();
    for _ in 0..4 {
        let Some(parent) = current.parent() else {
            break;
        };
        if parent == current {
            break;
        }
        if has_install_marker(parent) || parent.join("Plans").is_dir() {
            return parent.to_path_buf();
        }
        current = parent.to_path_buf();
    }
    modules.parent().unwrap_or(modules).to_path_buf()
}

fn plans_directory(root: &Path, modules: &Path) -> PathBuf {
    let parent = modules.parent().unwrap_or(modules);
    let candidates = [
        root.join("Plans"),
        parent.join("Plans"),
        modules.join("Plans"),
    ];
    for candidate in &candidates {
        if candidate.is_dir() {
            return candidate.clone();
        }
    }
    if root.as_os_str().is_empty() {
        parent.join("Plans")
    } else {
        root.join("Plans")
    }
}

fn read_refs(root: &Path) -> Vec<String> {
    let Some(path) = find_named(root, "VisioBible.ini") else {
        return Vec::new();
    };
    let Ok(bytes) = fs::read(&path) else {
        return Vec::new();
    };
    module_refs(root, &text::decode_text(&bytes))
}

pub fn module_refs(root: &Path, text_value: &str) -> Vec<String> {
    let ini = text::parse_ini(text_value);
    let mut refs = Vec::new();
    for section in &ini.sections {
        let bible = section.name.eq_ignore_ascii_case("BiblePath");
        let settings = section.name.eq_ignore_ascii_case("settings");
        for (key, value) in &section.lines {
            let raw = text::quoted_path(value).unwrap_or(value.as_str());
            let mentioned = bible
                || value.contains('"')
                || (settings && (is_path_key(key) || raw.contains('/') || raw.contains('\\')));
            let take = mentioned && looks_like_path(raw);
            if take {
                refs.push(resolve_ref(root, raw));
            }
        }
    }
    refs
}

fn looks_like_path(value: &str) -> bool {
    !value.is_empty() && !value.chars().all(|ch| ch.is_ascii_digit())
}

fn is_path_key(key: &str) -> bool {
    let key = key.to_ascii_lowercase();
    key.contains("path") || key.contains("module") || key.contains("dir") || key.contains("folder")
}

fn resolve_ref(root: &Path, raw: &str) -> String {
    let slash = raw.trim().replace('\\', "/");
    let mut candidates = vec![PathBuf::from(&slash)];
    if let Some(rest) = slash
        .strip_prefix(|c: char| c.is_ascii_alphabetic())
        .and_then(|rest| rest.strip_prefix(':'))
    {
        if rest.starts_with('/') {
            candidates.push(PathBuf::from(rest));
        }
    }
    if !Path::new(&slash).is_absolute() {
        candidates.push(root.join(&slash));
    }
    for candidate in &candidates {
        if candidate.exists() {
            return candidate.display().to_string();
        }
    }
    candidates
        .first()
        .map(|path| path.display().to_string())
        .unwrap_or_default()
}

fn catalog_of(path: &Path) -> Option<PathBuf> {
    if !path.exists() {
        return None;
    }
    let original = if path.is_file() {
        path.parent()?.to_path_buf()
    } else {
        path.to_path_buf()
    };
    let mut current = original.clone();
    loop {
        if name_is(&current, "Modules") {
            return Some(current);
        }
        if !current.pop() {
            break;
        }
    }
    Some(original)
}

fn primary_catalog(path: &Path) -> Option<PathBuf> {
    if let Some(modules) = child_modules(path) {
        return Some(modules);
    }
    if is_modules_catalog(path) {
        return Some(path.to_path_buf());
    }
    None
}

fn child_modules(dir: &Path) -> Option<PathBuf> {
    let entries = fs::read_dir(dir).ok()?;
    for entry in entries.filter_map(|entry| entry.ok()) {
        if entry.file_type().map(|kind| kind.is_dir()).unwrap_or(false)
            && name_is(&entry.path(), "Modules")
        {
            return Some(entry.path());
        }
    }
    None
}

fn has_install_marker(dir: &Path) -> bool {
    find_named(dir, "VisioBible.ini").is_some() || find_named(dir, "VisioBible.exe").is_some()
}

fn is_modules_catalog(dir: &Path) -> bool {
    if name_is(dir, "Modules") {
        return true;
    }
    let Ok(entries) = fs::read_dir(dir) else {
        return false;
    };
    for entry in entries.filter_map(|entry| entry.ok()) {
        let path = entry.path();
        if path.is_file()
            && path
                .extension()
                .and_then(|ext| ext.to_str())
                .is_some_and(|ext| ext.eq_ignore_ascii_case("vbm"))
        {
            return true;
        }
        if path.is_dir() && path.join("bibleqt.ini").is_file() {
            return true;
        }
    }
    false
}

fn find_named(dir: &Path, expect: &str) -> Option<PathBuf> {
    let entries = fs::read_dir(dir).ok()?;
    for entry in entries.filter_map(|entry| entry.ok()) {
        if name_is(&entry.path(), expect) {
            return Some(entry.path());
        }
    }
    None
}

fn name_is(path: &Path, expect: &str) -> bool {
    path.file_name()
        .and_then(|name| name.to_str())
        .is_some_and(|name| name.eq_ignore_ascii_case(expect))
}

fn push_unique(list: &mut Vec<PathBuf>, path: PathBuf) {
    let key = path.canonicalize().unwrap_or(path.clone());
    if list
        .iter()
        .any(|item| item.canonicalize().unwrap_or_else(|_| item.clone()) == key)
    {
        return;
    }
    list.push(path);
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::paths::TmpDir;

    fn sample() -> Book {
        Book {
            header: Header {
                format_version: "0x102".into(),
                compressed: 0,
                uncompressed: 0,
                flags: 0,
                charset: "0x4b0".into(),
                header_short: "FB".into(),
                header_title: "FlammeBible Test".into(),
                zlib_offset: 0,
            },
            title: "Синтетический сборник".into(),
            short_name: "SYNTH".into(),
            publisher: "testdata".into(),
            revision_date: "2026-11-20".into(),
            comment: "synthetic fixture, not a third-party module".into(),
            songs: vec![
                Song {
                    fbid: String::new(),
                    title: "Тестовая песня".into(),
                    alt_title: String::new(),
                    author: String::new(),
                    composer: String::new(),
                    note: String::new(),
                    properties:
                        "$TUNE$=\r\n$ID$=1\r\n$FBID$=fba56871-f5d6-4b32-8b42-4bca03278ec1\r\n"
                            .into(),
                    parts: vec![Part {
                        kind: "Куплет".into(),
                        text: "тестовый куплет".into(),
                        align: 0,
                    }],
                    deleted: false,
                },
                Song {
                    fbid: String::new(),
                    title: "Вторая тестовая".into(),
                    alt_title: String::new(),
                    author: String::new(),
                    composer: String::new(),
                    note: String::new(),
                    properties: "$TUNE$=\r\n$ID$=2\r\n".into(),
                    parts: vec![Part {
                        kind: "Куплет".into(),
                        text: "ещё одна строка\nвторая".into(),
                        align: 0,
                    }],
                    deleted: false,
                },
            ],
            groups: Vec::new(),
            bytes_left: 0,
        }
    }

    fn pack(book: &Book, extension: &[u8], tail: &[u8]) -> Vec<u8> {
        let body = encode_body_groups(book);
        let compressed = compress(&body).unwrap();
        let mut header = vec![0u8; HEADER_FIXED];
        header[..16].copy_from_slice(MAGIC);
        header[16..20].copy_from_slice(&0x0102u32.to_le_bytes());
        header[20..24].copy_from_slice(&(compressed.len() as u32).to_le_bytes());
        header[24..28].copy_from_slice(&(body.len() as u32).to_le_bytes());
        header[28..32].copy_from_slice(&book.header.flags.to_le_bytes());
        header[32..36].copy_from_slice(&0x04b0u32.to_le_bytes());
        let mut out = header;
        write_text(&mut out, &book.header.header_short).unwrap();
        write_text(&mut out, &book.header.header_title).unwrap();
        out.extend_from_slice(extension);
        out.extend_from_slice(&compressed);
        out.extend_from_slice(tail);
        out
    }

    fn encode_body_groups(book: &Book) -> Vec<u8> {
        let mut buf = encode_body(&Book {
            groups: Vec::new(),
            ..book.clone()
        })
        .unwrap();
        buf.truncate(buf.len() - 4);
        write_u32(&mut buf, book.groups.len() as u32);
        for group in &book.groups {
            write_text(&mut buf, &group.name).unwrap();
            write_u32(&mut buf, group.songs.len() as u32);
            for index in &group.songs {
                write_u32(&mut buf, *index);
            }
        }
        buf
    }

    #[test]
    fn roundtrip_and_rejects_bad_files() {
        let raw = pack(
            &sample(),
            &[1, 2, 3, 4, 5, 6, 7, 8],
            &[0xDE, 0xAD, 0xBE, 0xEF],
        );
        let parsed = parse(&raw).unwrap();
        assert_eq!(parsed.title, "Синтетический сборник");
        assert_eq!(parsed.songs[0].fbid, "fba56871-f5d6-4b32-8b42-4bca03278ec1");
        assert_eq!(parsed.songs[1].parts[0].text, "ещё одна строка\nвторая");
        assert!(parse(&raw[..80]).is_err());
        let mut wrong = raw.clone();
        wrong[24] = 1;
        wrong[25] = 0;
        wrong[26] = 0;
        wrong[27] = 0;
        assert!(parse(&wrong).is_err());
        let mut broken = raw.clone();
        let mid = broken.len() / 2;
        broken[mid] ^= 0xff;
        assert!(parse(&broken).is_err());
    }

    #[test]
    fn write_keeps_unread_bytes_and_bak_keeps_previous() {
        let extension = [9u8, 8, 7, 6, 5, 4, 3, 2];
        let raw = pack(&sample(), &extension, &[0xDE, 0xAD, 0xBE, 0xEF]);
        let mut template = raw.clone();
        let parsed = parse(&template).unwrap();
        template[40] = 0xAB;
        template[parsed.header.zlib_offset - 1] = 0xCD;
        let out = write(&parsed, &template).unwrap();
        assert_eq!(out[40], 0xAB);
        assert_eq!(out[parsed.header.zlib_offset - 1], 0xCD);
        assert_eq!(&out[out.len() - 4..], &[0xDE, 0xAD, 0xBE, 0xEF]);
        let again = parse(&out).unwrap();
        assert!(song_eq(&parsed.songs[0], &again.songs[0]));
        assert_eq!(again.songs.len(), 2);

        let tmp = TmpDir::new("bak");
        let path = tmp.path.join("book.vbm");
        write_book_file(&path, &parsed).unwrap();
        let first = fs::read(&path).unwrap();
        let mut edited = parsed.clone();
        edited.songs[0].title = "другая".into();
        write_book_file(&path, &edited).unwrap();
        assert_eq!(fs::read(backup_path(&path)).unwrap(), first);
        assert_eq!(parse_file(&path).unwrap().songs[0].title, "другая");
    }

    #[test]
    fn grouped_file_is_not_replaced() {
        let mut book = sample();
        book.groups = vec![Group {
            name: "группа".into(),
            songs: vec![0],
        }];
        let raw = pack(&book, &[0, 0, 0, 0], &[0, 0, 0, 0]);
        let parsed = parse(&raw).unwrap();
        assert_eq!(parsed.groups.len(), 1);
        assert!(write(&parsed, &raw).is_err());
        let tmp = TmpDir::new("groups");
        let path = tmp.path.join("grouped.vbm");
        fs::write(&path, &raw).unwrap();
        assert!(write_book_file(&path, &parsed).is_err());
        assert_eq!(fs::read(&path).unwrap(), raw);
        assert!(!backup_path(&path).exists());
    }

    #[test]
    fn line_endings_do_not_count_as_an_edit() {
        let mut left = sample().songs.remove(0);
        let mut right = left.clone();
        right.parts[0].text = "тестовый куплет\r\n".into();
        assert!(song_eq(&left, &right));
        right.parts[0].text = "тестовый куплет ".into();
        assert!(!song_eq(&left, &right));
        left.parts[0].align = 1;
        right.parts[0].text = "тестовый куплет".into();
        assert!(!song_eq(&left, &right));
    }

    #[test]
    fn missing_song_number_stays_stable() {
        let song = Song {
            fbid: String::new(),
            title: "Без номера".into(),
            alt_title: String::new(),
            author: String::new(),
            composer: String::new(),
            note: String::new(),
            properties: String::new(),
            parts: vec![Part {
                kind: "Куплет".into(),
                text: "текст".into(),
                align: 0,
            }],
            deleted: false,
        };
        let mut pending = BTreeMap::new();
        let first = prepare_local_songs(
            "book.vbm",
            std::slice::from_ref(&song),
            &mut pending,
            || "11111111-1111-4111-8111-111111111111".into(),
        )
        .unwrap();
        let second = prepare_local_songs("book.vbm", &[song], &mut pending, || {
            "22222222-2222-4222-8222-222222222222".into()
        })
        .unwrap();
        assert_eq!(first[0].fbid, second[0].fbid);
        assert!(first[0]
            .properties
            .contains("$FBID$=11111111-1111-4111-8111-111111111111"));
    }

    #[test]
    fn discovers_ini_modules_and_direct_folder() {
        let tmp = TmpDir::new("scan");
        let root = tmp.path.join("VisioBible");
        let modules = root.join("library").join("Modules");
        let bible = modules.join("rst+");
        let plans = root.join("Plans");
        fs::create_dir_all(&bible).unwrap();
        fs::create_dir_all(&plans).unwrap();
        fs::write(root.join("VisioBible.exe"), b"exe").unwrap();
        let ini = "[BiblePath]\r\n0=\"C:\\ProgramData\\VisioBible V2.5\\Modules\"|1\r\n[settings]\r\nModulesPath=library\\Modules\r\nLanguage=ru\r\n";
        fs::write(root.join("VisioBible.ini"), ini).unwrap();
        let book = sample();
        write_book_file(&modules.join("book.vbm"), &book).unwrap();
        fs::write(
            bible.join("bibleqt.ini"),
            "BibleName=Синодальный\r\nBibleShortName=RST+\r\nPathName=01.html\r\nFullName=Бытие\r\nShortName=Быт\r\nChapterQty=50\r\n",
        )
        .unwrap();
        fs::write(
            plans.join("History.ini"),
            "<VisioBibleJournal><Journal Name=\"Songs\"><Item Caption=\"Тестовая песня\" Title=\"Тестовая песня\" Class=\"-1\"/></Journal></VisioBibleJournal>",
        )
        .unwrap();
        fs::write(
            plans.join("PlanDef.ini"),
            "[Plan]\r\nTitle=Воскресенье\r\n\r\n[Bible.1]\r\nmodule_short_name=RST+\r\nbook=43\r\nchapter=3\r\nverse=16\r\ncaption=Ин 3:16\r\n",
        )
        .unwrap();
        fs::write(
            plans.join("plan.vplt"),
            "<VisioBibleJournal><Journal Name=\"Text\"><Item Caption=\"Слайд\" Quote=\"мир\" Class=\"-1\"/></Journal></VisioBibleJournal>",
        )
        .unwrap();

        let scan = open_location(&root).unwrap();
        assert!(!scan.needs_choice);
        assert_eq!(scan.songbooks.len(), 1);
        assert_eq!(scan.songbooks[0].title, "Синтетический сборник");
        assert_eq!(scan.bibles.len(), 1);
        assert_eq!(scan.bibles[0].short_name, "RST+");
        assert_eq!(scan.bibles[0].books[0].short_name, "Быт");
        assert_eq!(scan.plans.len(), 3);
        assert!(scan
            .plans
            .iter()
            .any(|plan| plan.kind == "history" && plan.items[0].kind == "song"));
        assert!(scan
            .plans
            .iter()
            .any(|plan| plan.kind == "plandef" && plan.items[0].book == 43));
        assert!(scan.plans.iter().any(|plan| plan.kind == "vplt"));
        assert!(scan
            .module_refs
            .iter()
            .any(|path| path.replace('\\', "/").contains("ProgramData/VisioBible")));
        assert!(scan.module_refs.iter().any(|path| path.contains("library")));

        let direct = open_location(&modules).unwrap();
        assert_eq!(direct.songbooks.len(), 1);
        assert!(direct.plans_dir.ends_with("Plans"));
    }

    #[test]
    fn two_catalogs_need_a_choice_and_are_not_mixed() {
        let tmp = TmpDir::new("choice");
        let root = &tmp.path;
        let first = root.join("one").join("Modules");
        let second = root.join("two").join("Modules");
        fs::create_dir_all(&first).unwrap();
        fs::create_dir_all(&second).unwrap();
        fs::write(root.join("one").join("VisioBible.exe"), b"exe").unwrap();
        fs::write(root.join("two").join("VisioBible.exe"), b"exe").unwrap();
        write_book_file(&first.join("a.vbm"), &sample()).unwrap();
        let mut other = sample();
        other.title = "Другой".into();
        write_book_file(&second.join("b.vbm"), &other).unwrap();
        let scan = open_location(root).unwrap();
        assert!(scan.needs_choice);
        assert!(scan.songbooks.is_empty());
        assert_eq!(scan.catalogs.len(), 2);
        let chosen = open_location(Path::new(&scan.catalogs[0])).unwrap();
        assert!(!chosen.needs_choice);
        assert_eq!(chosen.songbooks.len(), 1);
    }
}
