use std::fs;
use std::path::Path;

use quick_xml::events::{BytesStart, Event};
use quick_xml::Reader;
use quick_xml::Writer;
use serde::{Deserialize, Serialize};

use crate::mojibake;
use crate::text;

/// Пункт плана в том виде, в каком его принимает `POST /api/plans`.
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct PlanItem {
    #[serde(default)]
    pub id: String,
    pub kind: String,
    #[serde(default)]
    pub song_id: String,
    #[serde(default)]
    pub songbook_id: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub module_short_name: String,
    #[serde(default)]
    pub module_short_name_second: String,
    #[serde(default)]
    pub book: i32,
    #[serde(default)]
    pub chapter: i32,
    #[serde(default)]
    pub verse: i32,
    #[serde(default)]
    pub caption: String,
    #[serde(default)]
    pub reference: String,
    #[serde(default)]
    pub quote: String,
    #[serde(default)]
    pub class: String,
    #[serde(default)]
    pub position: i32,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct RemotePlan {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub owner_user_id: Option<String>,
    #[serde(default)]
    pub dimension_id: Option<String>,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub version: i64,
    #[serde(default)]
    pub updated_at: String,
    #[serde(default)]
    pub deleted: bool,
    #[serde(default)]
    pub items: Vec<PlanItem>,
}

#[derive(Clone, Debug, PartialEq, Default)]
pub struct PlanDef {
    pub name: String,
    pub journals: Vec<String>,
    pub module_short_name: String,
    pub module_short_name_second: String,
    pub items: Vec<PlanItem>,
}

pub struct LoadedText {
    pub text: String,
    pub notice: Option<mojibake::Notice>,
}

pub fn load_text(path: &Path) -> Result<LoadedText, String> {
    let bytes = fs::read(path).map_err(|err| err.to_string())?;
    let text = text::decode_text(&bytes);
    Ok(LoadedText {
        notice: mojibake::notice(&text),
        text,
    })
}

pub fn normalize_plan_encoding(value: &str) -> &'static str {
    match value.trim().to_ascii_lowercase().as_str() {
        "windows-1251" | "cp1251" | "windows1251" => "windows-1251",
        _ => "utf-8",
    }
}

pub fn plan_file_bytes(text: &str, format: &str, encoding: &str) -> Result<Vec<u8>, String> {
    let encoding = normalize_plan_encoding(encoding);
    let body = if format == "vplt" || format == "history" {
        with_xml_encoding(text, encoding)
    } else {
        text.to_string()
    };
    if encoding == "windows-1251" {
        return mojibake::encode_windows_1251(&body);
    }
    Ok(body.into_bytes())
}

pub fn repaired_utf8(path: &Path) -> Result<Vec<u8>, String> {
    let loaded = load_text(path)?;
    let fixed = mojibake::repair(&loaded.text).ok_or("Кодировку чинить не нужно")?;
    plan_file_bytes(&fixed, file_format(path), "utf-8")
}

fn with_xml_encoding(text: &str, encoding: &str) -> String {
    let label = if encoding == "windows-1251" {
        "windows-1251"
    } else {
        "UTF-8"
    };
    let decl = format!("<?xml version=\"1.0\" encoding=\"{label}\"?>");
    let trimmed = text.trim_start_matches('\u{feff}');
    if let Some(rest) = trimmed.strip_prefix("<?xml") {
        if let Some(end) = rest.find("?>") {
            let mut after = &rest[end + 2..];
            if let Some(stripped) = after.strip_prefix('\n') {
                after = stripped;
            }
            return format!("{decl}\n{after}");
        }
    }
    format!("{decl}\n{text}")
}

/// `plan.vplt` keeps only Caption, Reference, Quote and Class.
/// VisioBible drops every other attribute the next time it saves the file.
pub fn parse_vplt(text_value: &str) -> Result<Vec<PlanItem>, String> {
    let rows = xml_items(text_value)?;
    let mut items = Vec::new();
    for (journal, attrs) in rows {
        let Some(kind) = kind_from_journal(&journal) else {
            continue;
        };
        items.push(item_from_vplt(&kind, &attrs));
    }
    Ok(items)
}

/// History.ini is a VisioBibleJournal, despite the `.ini` name.
/// Bible and song fields live here because `plan.vplt` cannot keep them.
pub fn parse_history(text_value: &str) -> Result<Vec<PlanItem>, String> {
    let rows = xml_items(text_value)?;
    let mut items = Vec::new();
    for (journal, attrs) in rows {
        let Some(kind) = kind_from_journal(&journal) else {
            continue;
        };
        items.push(item_from_history(&kind, &attrs));
    }
    Ok(items)
}

pub fn write_vplt(items: &[PlanItem]) -> Result<String, String> {
    write_xml(items, false)
}

pub fn write_history(items: &[PlanItem]) -> Result<String, String> {
    write_xml(items, true)
}

fn write_xml(items: &[PlanItem], history: bool) -> Result<String, String> {
    let mut writer = Writer::new_with_indent(Vec::new(), b' ', 2);
    writer
        .write_event(Event::Start(BytesStart::new("VisioBibleJournal")))
        .map_err(|err| err.to_string())?;
    for item in items {
        let mut journal = BytesStart::new("Journal");
        let name = journal_name(&item.kind);
        journal.push_attribute(("Name", name));
        writer
            .write_event(Event::Start(journal))
            .map_err(|err| err.to_string())?;
        let mut element = BytesStart::new("Item");
        let caption = vplt_caption(item);
        let reference = vplt_reference(item);
        let quote = vplt_quote(item);
        let class = vplt_class(item);
        element.push_attribute(("Caption", caption.as_str()));
        element.push_attribute(("Reference", reference.as_str()));
        element.push_attribute(("Quote", quote.as_str()));
        element.push_attribute(("Class", class.as_str()));
        if history {
            push_if(&mut element, "Title", &item.title);
            push_if(&mut element, "SongID", &item.song_id);
            push_if(&mut element, "SongbookID", &item.songbook_id);
            push_if(&mut element, "ModuleShortName", &item.module_short_name);
            push_if(
                &mut element,
                "ModuleShortNameSecond",
                &item.module_short_name_second,
            );
            if item.book > 0 {
                let value = item.book.to_string();
                element.push_attribute(("Book", value.as_str()));
            }
            if item.chapter > 0 {
                let value = item.chapter.to_string();
                element.push_attribute(("Chapter", value.as_str()));
            }
            if item.verse > 0 {
                let value = item.verse.to_string();
                element.push_attribute(("Verse", value.as_str()));
            }
        }
        writer
            .write_event(Event::Empty(element))
            .map_err(|err| err.to_string())?;
        writer
            .write_event(Event::End(BytesStart::new("Journal").to_end()))
            .map_err(|err| err.to_string())?;
    }
    writer
        .write_event(Event::End(BytesStart::new("VisioBibleJournal").to_end()))
        .map_err(|err| err.to_string())?;
    String::from_utf8(writer.into_inner()).map_err(|err| err.to_string())
}

fn push_if(element: &mut BytesStart, key: &str, value: &str) {
    if !value.is_empty() {
        element.push_attribute((key, value));
    }
}

/// PlanDef.ini: `[Plan]`, then `[Bible.N]`, `[Text.N]`, `[Songs.N]`.
/// A newline inside a value is stored as `\n`.
pub fn parse_plan_def(text_value: &str) -> PlanDef {
    let text_value = text_value.replace("\r\n", "\n");
    let mut def = PlanDef::default();
    let mut section = String::new();
    let mut keys: Vec<(String, String)> = Vec::new();
    let flush = |section: &mut String, keys: &mut Vec<(String, String)>, def: &mut PlanDef| {
        if section.is_empty() {
            return;
        }
        if section.eq_ignore_ascii_case("plan") {
            section.clear();
            keys.clear();
            return;
        }
        let base = section.split('.').next().unwrap_or(section);
        if let Some(kind) = kind_from_journal(base) {
            def.items.push(item_from_ini(&kind, keys));
            if !def
                .journals
                .iter()
                .any(|name| name.eq_ignore_ascii_case(base))
            {
                def.journals.push(journal_name(&kind).to_string());
            }
        }
        section.clear();
        keys.clear();
    };
    for line in text_value.split('\n') {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') || line.starts_with(';') {
            continue;
        }
        if line.starts_with('[') && line.ends_with(']') && line.len() >= 2 {
            flush(&mut section, &mut keys, &mut def);
            section = line[1..line.len() - 1].trim().to_string();
            continue;
        }
        let Some((key, value)) = line.split_once('=') else {
            continue;
        };
        let key = key.trim().to_ascii_lowercase();
        let value = ini_unescape(value.trim());
        if section.eq_ignore_ascii_case("plan") && key == "title" {
            def.name = value;
            continue;
        }
        keys.push((key, value));
    }
    flush(&mut section, &mut keys, &mut def);
    if let Some(bible) = def.items.iter().find(|item| item.kind == "bible") {
        def.module_short_name = bible.module_short_name.clone();
        def.module_short_name_second = bible.module_short_name_second.clone();
    }
    def
}

pub fn write_plan_def(def: &PlanDef) -> String {
    let mut out = String::new();
    out.push_str("[Plan]\n");
    out.push_str("Title=");
    out.push_str(&ini_escape(&def.name));
    out.push('\n');
    for (index, item) in def.items.iter().enumerate() {
        out.push('\n');
        out.push('[');
        out.push_str(journal_name(&item.kind));
        out.push('.');
        out.push_str(&(index + 1).to_string());
        out.push_str("]\n");
        ini_line(&mut out, "caption", &item.caption);
        ini_line(&mut out, "reference", &item.reference);
        ini_line(&mut out, "quote", &item.quote);
        ini_line(&mut out, "class", &vplt_class(item));
        match item.kind.as_str() {
            "song" => {
                ini_line(&mut out, "song_id", &item.song_id);
                ini_line(&mut out, "songbook_id", &item.songbook_id);
                ini_line(&mut out, "title", &item.title);
            }
            "bible" => {
                ini_line(&mut out, "module_short_name", &item.module_short_name);
                ini_line(
                    &mut out,
                    "module_short_name_second",
                    &item.module_short_name_second,
                );
                ini_line(&mut out, "book", &item.book.to_string());
                ini_line(&mut out, "chapter", &item.chapter.to_string());
                ini_line(&mut out, "verse", &item.verse.to_string());
            }
            _ => {}
        }
    }
    out
}

pub fn render_file(
    path: &Path,
    items: &[PlanItem],
    title: &str,
    encoding: &str,
) -> Result<Vec<u8>, String> {
    let format = file_format(path);
    let text_value = match format {
        "history" => write_history(items)?,
        "plandef" => write_plan_def(&PlanDef {
            name: title.to_string(),
            items: items.to_vec(),
            ..PlanDef::default()
        }),
        _ => write_vplt(items)?,
    };
    plan_file_bytes(&text_value, format, encoding)
}

pub fn write_items(
    path: &Path,
    items: &[PlanItem],
    title: &str,
    encoding: &str,
) -> Result<(), String> {
    let bytes = render_file(path, items, title, encoding)?;
    if let Some(parent) = path.parent() {
        if !parent.as_os_str().is_empty() {
            fs::create_dir_all(parent).map_err(|err| err.to_string())?;
        }
    }
    fs::write(path, bytes).map_err(|err| err.to_string())
}

pub fn file_format(path: &Path) -> &'static str {
    let name = path
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("")
        .to_ascii_lowercase();
    if name == "history.ini" {
        "history"
    } else if name == "plandef.ini" || name.ends_with(".plandef.ini") {
        "plandef"
    } else {
        "vplt"
    }
}

/// What VisioBible will still see after it rewrites `plan.vplt`.
pub fn project(items: &[PlanItem], format: &str) -> Vec<PlanItem> {
    match format {
        "vplt" => parse_vplt(&write_vplt(items).unwrap_or_default()).unwrap_or_default(),
        "history" => parse_history(&write_history(items).unwrap_or_default()).unwrap_or_default(),
        _ => {
            let def = PlanDef {
                name: String::new(),
                items: items.to_vec(),
                ..PlanDef::default()
            };
            parse_plan_def(&write_plan_def(&def)).items
        }
    }
}

/// A vplt edit should not wipe book and translation fields that the file cannot store.
pub fn items_for_push(format: &str, base: &[PlanItem], local: &[PlanItem]) -> Vec<PlanItem> {
    if format != "vplt" || base.is_empty() || base.len() != local.len() {
        return local.to_vec();
    }
    base.iter()
        .zip(local)
        .map(|(base, local)| {
            let mut item = base.clone();
            item.caption = local.caption.clone();
            item.reference = local.reference.clone();
            item.quote = local.quote.clone();
            item.class = local.class.clone();
            if item.kind == "song" {
                if !local.title.is_empty() {
                    item.title = local.title.clone();
                }
                if !local.song_id.is_empty() {
                    item.song_id = local.song_id.clone();
                }
                if !local.songbook_id.is_empty() {
                    item.songbook_id = local.songbook_id.clone();
                }
            }
            item
        })
        .collect()
}

pub fn write_new_plan(
    dir: &Path,
    name: &str,
    items: &[PlanItem],
    encoding: &str,
) -> Result<std::path::PathBuf, String> {
    fs::create_dir_all(dir).map_err(|err| err.to_string())?;
    let stem = file_stem(name);
    let bible = items
        .iter()
        .any(|item| item.kind == "bible" && item.book > 0);
    let path = if bible {
        dir.join(format!("{stem}.plandef.ini"))
    } else {
        dir.join(format!("{stem}.vplt"))
    };
    if path.exists() {
        return Err("Такой план уже есть".into());
    }
    write_items(&path, items, name, encoding)?;
    Ok(path)
}

pub fn export_text(format: &str, title: &str, items: &[PlanItem]) -> Result<String, String> {
    let format = match format.trim().to_ascii_lowercase().as_str() {
        "json" => "json",
        "history" => "history",
        "plandef" | "plan_def" | "ini" => "plandef",
        "vplt" => "vplt",
        _ => return Err("Неизвестный формат".into()),
    };
    let text_value = match format {
        "json" => serde_json::to_string_pretty(&serde_json::json!({
            "title": title,
            "items": items,
        }))
        .map_err(|err| err.to_string())?,
        "history" => write_history(items)?,
        "plandef" => write_plan_def(&PlanDef {
            name: title.to_string(),
            items: items.to_vec(),
            ..PlanDef::default()
        }),
        _ => write_vplt(items)?,
    };
    if format == "json" || format == "plandef" {
        return Ok(text_value);
    }
    let bytes = plan_file_bytes(&text_value, format, "utf-8")?;
    String::from_utf8(bytes).map_err(|err| err.to_string())
}

pub fn plan_path_allowed(plans_dir: &Path, file: &Path) -> bool {
    if !file.is_file() {
        return false;
    }
    let Some(name) = file.file_name().and_then(|name| name.to_str()) else {
        return false;
    };
    if !is_plan_filename(name) {
        return false;
    }
    let Ok(root) = plans_dir.canonicalize() else {
        return false;
    };
    let Ok(target) = file.canonicalize() else {
        return false;
    };
    target.starts_with(root)
}

pub fn path_under(root: &Path, file: &Path) -> bool {
    let Ok(root) = root.canonicalize() else {
        return false;
    };
    let anchor = if file.exists() {
        file.to_path_buf()
    } else {
        match file.parent() {
            Some(parent) if !parent.as_os_str().is_empty() => parent.to_path_buf(),
            _ => return false,
        }
    };
    let Ok(anchor) = anchor.canonicalize() else {
        return false;
    };
    anchor.starts_with(root)
}

fn is_plan_filename(name: &str) -> bool {
    let name = name.to_ascii_lowercase();
    name.ends_with(".vplt")
        || name == "history.ini"
        || name == "plandef.ini"
        || name.ends_with(".plandef.ini")
}

pub fn file_stem(name: &str) -> String {
    let mut out = String::new();
    for ch in name.chars() {
        if ch.is_alphanumeric() || ch == ' ' || ch == '-' || ch == '_' {
            out.push(ch);
        }
    }
    let out = out.trim();
    if out.is_empty() {
        "plan".into()
    } else {
        out.to_string()
    }
}

pub fn summarize(items: &[PlanItem]) -> String {
    if items.is_empty() {
        return "пустой план".into();
    }
    let mut parts = Vec::new();
    for item in items.iter().take(4) {
        parts.push(match item.kind.as_str() {
            "song" => {
                let title = if item.title.is_empty() {
                    &item.caption
                } else {
                    &item.title
                };
                format!("песня «{title}»")
            }
            "bible" => format!("Библия {}", bible_label(item)),
            _ => {
                let caption = if item.caption.is_empty() {
                    &item.quote
                } else {
                    &item.caption
                };
                format!("текст «{caption}»")
            }
        });
    }
    if items.len() > 4 {
        parts.push("…".into());
    }
    parts.join(", ")
}

fn bible_label(item: &PlanItem) -> String {
    if !item.caption.is_empty() {
        return item.caption.clone();
    }
    if !item.reference.is_empty() {
        return item.reference.clone();
    }
    let module = if item.module_short_name.is_empty() {
        "перевод"
    } else {
        item.module_short_name.as_str()
    };
    format!("{module} {}:{}", item.chapter, item.verse)
}

fn item_from_vplt(kind: &str, attrs: &[(String, String)]) -> PlanItem {
    let caption = attr(attrs, "Caption");
    let reference = attr(attrs, "Reference");
    let quote = attr(attrs, "Quote");
    let class = attr(attrs, "Class");
    let mut item = PlanItem {
        kind: kind.to_string(),
        caption: caption.clone(),
        reference: reference.clone(),
        quote: quote.clone(),
        class: if class.is_empty() { "-1".into() } else { class },
        ..PlanItem::default()
    };
    match kind {
        "song" => {
            item.title = caption;
            item.song_id = quote;
            item.songbook_id = reference;
        }
        "bible" if valid_module_name(&caption) => {
            item.module_short_name = caption;
        }
        _ => {}
    }
    item
}

fn item_from_history(kind: &str, attrs: &[(String, String)]) -> PlanItem {
    let mut item = item_from_vplt(kind, attrs);
    let title = attr(attrs, "Title");
    if !title.is_empty() {
        item.title = title;
    }
    let song_id = attr(attrs, "SongID");
    if !song_id.is_empty() {
        item.song_id = song_id;
    }
    let songbook = attr(attrs, "SongbookID");
    if !songbook.is_empty() {
        item.songbook_id = songbook;
    }
    let module = first_nonempty(&[attr(attrs, "ModuleShortName"), attr(attrs, "Module")]);
    if !module.is_empty() {
        item.module_short_name = module;
    }
    let second = first_nonempty(&[
        attr(attrs, "ModuleShortNameSecond"),
        attr(attrs, "ModuleSecond"),
    ]);
    if !second.is_empty() {
        item.module_short_name_second = second;
    }
    item.book = number(&attr(attrs, "Book"));
    item.chapter = number(&attr(attrs, "Chapter"));
    item.verse = number(&attr(attrs, "Verse"));
    item.caption = attr(attrs, "Caption");
    item.reference = attr(attrs, "Reference");
    item.quote = attr(attrs, "Quote");
    item
}

fn item_from_ini(kind: &str, keys: &[(String, String)]) -> PlanItem {
    let mut item = PlanItem {
        kind: kind.to_string(),
        caption: key(keys, "caption"),
        reference: key(keys, "reference"),
        quote: key(keys, "quote"),
        class: key(keys, "class"),
        title: key(keys, "title"),
        song_id: key(keys, "song_id"),
        songbook_id: key(keys, "songbook_id"),
        module_short_name: key(keys, "module_short_name"),
        module_short_name_second: key(keys, "module_short_name_second"),
        book: number(&key(keys, "book")),
        chapter: number(&key(keys, "chapter")),
        verse: number(&key(keys, "verse")),
        ..PlanItem::default()
    };
    if item.class.is_empty() {
        item.class = "-1".into();
    }
    item
}

fn vplt_caption(item: &PlanItem) -> String {
    if item.kind == "song" {
        if item.title.is_empty() {
            item.caption.clone()
        } else {
            item.title.clone()
        }
    } else if item.kind == "bible" && item.caption.is_empty() {
        item.module_short_name.clone()
    } else {
        item.caption.clone()
    }
}

fn vplt_reference(item: &PlanItem) -> String {
    if item.kind == "song" {
        return item.songbook_id.clone();
    }
    if !item.reference.is_empty() {
        return item.reference.clone();
    }
    if item.kind == "bible" && item.book > 0 {
        return format!("{} {}:{}", item.book, item.chapter, item.verse);
    }
    String::new()
}

fn vplt_quote(item: &PlanItem) -> String {
    if item.kind == "song" {
        item.song_id.clone()
    } else {
        item.quote.clone()
    }
}

fn vplt_class(item: &PlanItem) -> String {
    if item.class.is_empty() {
        "-1".into()
    } else {
        item.class.clone()
    }
}

fn kind_from_journal(name: &str) -> Option<String> {
    match name.trim().to_ascii_lowercase().as_str() {
        "text" => Some("text".into()),
        "bible" => Some("bible".into()),
        "songs" | "song" => Some("song".into()),
        _ => None,
    }
}

fn journal_name(kind: &str) -> &'static str {
    match kind {
        "bible" => "Bible",
        "song" => "Songs",
        _ => "Text",
    }
}

fn valid_module_name(name: &str) -> bool {
    let mut chars = name.chars();
    let Some(first) = chars.next() else {
        return false;
    };
    if !first.is_ascii_alphanumeric() || name.len() > 32 {
        return false;
    }
    chars.all(|ch| ch.is_ascii_alphanumeric() || matches!(ch, '_' | '+' | '-'))
}

type XmlItem = (String, Vec<(String, String)>);

fn xml_items(text_value: &str) -> Result<Vec<XmlItem>, String> {
    let mut reader = Reader::from_str(text_value);
    reader.config_mut().trim_text(true);
    let mut items = Vec::new();
    let mut journal = String::new();
    let mut buf = Vec::new();
    loop {
        match reader.read_event_into(&mut buf) {
            Ok(Event::Start(event)) | Ok(Event::Empty(event)) => {
                let name = local_name(event.name().as_ref()).to_vec();
                if name == b"Journal" {
                    journal = attr_event(&event, "Name");
                } else if name == b"Item" {
                    items.push((journal.clone(), all_attrs(&event)));
                }
            }
            Ok(Event::End(event)) => {
                if local_name(event.name().as_ref()).to_vec() == b"Journal" {
                    journal.clear();
                }
            }
            Ok(Event::Eof) => break,
            Err(err) => return Err(format!("план не прочитан: {err}")),
            _ => {}
        }
        buf.clear();
    }
    Ok(items)
}

fn all_attrs(event: &BytesStart) -> Vec<(String, String)> {
    event
        .attributes()
        .flatten()
        .filter_map(|attribute| {
            let key = String::from_utf8_lossy(local_name(attribute.key.as_ref())).to_string();
            let value = attribute.unescape_value().ok()?.to_string();
            Some((key, value))
        })
        .collect()
}

fn attr_event(event: &BytesStart, key: &str) -> String {
    attr(&all_attrs(event), key)
}

fn attr(attrs: &[(String, String)], key: &str) -> String {
    attrs
        .iter()
        .find(|(name, _)| name.eq_ignore_ascii_case(key))
        .map(|(_, value)| value.clone())
        .unwrap_or_default()
}

fn key(keys: &[(String, String)], name: &str) -> String {
    keys.iter()
        .find(|(key, _)| key.eq_ignore_ascii_case(name))
        .map(|(_, value)| value.clone())
        .unwrap_or_default()
}

fn number(value: &str) -> i32 {
    value.trim().parse().unwrap_or(0)
}

fn first_nonempty(values: &[String]) -> String {
    values
        .iter()
        .find(|value| !value.trim().is_empty())
        .cloned()
        .unwrap_or_default()
}

fn local_name(name: &[u8]) -> &[u8] {
    name.rsplit(|byte| *byte == b':').next().unwrap_or(name)
}

fn ini_line(out: &mut String, key: &str, value: &str) {
    out.push_str(key);
    out.push('=');
    out.push_str(&ini_escape(value));
    out.push('\n');
}

fn ini_escape(value: &str) -> String {
    value
        .replace('\\', "\\\\")
        .replace("\r\n", "\\n")
        .replace(['\n', '\r'], "\\n")
}

fn ini_unescape(value: &str) -> String {
    let mut out = String::new();
    let chars: Vec<char> = value.chars().collect();
    let mut index = 0;
    while index < chars.len() {
        if chars[index] == '\\' && index + 1 < chars.len() {
            match chars[index + 1] {
                'n' => {
                    out.push('\n');
                    index += 2;
                    continue;
                }
                '\\' => {
                    out.push('\\');
                    index += 2;
                    continue;
                }
                _ => {}
            }
        }
        out.push(chars[index]);
        index += 1;
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use encoding_rs::{WINDOWS_1251, WINDOWS_1252};

    fn song() -> PlanItem {
        PlanItem {
            kind: "song".into(),
            title: "Тестовая песня".into(),
            song_id: "11111111-1111-4111-8111-111111111111".into(),
            songbook_id: "22222222-2222-4222-8222-222222222222".into(),
            caption: "Песнь".into(),
            class: "-1".into(),
            ..PlanItem::default()
        }
    }

    fn bible() -> PlanItem {
        PlanItem {
            kind: "bible".into(),
            caption: "Ин 3:16".into(),
            reference: "Ин 3:16".into(),
            module_short_name: "RST+".into(),
            module_short_name_second: "NRT".into(),
            book: 43,
            chapter: 3,
            verse: 16,
            class: "-1".into(),
            ..PlanItem::default()
        }
    }

    fn slide() -> PlanItem {
        PlanItem {
            kind: "text".into(),
            caption: "Объявление".into(),
            quote: "Сбор после служения".into(),
            class: "-1".into(),
            ..PlanItem::default()
        }
    }

    #[test]
    fn vplt_keeps_only_visio_fields_and_history_keeps_the_verse() {
        let items = vec![song(), bible(), slide()];
        let xml = write_vplt(&items).unwrap();
        assert!(xml.contains("Caption="));
        assert!(!xml.contains("ModuleShortName"));
        assert!(!xml.contains("Book="));
        let parsed = parse_vplt(&xml).unwrap();
        assert_eq!(parsed[0].kind, "song");
        assert_eq!(parsed[0].title, "Тестовая песня");
        assert_eq!(parsed[0].song_id, items[0].song_id);
        assert_eq!(parsed[1].kind, "bible");
        assert_eq!(parsed[1].book, 0, "vplt не хранит номер стиха");
        assert_eq!(parsed[2].kind, "text");
        assert_eq!(parsed[2].quote, "Сбор после служения");

        let history = write_history(&items).unwrap();
        assert!(history.contains("ModuleShortName=\"RST+\""));
        assert!(history.contains("ModuleShortNameSecond=\"NRT\""));
        let full = parse_history(&history).unwrap();
        assert_eq!(full[1].book, 43);
        assert_eq!(full[1].chapter, 3);
        assert_eq!(full[1].verse, 16);
        assert_eq!(full[1].module_short_name_second, "NRT");
        assert_eq!(full[0].title, "Тестовая песня");
    }

    #[test]
    fn history_accepts_windows_1251() {
        let utf = r#"<VisioBibleJournal><Journal Name="Bible"><Item Caption="Ин 3:16" ModuleShortName="RST+" ModuleShortNameSecond="NRT" Book="43" Chapter="3" Verse="16" Reference="Ин 3:16" Quote=""/></Journal></VisioBibleJournal>"#;
        let (encoded, _, _) = WINDOWS_1251.encode(utf);
        let items = parse_history(&text::decode_text(&encoded)).unwrap();
        assert_eq!(items[0].caption, "Ин 3:16");
        assert_eq!(items[0].module_short_name, "RST+");
        assert_eq!(
            (items[0].book, items[0].chapter, items[0].verse),
            (43, 3, 16)
        );
    }

    #[test]
    fn plan_def_sections_keep_translation_and_escaped_newlines() {
        let def = PlanDef {
            name: "Воскресенье".into(),
            items: vec![bible(), slide()],
            ..PlanDef::default()
        };
        let mut with_break = def.clone();
        with_break.items[1].quote = "строка\nвторая".into();
        let text_value = write_plan_def(&with_break);
        assert!(text_value.contains("[Plan]"));
        assert!(text_value.contains("[Bible.1]"));
        assert!(text_value.contains("[Text.2]"));
        assert!(text_value.contains("module_short_name=RST+"));
        assert!(text_value.contains("\\n"));
        let parsed = parse_plan_def(&text_value);
        assert_eq!(parsed.name, "Воскресенье");
        assert_eq!(parsed.items[0].module_short_name, "RST+");
        assert_eq!(parsed.items[0].module_short_name_second, "NRT");
        assert_eq!(
            (
                parsed.items[0].book,
                parsed.items[0].chapter,
                parsed.items[0].verse
            ),
            (43, 3, 16)
        );
        assert_eq!(parsed.items[1].quote, "строка\nвторая");
    }

    #[test]
    fn windows_1251_write_matches_the_header_and_does_not_replace_the_file() {
        let dir = crate::paths::TmpDir::new("plan-enc");
        let path = dir.path.join("plan.vplt");
        fs::write(&path, "старый").unwrap();
        let mut accent = slide();
        accent.quote = "café".into();
        let err = write_items(&path, &[accent], "Служение", "windows-1251").unwrap_err();
        assert!(err.contains("windows-1251"));
        assert_eq!(fs::read(&path).unwrap(), "старый".as_bytes());

        let plain = PlanItem {
            kind: "text".into(),
            caption: "Привет".into(),
            quote: "мир".into(),
            class: "-1".into(),
            ..PlanItem::default()
        };
        write_items(&path, &[plain], "Служение", "windows-1251").unwrap();
        let bytes = fs::read(&path).unwrap();
        assert!(std::str::from_utf8(&bytes).is_err());
        let text = text::decode_text(&bytes);
        assert!(text.contains("encoding=\"windows-1251\""));
        assert!(text.contains("Привет"));
        assert!(!text.contains("encoding=\"UTF-8\""));

        let mojibake = decode_mojibake("Привет");
        let broken = dir.path.join("History.ini");
        fs::write(
            &broken,
            format!("<VisioBibleJournal><Journal Name=\"Text\"><Item Caption=\"{mojibake}\" Quote=\"\" Class=\"-1\"/></Journal></VisioBibleJournal>"),
        )
        .unwrap();
        let repaired = repaired_utf8(&broken).unwrap();
        let repaired_text = String::from_utf8(repaired).unwrap();
        assert!(repaired_text.contains("encoding=\"UTF-8\""));
        assert!(repaired_text.contains("Привет"));
        assert!(fs::read(&broken).unwrap().starts_with(b"<Visio"));
    }

    fn decode_mojibake(text: &str) -> String {
        let (bytes, _, _) = WINDOWS_1252.decode(text.as_bytes());
        bytes.into_owned()
    }

    #[test]
    fn export_text_covers_the_four_formats_and_delete_stays_inside_plans() {
        let dir = crate::paths::TmpDir::new("plan-export");
        let plans = dir.path.join("Plans");
        fs::create_dir_all(&plans).unwrap();
        let file = plans.join("Sunday.vplt");
        fs::write(&file, "x").unwrap();
        assert!(plan_path_allowed(&plans, &file));
        assert!(path_under(&dir.path, &file));
        assert!(path_under(&dir.path, &plans.join("new.vplt")));
        let outside = dir.path.join("note.txt");
        fs::write(&outside, "x").unwrap();
        assert!(!plan_path_allowed(&plans, &outside));
        assert!(!path_under(&plans, &outside));
        let history = plans.join("History.ini");
        fs::write(&history, "x").unwrap();
        assert!(plan_path_allowed(&plans, &history));
        let def = plans.join("Week.plandef.ini");
        fs::write(&def, "x").unwrap();
        assert!(plan_path_allowed(&plans, &def));
        let song = plans.join("song.vbm");
        fs::write(&song, "x").unwrap();
        assert!(!plan_path_allowed(&plans, &song));
        let other = crate::paths::TmpDir::new("plan-other");
        assert!(!path_under(&plans, &other.path.join("missing.vplt")));

        let items = vec![bible(), slide()];
        let json = export_text("json", "Воскресенье", &items).unwrap();
        assert!(json.contains("\"title\": \"Воскресенье\""));
        assert!(json.contains("\"kind\": \"bible\""));
        let vplt = export_text("vplt", "Воскресенье", &items).unwrap();
        assert!(vplt.contains("encoding=\"UTF-8\""));
        assert!(!vplt.contains("ModuleShortName"));
        let history_text = export_text("history", "Воскресенье", &items).unwrap();
        assert!(history_text.contains("Book=\"43\""));
        let def_text = export_text("plandef", "Воскресенье", &items).unwrap();
        assert!(def_text.contains("Title=Воскресенье"));
        assert!(export_text("nope", "Воскресенье", &items).is_err());
    }

    #[test]
    fn vplt_push_keeps_verse_numbers_from_the_baseline() {
        let base = vec![bible()];
        let mut edited = project(&base, "vplt");
        edited[0].caption = "Другая подпись".into();
        let pushed = items_for_push("vplt", &base, &edited);
        assert_eq!(pushed[0].book, 43);
        assert_eq!(pushed[0].module_short_name, "RST+");
        assert_eq!(pushed[0].caption, "Другая подпись");
    }
}
