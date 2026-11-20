use encoding_rs::{Encoding, WINDOWS_1251, WINDOWS_1252};

pub const WARNING: &str =
    "Похоже, кириллица была прочитана как windows-1251 или latin-1 и сохранена ещё раз.";

#[derive(Clone, Debug, PartialEq)]
pub struct Notice {
    pub warning: String,
    pub before: String,
    pub after: String,
}

/// Та же эвристика, что в `internal/server/mojibake.go`.
/// Текст не подменяется: вызывающий показывает предупреждение и пишет файл только по просьбе.
pub fn repair(text: &str) -> Option<String> {
    if text.is_empty() {
        return None;
    }
    let mut best: Option<String> = None;
    let mut consider = |cand: String| {
        if !better(text, &cand) {
            return;
        }
        if best.as_ref().is_none_or(|current| prefer(&cand, current)) {
            best = Some(cand);
        }
    };
    for kind in [Single::Win1251, Single::Win1252, Single::Latin1] {
        let Some(raw) = encode_single(kind, text) else {
            continue;
        };
        if let Ok(utf) = std::str::from_utf8(&raw) {
            consider(utf.to_string());
        }
        if kind != Single::Win1251 {
            if let Some(decoded) = decode_1251(&raw) {
                consider(decoded);
            }
        }
    }
    best.filter(|value| value != text)
}

pub fn notice(text: &str) -> Option<Notice> {
    let fixed = repair(text)?;
    Some(Notice {
        warning: WARNING.to_string(),
        before: clip(text),
        after: clip(&fixed),
    })
}

pub fn encode_windows_1251(text: &str) -> Result<Vec<u8>, String> {
    encode_strict(WINDOWS_1251, text).ok_or_else(|| {
        "В тексте есть символы, которых нет в windows-1251. Файл не перезаписан.".to_string()
    })
}

#[derive(Clone, Copy, PartialEq)]
enum Single {
    Win1251,
    Win1252,
    Latin1,
}

fn encode_single(kind: Single, text: &str) -> Option<Vec<u8>> {
    match kind {
        Single::Win1251 => encode_strict(WINDOWS_1251, text),
        Single::Win1252 => encode_strict(WINDOWS_1252, text),
        Single::Latin1 => encode_latin1(text),
    }
}

fn encode_strict(encoding: &'static Encoding, text: &str) -> Option<Vec<u8>> {
    let (bytes, _, unmappable) = encoding.encode(text);
    if unmappable {
        return None;
    }
    Some(bytes.into_owned())
}

fn encode_latin1(text: &str) -> Option<Vec<u8>> {
    let mut out = Vec::with_capacity(text.len());
    for ch in text.chars() {
        if ch > '\u{00FF}' {
            return None;
        }
        out.push(ch as u8);
    }
    Some(out)
}

fn decode_1251(bytes: &[u8]) -> Option<String> {
    let (text, _, had_errors) = WINDOWS_1251.decode(bytes);
    if had_errors || text.contains('\u{FFFD}') {
        return None;
    }
    Some(text.into_owned())
}

fn better(before: &str, after: &str) -> bool {
    if after.is_empty() || after == before || after.contains('\u{FFFD}') {
        return false;
    }
    let pairs_before = marker_pairs(before);
    let pairs_after = marker_pairs(after);
    let cyr_before = cyrillic_count(before);
    let cyr_after = cyrillic_count(after);
    let share_before = cyrillic_share(before);
    let share_after = cyrillic_share(after);
    if pairs_before > 0
        && pairs_after == 0
        && cyr_after > 0
        && share_after + 0.001 >= share_before
        && after.chars().count() < before.chars().count()
    {
        return true;
    }
    cyr_after > cyr_before
        && share_after > share_before
        && pairs_after <= pairs_before
        && cp1251_read_as_latin1(before, after)
}

fn prefer(candidate: &str, current: &str) -> bool {
    let pairs_candidate = marker_pairs(candidate);
    let pairs_current = marker_pairs(current);
    if pairs_candidate != pairs_current {
        return pairs_candidate < pairs_current;
    }
    let share_candidate = cyrillic_share(candidate);
    let share_current = cyrillic_share(current);
    if (share_candidate - share_current).abs() > f64::EPSILON {
        return share_candidate > share_current;
    }
    candidate.chars().count() < current.chars().count()
}

fn marker_pairs(text: &str) -> usize {
    let chars: Vec<char> = text.chars().collect();
    let mut count = 0;
    for index in 0..chars.len().saturating_sub(1) {
        let next = chars[index + 1];
        match chars[index] {
            '\u{00D0}' | '\u{00D1}' => {
                if ('\u{0080}'..='\u{00BF}').contains(&next) {
                    count += 1;
                    continue;
                }
                if let Some(byte) = single_byte(WINDOWS_1252, next) {
                    if (0x80..=0xBF).contains(&byte) {
                        count += 1;
                    }
                }
            }
            'Р' | 'С' => {
                if let Some(byte) = single_byte(WINDOWS_1251, next) {
                    if (0x80..=0xBF).contains(&byte) {
                        count += 1;
                    }
                }
            }
            _ => {}
        }
    }
    count
}

fn cp1251_read_as_latin1(before: &str, after: &str) -> bool {
    let before: Vec<char> = before.chars().collect();
    let after: Vec<char> = after.chars().collect();
    if before.len() != after.len() {
        return false;
    }
    let mut changed = 0;
    let mut distinctive = false;
    let mut vowel = false;
    for (left, right) in before.iter().zip(after.iter()) {
        if left == right {
            continue;
        }
        if *left < '\u{0080}' || is_cyrillic_letter(*left) || !is_cyrillic_letter(*right) {
            return false;
        }
        if is_cyrillic_vowel(*right) {
            vowel = true;
        }
        if distinctive_latin1(*left) {
            distinctive = true;
        }
        changed += 1;
    }
    changed >= 4 && distinctive && vowel
}

fn distinctive_latin1(ch: char) -> bool {
    matches!(
        ch,
        'ð' | 'þ' | 'ø' | '÷' | 'ÿ' | 'Ï' | 'Ð' | 'ý' | '¸' | '¨' | 'å' | 'æ' | 'õ' | 'ã'
    )
}

fn is_cyrillic_letter(ch: char) -> bool {
    matches!(
        ch,
        '\u{0400}'..='\u{04FF}'
            | '\u{0500}'..='\u{052F}'
            | '\u{2DE0}'..='\u{2DFF}'
            | '\u{A640}'..='\u{A69F}'
            | '\u{1C80}'..='\u{1C8F}'
    ) && ch.is_alphabetic()
}

fn is_cyrillic_vowel(ch: char) -> bool {
    matches!(
        ch,
        'а' | 'е'
            | 'и'
            | 'о'
            | 'у'
            | 'ы'
            | 'э'
            | 'ю'
            | 'я'
            | 'ё'
            | 'А'
            | 'Е'
            | 'И'
            | 'О'
            | 'У'
            | 'Ы'
            | 'Э'
            | 'Ю'
            | 'Я'
            | 'Ё'
    )
}

fn cyrillic_count(text: &str) -> usize {
    text.chars().filter(|ch| is_cyrillic_letter(*ch)).count()
}

fn cyrillic_share(text: &str) -> f64 {
    let mut letters = 0;
    let mut cyr = 0;
    for ch in text.chars() {
        if ch.is_alphabetic() {
            letters += 1;
            if is_cyrillic_letter(ch) {
                cyr += 1;
            }
        }
    }
    if letters == 0 {
        0.0
    } else {
        cyr as f64 / letters as f64
    }
}

fn single_byte(encoding: &'static Encoding, ch: char) -> Option<u8> {
    let mut buf = [0u8; 4];
    let (bytes, _, bad) = encoding.encode(ch.encode_utf8(&mut buf));
    if bad || bytes.len() != 1 {
        return None;
    }
    Some(bytes[0])
}

fn clip(text: &str) -> String {
    let flat = text.split_whitespace().collect::<Vec<_>>().join(" ");
    let mut out = String::new();
    for (index, ch) in flat.chars().enumerate() {
        if index == 280 {
            out.push('…');
            break;
        }
        out.push(ch);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use encoding_rs::{Encoding, WINDOWS_1251, WINDOWS_1252};

    fn decode_encoding(encoding: &'static Encoding, bytes: &[u8]) -> String {
        let (text, _, _) = encoding.decode(bytes);
        text.into_owned()
    }

    fn samples_left_alone() -> &'static [&'static str] {
        &[
            "",
            "Привет, мир",
            "Ёлка у реки. Ин. 3:16",
            "Ибо так возлюбил Бог мир, что отдал Сына Своего Единородного.",
            "Grüße aus Köln",
            "Straße, schön und groß",
            "für München",
            "Café naïve à l'été",
            "Ça va très bien, où est l'hôtel?",
            "L'élève est à Paris",
            "Привет, naïve café und Grüße",
            "Hello, Ин. 3:16 и Straße",
            "äöüß",
            "àèéêëîôû",
        ]
    }

    #[test]
    fn repairs_double_encoded_cyrillic_only() {
        let plain = "Привет, мир";
        let as_1252 = decode_encoding(WINDOWS_1252, plain.as_bytes());
        let as_1251 = decode_encoding(WINDOWS_1251, plain.as_bytes());
        let cp = encode_strict(WINDOWS_1251, plain).unwrap();
        let as_latin: String = cp.iter().copied().map(char::from).collect();

        assert_eq!(repair(&as_1252).as_deref(), Some(plain));
        assert!(as_1252.contains('Ð'));
        assert_eq!(repair(&as_1251).as_deref(), Some(plain));
        assert_eq!(repair(&as_latin).as_deref(), Some(plain));
        assert!(repair(plain).is_none());

        let xml = r#"<VisioBibleJournal><Journal Name="Text"><Item Caption="Привет" Quote="мир" Class="-1"/></Journal></VisioBibleJournal>"#;
        let broken = decode_encoding(WINDOWS_1252, xml.as_bytes());
        assert_eq!(repair(&broken).as_deref(), Some(xml));
        let cp_xml = encode_strict(WINDOWS_1251, xml).unwrap();
        let latin_xml: String = cp_xml.iter().map(|byte| char::from(*byte)).collect();
        assert_eq!(repair(&latin_xml).as_deref(), Some(xml));
    }

    #[test]
    fn leaves_russian_german_french_and_mixed_text() {
        for sample in samples_left_alone() {
            assert!(repair(sample).is_none(), "{sample}");
        }
        let mixed = format!(
            "<Item Caption=\"{}\" Quote=\"ёлка\"/>",
            decode_encoding(WINDOWS_1252, "Привет".as_bytes())
        );
        assert!(repair(&mixed).is_none(), "{mixed}");
    }

    #[test]
    fn windows_1251_encode_refuses_unmappable() {
        let raw = encode_windows_1251("Привет").unwrap();
        assert_eq!(decode_1251(&raw).as_deref(), Some("Привет"));
        assert!(encode_windows_1251("café").is_err());
    }
}
