use encoding_rs::WINDOWS_1251;

#[derive(Clone, Debug, PartialEq)]
pub struct Ini {
    pub sections: Vec<IniSection>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct IniSection {
    pub name: String,
    pub lines: Vec<(String, String)>,
}

/// VisioBible writes UTF-8 or Windows-1251, sometimes with a BOM.
pub fn decode_text(data: &[u8]) -> String {
    let data = data.strip_prefix(&[0xEF, 0xBB, 0xBF]).unwrap_or(data);
    if let Ok(text) = std::str::from_utf8(data) {
        return text.to_string();
    }
    let (text, _, _) = WINDOWS_1251.decode(data);
    text.into_owned()
}

pub fn parse_ini(text: &str) -> Ini {
    let mut sections = Vec::new();
    let mut current = IniSection {
        name: String::new(),
        lines: Vec::new(),
    };
    for line in text.split('\n') {
        let line = line.trim_end_matches('\r').trim();
        if line.is_empty() || line.starts_with(';') || line.starts_with('#') {
            continue;
        }
        if let Some(name) = line
            .strip_prefix('[')
            .and_then(|rest| rest.strip_suffix(']'))
        {
            if !current.name.is_empty() || !current.lines.is_empty() {
                sections.push(current);
            }
            current = IniSection {
                name: name.trim().to_string(),
                lines: Vec::new(),
            };
            continue;
        }
        let Some((key, value)) = line.split_once('=') else {
            continue;
        };
        current
            .lines
            .push((key.trim().to_string(), value.trim().to_string()));
    }
    if !current.name.is_empty() || !current.lines.is_empty() {
        sections.push(current);
    }
    Ini { sections }
}

/// VisioBible stores a path in quotes and may append `|flags` after them.
pub fn quoted_path(value: &str) -> Option<&str> {
    let start = value.find('"')?;
    let rest = &value[start + 1..];
    let end = rest.find('"')?;
    Some(&rest[..end])
}
