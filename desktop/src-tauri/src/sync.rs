use std::collections::{BTreeMap, BTreeSet};

use serde::{Deserialize, Serialize};

use crate::groups::{self, Applied};
use crate::plans::{self, PlanItem};
use crate::visio::{self, Group, Song};

/// Last synced text is the baseline. A side that still matches it did not change,
/// so the other side can be applied. If both differ, the operator chooses.
fn action_of(
    base: Option<&Song>,
    local: Option<&Song>,
    remote: Option<&Song>,
) -> Option<&'static str> {
    let base = base.filter(|song| !song.deleted);
    let local = local.filter(|song| !song.deleted);
    let remote = remote.filter(|song| !song.deleted);
    Some(match (local, remote) {
        (None, None) => return None,
        (Some(local), Some(remote)) if visio::song_eq(local, remote) => "unchanged",
        (Some(local), Some(remote)) => {
            if base.is_some_and(|base| visio::song_eq(base, local)) {
                "pull_update"
            } else if base.is_some_and(|base| visio::song_eq(base, remote)) {
                "push_update"
            } else {
                "conflict"
            }
        }
        (Some(local), None) => {
            if base.is_none() {
                "push_add"
            } else if base.is_some_and(|base| visio::song_eq(base, local)) {
                "pull_delete"
            } else {
                "conflict"
            }
        }
        (None, Some(remote)) => {
            if base.is_none() {
                "pull_add"
            } else if base.is_some_and(|base| visio::song_eq(base, remote)) {
                "push_delete"
            } else {
                "conflict"
            }
        }
    })
}

#[derive(Clone, Debug)]
pub struct BookIn {
    pub path: String,
    pub title: String,
    pub short_name: String,
    pub publisher: String,
    pub revision_date: String,
    pub comment: String,
    pub format_version: String,
    pub charset: String,
    pub flags: u32,
    pub header_short: String,
    pub header_title: String,
    pub original: Vec<Song>,
    pub songs: Vec<Song>,
    pub base: Vec<Song>,
    pub server_id: String,
    pub book_fbid: String,
    pub groups: Vec<Group>,
    pub base_groups: Vec<groups::Baseline>,
}

#[derive(Clone, Debug)]
pub struct RemoteBook {
    pub server_id: String,
    pub fbid: String,
    pub title: String,
    pub short_name: String,
    pub publisher: String,
    pub revision_date: String,
    pub comment: String,
    pub format_version: String,
    pub charset: String,
    pub flags: u32,
    pub header_short: String,
    pub header_title: String,
    pub songs: Vec<Song>,
    pub order: Vec<String>,
    pub sections: Vec<groups::Section>,
}

#[derive(Clone, Debug)]
pub struct PlanIn {
    pub key: String,
    pub path: String,
    pub title: String,
    pub format: String,
    pub items: Vec<PlanItem>,
    pub base: Vec<PlanItem>,
    pub server_id: String,
    pub dimension_id: String,
    pub version: i64,
    pub updated_at: String,
}

#[derive(Clone, Debug)]
pub struct Input {
    pub books: Vec<BookIn>,
    pub plans: Vec<PlanIn>,
    pub remote_books: Vec<RemoteBook>,
    pub remote_plans: Vec<crate::plans::RemotePlan>,
    pub modules_dir: String,
    pub plans_dir: String,
}

#[derive(Clone, Debug, Serialize)]
pub struct SongRow {
    pub book_path: String,
    pub book_title: String,
    pub fbid: String,
    pub title: String,
    pub action: String,
    pub local_title: String,
    pub local_lyrics: String,
    pub remote_title: String,
    pub remote_lyrics: String,
}

#[derive(Clone, Debug, Serialize)]
pub struct PlanRow {
    pub key: String,
    pub path: String,
    pub name: String,
    pub action: String,
    pub summary: String,
    pub local_summary: String,
    pub remote_summary: String,
}

#[derive(Clone, Debug, Serialize)]
pub struct GroupRow {
    pub book_path: String,
    pub book_title: String,
    pub section_id: String,
    pub name: String,
    pub action: String,
    pub local_name: String,
    pub remote_name: String,
}

#[derive(Clone, Debug, Serialize)]
pub struct Preview {
    pub songs: Vec<SongRow>,
    pub plans: Vec<PlanRow>,
    pub groups: Vec<GroupRow>,
    pub warnings: Vec<String>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct SongDecision {
    pub book_path: String,
    pub fbid: String,
    pub choice: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub lyrics: String,
}

#[derive(Clone, Debug, Deserialize)]
pub struct PlanDecision {
    pub key: String,
    pub choice: String,
}

#[derive(Clone, Debug, Deserialize)]
pub struct GroupDecision {
    pub book_path: String,
    pub section_id: String,
    pub choice: String,
}

#[derive(Clone, Debug)]
pub struct BookWork {
    pub path: String,
    pub title: String,
    pub short_name: String,
    pub publisher: String,
    pub revision_date: String,
    pub comment: String,
    pub format_version: String,
    pub charset: String,
    pub flags: u32,
    pub header_short: String,
    pub header_title: String,
    pub server_id: String,
    pub book_fbid: String,
    pub create: bool,
    pub write: bool,
    pub songs: Vec<Song>,
    pub push: Vec<Song>,
    pub order: Vec<String>,
    pub groups: Vec<Group>,
    pub group_ops: Vec<Applied>,
    pub group_state: Vec<Applied>,
}

#[derive(Clone, Debug)]
pub struct PlanWork {
    pub path: String,
    pub title: String,
    pub server_id: String,
    pub dimension_id: String,
    pub version: i64,
    pub updated_at: String,
    pub create: bool,
    pub write: bool,
    pub push: bool,
    pub items: Vec<PlanItem>,
}

#[derive(Clone, Debug)]
pub struct Outcome {
    pub books: Vec<BookWork>,
    pub plans: Vec<PlanWork>,
    pub warnings: Vec<String>,
}

struct Analysis {
    songs: Vec<SongRow>,
    plans: Vec<PlanRow>,
    groups: Vec<GroupRow>,
    warnings: Vec<String>,
    books: Vec<BookWork>,
    plan_work: Vec<PlanWork>,
}

struct SongFact {
    id: String,
    action: &'static str,
    local: Option<Song>,
    remote: Option<Song>,
    base: Option<Song>,
}

pub fn preview(input: &Input) -> Result<Preview, String> {
    let mut ids = String::new;
    let analysis = analyze(input, &[], &[], &[], false, &mut ids)?;
    Ok(Preview {
        songs: analysis.songs,
        plans: analysis.plans,
        groups: analysis.groups,
        warnings: analysis.warnings,
    })
}

pub fn apply(
    input: &Input,
    song_choices: &[SongDecision],
    plan_choices: &[PlanDecision],
    group_choices: &[GroupDecision],
    mut ids: impl FnMut() -> String,
) -> Result<Outcome, String> {
    let analysis = analyze(
        input,
        song_choices,
        plan_choices,
        group_choices,
        true,
        &mut ids,
    )?;
    Ok(Outcome {
        books: analysis.books,
        plans: analysis.plan_work,
        warnings: analysis.warnings,
    })
}

fn analyze(
    input: &Input,
    song_choices: &[SongDecision],
    plan_choices: &[PlanDecision],
    group_choices: &[GroupDecision],
    require: bool,
    ids: &mut impl FnMut() -> String,
) -> Result<Analysis, String> {
    let warnings = Vec::new();
    let mut rows = Vec::new();
    let mut group_rows = Vec::new();
    let mut book_work = Vec::new();
    let mut used_remote = BTreeSet::new();
    for book in &input.books {
        let remote = match_book(book, &input.remote_books, &mut used_remote);
        let facts = song_facts(book, remote)?;
        for fact in &facts {
            rows.push(song_row(book, fact));
        }
        if require {
            let mut work = book_work_from(book, remote, &facts, song_choices, ids)?;
            attach_groups(
                book,
                remote,
                Some(&mut work),
                &mut group_rows,
                group_choices,
                ids,
            )?;
            book_work.push(work);
        } else {
            attach_groups(book, remote, None, &mut group_rows, group_choices, ids)?;
        }
    }
    for remote in &input.remote_books {
        if used_remote.contains(&remote.server_id) || input.modules_dir.is_empty() {
            continue;
        }
        let book = remote_as_local(remote, &input.modules_dir);
        let facts = song_facts(&book, Some(remote))?;
        for fact in &facts {
            rows.push(song_row(&book, fact));
        }
        if require {
            let mut work = book_work_from(&book, Some(remote), &facts, song_choices, ids)?;
            attach_groups(
                &book,
                Some(remote),
                Some(&mut work),
                &mut group_rows,
                group_choices,
                ids,
            )?;
            book_work.push(work);
        } else {
            attach_groups(
                &book,
                Some(remote),
                None,
                &mut group_rows,
                group_choices,
                ids,
            )?;
        }
    }
    let mut plan_rows = Vec::new();
    let mut plan_work = Vec::new();
    let mut used_plans = BTreeSet::new();
    for plan in &input.plans {
        let remote = match_plan(plan, &input.remote_plans, &mut used_plans);
        let action = plan_action(plan, remote);
        plan_rows.push(plan_row(plan, remote, action));
        if require {
            plan_work.push(plan_work_from(plan, remote, action, plan_choices)?);
        }
    }
    for remote in &input.remote_plans {
        if used_plans.contains(&remote.id) || remote.deleted || input.plans_dir.is_empty() {
            continue;
        }
        let plan = remote_as_plan(remote, &input.plans_dir);
        let action = plan_action(&plan, Some(remote));
        plan_rows.push(plan_row(&plan, Some(remote), action));
        if require {
            plan_work.push(plan_work_from(&plan, Some(remote), action, plan_choices)?);
        }
    }
    Ok(Analysis {
        songs: rows,
        plans: plan_rows,
        groups: group_rows,
        warnings,
        books: book_work,
        plan_work,
    })
}

fn attach_groups(
    book: &BookIn,
    remote: Option<&RemoteBook>,
    mut work: Option<&mut BookWork>,
    rows: &mut Vec<GroupRow>,
    choices: &[GroupDecision],
    ids: &mut impl FnMut() -> String,
) -> Result<(), String> {
    let allowed = if let Some(work) = work.as_ref() {
        alive(&work.songs)
    } else {
        let mut ids = alive(&book.songs);
        if let Some(remote) = remote {
            ids.extend(alive(&remote.songs));
        }
        ids
    };
    let local = sides_from(book, &allowed);
    let sections = remote
        .map(|remote| filter_sections(&remote.sections, &allowed))
        .unwrap_or_default();
    let choices: Vec<groups::Choice> = choices
        .iter()
        .filter(|choice| choice.book_path == book.path)
        .map(|choice| groups::Choice {
            section_id: choice.section_id.clone(),
            choice: choice.choice.clone(),
        })
        .collect();
    let applied = groups::merge(
        &local,
        &sections,
        &book.base_groups,
        &choices,
        work.is_some(),
    )
    .map_err(|err| format!("Сборник «{}»: {err}", book.title))?;
    for row in &applied {
        rows.push(GroupRow {
            book_path: book.path.clone(),
            book_title: book.title.clone(),
            section_id: row.section_id.clone(),
            name: row.name.clone(),
            action: row.action.to_string(),
            local_name: row.local_name.clone(),
            remote_name: row.remote_name.clone(),
        });
    }
    if let Some(work) = work.as_mut() {
        let file_groups = file_groups(&applied, &work.songs);
        if semantic(&book.groups, &book.songs) != semantic_applied(&applied) {
            work.write = true;
        }
        work.groups = file_groups;
        work.group_ops = applied
            .iter()
            .filter(|row| row.create || row.rename || row.push_songs)
            .cloned()
            .collect();
        work.group_state = applied;
        if work.server_id.is_empty()
            && work
                .group_ops
                .iter()
                .any(|row| row.create || row.rename || row.push_songs)
        {
            if work.book_fbid.is_empty() {
                work.book_fbid = ids();
            }
            work.create = true;
        }
    }
    Ok(())
}

fn alive(songs: &[Song]) -> BTreeSet<String> {
    songs
        .iter()
        .filter(|song| !song.deleted && !song.fbid.is_empty())
        .map(|song| song.fbid.clone())
        .collect()
}

fn sides_from(book: &BookIn, allowed: &BTreeSet<String>) -> Vec<groups::Side> {
    book.groups
        .iter()
        .map(|group| {
            let mut songs = Vec::new();
            for index in &group.songs {
                let Some(song) = book.songs.get(*index as usize) else {
                    continue;
                };
                if allowed.contains(&song.fbid) && !songs.contains(&song.fbid) {
                    songs.push(song.fbid.clone());
                }
            }
            groups::Side {
                name: group.name.clone(),
                songs,
            }
        })
        .collect()
}

fn filter_sections(
    sections: &[groups::Section],
    allowed: &BTreeSet<String>,
) -> Vec<groups::Section> {
    sections
        .iter()
        .map(|section| groups::Section {
            id: section.id.clone(),
            name: section.name.clone(),
            songs: section
                .songs
                .iter()
                .filter(|id| allowed.contains(*id))
                .cloned()
                .collect(),
        })
        .collect()
}

fn file_groups(applied: &[Applied], songs: &[Song]) -> Vec<Group> {
    let mut at = BTreeMap::new();
    for (index, song) in songs.iter().enumerate() {
        if !song.deleted && !song.fbid.is_empty() {
            at.entry(song.fbid.clone()).or_insert(index as u32);
        }
    }
    applied
        .iter()
        .map(|row| Group {
            name: row.name.clone(),
            songs: row
                .songs
                .iter()
                .filter_map(|id| at.get(id).copied())
                .collect(),
        })
        .collect()
}

fn semantic(groups: &[Group], songs: &[Song]) -> Vec<(String, Vec<String>)> {
    groups
        .iter()
        .map(|group| {
            let mut ids = Vec::new();
            for index in &group.songs {
                let Some(song) = songs.get(*index as usize) else {
                    continue;
                };
                if !song.fbid.is_empty() && !ids.contains(&song.fbid) {
                    ids.push(song.fbid.clone());
                }
            }
            (group.name.trim().to_string(), ids)
        })
        .collect()
}

fn semantic_applied(applied: &[Applied]) -> Vec<(String, Vec<String>)> {
    applied
        .iter()
        .map(|row| (row.name.clone(), row.songs.clone()))
        .collect()
}

fn song_facts(book: &BookIn, remote: Option<&RemoteBook>) -> Result<Vec<SongFact>, String> {
    let local = index_songs(&book.songs)?;
    let base = index_songs(&book.base).unwrap_or_default();
    let remote_songs = remote
        .map(|book| index_songs(&book.songs).unwrap_or_default())
        .unwrap_or_default();
    let mut ids = Vec::new();
    let mut seen = BTreeSet::new();
    for list in [
        &book.songs,
        remote.map(|book| &book.songs).unwrap_or(&book.songs),
        &book.base,
    ] {
        for song in list {
            if song.fbid.is_empty() || !seen.insert(song.fbid.clone()) {
                continue;
            }
            ids.push(song.fbid.clone());
        }
    }
    let mut facts = Vec::new();
    for id in ids {
        let local = local.get(&id);
        let remote = remote_songs.get(&id);
        let base = base.get(&id);
        let Some(action) = action_of(base, local, remote) else {
            continue;
        };
        facts.push(SongFact {
            id,
            action,
            local: local.cloned(),
            remote: remote.cloned(),
            base: base.cloned(),
        });
    }
    Ok(facts)
}

fn index_songs(songs: &[Song]) -> Result<std::collections::BTreeMap<String, Song>, String> {
    let mut map = std::collections::BTreeMap::new();
    for song in songs {
        if song.fbid.is_empty() {
            continue;
        }
        if map.insert(song.fbid.clone(), song.clone()).is_some() {
            return Err("В одном сборнике две песни с одним номером".into());
        }
    }
    Ok(map)
}

fn song_row(book: &BookIn, fact: &SongFact) -> SongRow {
    let title = fact
        .local
        .as_ref()
        .or(fact.remote.as_ref())
        .or(fact.base.as_ref())
        .map(|song| song.title.clone())
        .filter(|title| !title.is_empty())
        .unwrap_or_else(|| "Песня".into());
    SongRow {
        book_path: book.path.clone(),
        book_title: book.title.clone(),
        fbid: fact.id.clone(),
        title,
        action: fact.action.to_string(),
        local_title: fact
            .local
            .as_ref()
            .map(|song| song.title.clone())
            .unwrap_or_default(),
        local_lyrics: fact.local.as_ref().map(visio::lyrics).unwrap_or_default(),
        remote_title: fact
            .remote
            .as_ref()
            .map(|song| song.title.clone())
            .unwrap_or_default(),
        remote_lyrics: fact.remote.as_ref().map(visio::lyrics).unwrap_or_default(),
    }
}

fn book_work_from(
    book: &BookIn,
    remote: Option<&RemoteBook>,
    facts: &[SongFact],
    choices: &[SongDecision],
    ids: &mut impl FnMut() -> String,
) -> Result<BookWork, String> {
    let mut result = Vec::new();
    let mut push = Vec::new();
    for fact in facts {
        let choice = choices
            .iter()
            .find(|choice| choice.book_path == book.path && choice.fbid == fact.id);
        match fact.action {
            "unchanged" => {
                if let Some(song) = fact.local.clone().or(fact.remote.clone()) {
                    if !song.deleted {
                        result.push(song);
                    }
                }
            }
            "push_add" | "push_update" => {
                let song = fact.local.clone().ok_or("нет песни")?;
                push.push(song.clone());
                result.push(song);
            }
            "pull_add" | "pull_update" => {
                let mut song = fact.remote.clone().ok_or("нет песни")?;
                song.deleted = false;
                result.push(song);
            }
            "pull_delete" => {}
            "push_delete" => {
                let mut song = fact
                    .base
                    .clone()
                    .or(fact.local.clone())
                    .ok_or("нет песни")?;
                song.deleted = true;
                push.push(song);
            }
            "conflict" => {
                let choice = choice.ok_or("Нужно выбрать, чью версию оставить")?;
                resolve_conflict(fact, choice, &mut result, &mut push, ids)?;
            }
            _ => {}
        }
    }
    // The snapshot lists songs by fbid. Display order is `order`.
    // A song the list does not name (local-only, or a "keep both" copy)
    // keeps its current relative place after the named songs.
    let result = if let Some(remote) = remote {
        apply_server_order(result, &remote.order)
    } else {
        result
    };
    let server_id = remote
        .map(|book| book.server_id.clone())
        .unwrap_or_else(|| book.server_id.clone());
    let book_fbid = if book.book_fbid.is_empty() {
        remote.map(|book| book.fbid.clone()).unwrap_or_default()
    } else {
        book.book_fbid.clone()
    };
    let book_fbid = if server_id.is_empty() && !push.is_empty() && book_fbid.is_empty() {
        ids()
    } else {
        book_fbid
    };
    Ok(BookWork {
        path: book.path.clone(),
        title: book.title.clone(),
        short_name: book.short_name.clone(),
        publisher: book.publisher.clone(),
        revision_date: book.revision_date.clone(),
        comment: book.comment.clone(),
        format_version: if book.format_version.is_empty() {
            "0x102".into()
        } else {
            book.format_version.clone()
        },
        charset: if book.charset.is_empty() {
            "0x4b0".into()
        } else {
            book.charset.clone()
        },
        flags: book.flags,
        header_short: book.header_short.clone(),
        header_title: book.header_title.clone(),
        server_id,
        book_fbid,
        create: remote.is_none() && book.server_id.is_empty() && !push.is_empty(),
        write: !visio::same_on_disk(&result, &book.original),
        order: result.iter().map(|song| song.fbid.clone()).collect(),
        songs: result,
        push,
        groups: Vec::new(),
        group_ops: Vec::new(),
        group_state: Vec::new(),
    })
}

fn apply_server_order(songs: Vec<Song>, order: &[String]) -> Vec<Song> {
    if order.is_empty() {
        return songs;
    }
    let mut used = vec![false; songs.len()];
    let mut out = Vec::with_capacity(songs.len());
    for id in order {
        if id.is_empty() {
            continue;
        }
        let Some(index) = songs
            .iter()
            .enumerate()
            .position(|(index, song)| !used[index] && song.fbid == *id)
        else {
            continue;
        };
        used[index] = true;
        out.push(songs[index].clone());
    }
    for (index, song) in songs.into_iter().enumerate() {
        if !used[index] {
            out.push(song);
        }
    }
    out
}

fn resolve_conflict(
    fact: &SongFact,
    choice: &SongDecision,
    result: &mut Vec<Song>,
    push: &mut Vec<Song>,
    ids: &mut impl FnMut() -> String,
) -> Result<(), String> {
    match choice.choice.as_str() {
        "keep_local" => {
            if let Some(song) = fact.local.clone().filter(|song| !song.deleted) {
                push.push(song.clone());
                result.push(song);
            } else if let Some(mut song) = fact.base.clone().or(fact.remote.clone()) {
                song.deleted = true;
                push.push(song);
            }
        }
        "take_server" => {
            if let Some(mut song) = fact.remote.clone().filter(|song| !song.deleted) {
                song.deleted = false;
                result.push(song);
            }
        }
        "keep_both" => {
            if let Some(mut song) = fact.remote.clone().filter(|song| !song.deleted) {
                song.deleted = false;
                result.push(song);
            }
            if let Some(local) = fact.local.clone().filter(|song| !song.deleted) {
                let copy = visio::with_fbid(&local, &ids());
                push.push(copy.clone());
                result.push(copy);
            }
        }
        "manual" => {
            let seed = fact
                .local
                .clone()
                .or(fact.remote.clone())
                .or(fact.base.clone())
                .ok_or("нет песни")?;
            let song = visio::song_from_manual(&seed, &choice.title, &choice.lyrics);
            push.push(song.clone());
            result.push(song);
        }
        _ => return Err("Нужно выбрать, чью версию оставить".into()),
    }
    Ok(())
}

fn match_book<'a>(
    book: &BookIn,
    remotes: &'a [RemoteBook],
    used: &mut BTreeSet<String>,
) -> Option<&'a RemoteBook> {
    if !book.server_id.is_empty() {
        if let Some(found) = remotes
            .iter()
            .find(|remote| remote.server_id == book.server_id)
        {
            used.insert(found.server_id.clone());
            return Some(found);
        }
    }
    let hits: Vec<_> = remotes
        .iter()
        .filter(|remote| {
            !used.contains(&remote.server_id)
                && !book.short_name.is_empty()
                && remote.short_name == book.short_name
        })
        .collect();
    if hits.len() == 1 {
        used.insert(hits[0].server_id.clone());
        return Some(hits[0]);
    }
    None
}

fn remote_as_local(remote: &RemoteBook, modules_dir: &str) -> BookIn {
    let stem = plans::file_stem(if remote.short_name.is_empty() {
        &remote.title
    } else {
        &remote.short_name
    });
    BookIn {
        path: format!("{modules_dir}/{stem}.vbm"),
        title: remote.title.clone(),
        short_name: remote.short_name.clone(),
        publisher: remote.publisher.clone(),
        revision_date: remote.revision_date.clone(),
        comment: remote.comment.clone(),
        format_version: remote.format_version.clone(),
        charset: remote.charset.clone(),
        flags: remote.flags,
        header_short: remote.header_short.clone(),
        header_title: remote.header_title.clone(),
        original: Vec::new(),
        songs: Vec::new(),
        base: Vec::new(),
        server_id: remote.server_id.clone(),
        book_fbid: remote.fbid.clone(),
        groups: Vec::new(),
        base_groups: Vec::new(),
    }
}

fn plan_action(plan: &PlanIn, remote: Option<&crate::plans::RemotePlan>) -> &'static str {
    let local = plans::project(&plan.items, &plan.format);
    let base = plans::project(&plan.base, &plan.format);
    let remote_items = remote
        .map(|plan| plans::project(&plan.items, "json"))
        .unwrap_or_default();
    let remote_items = if plan.format == "vplt" {
        plans::project(
            &remote.map(|plan| plan.items.clone()).unwrap_or_default(),
            "vplt",
        )
    } else {
        remote_items
    };
    let remote_present = remote.filter(|plan| !plan.deleted);
    match (local.is_empty(), remote_present) {
        (true, None) => "unchanged",
        (false, None) if plan.base.is_empty() => "push_add",
        (false, None) => "push_update",
        (true, Some(_)) if plan.base.is_empty() => "pull_add",
        (_, Some(remote_plan)) => {
            let same_remote = local == remote_items;
            if same_remote {
                "unchanged"
            } else if base == local {
                if plan.server_id.is_empty() && plan.base.is_empty() {
                    "pull_add"
                } else {
                    "pull_update"
                }
            } else if base == remote_items || (plan.base.is_empty() && remote_plan.items.is_empty())
            {
                "push_update"
            } else {
                "conflict"
            }
        }
    }
}

fn plan_row(plan: &PlanIn, remote: Option<&crate::plans::RemotePlan>, action: &str) -> PlanRow {
    PlanRow {
        key: plan.key.clone(),
        path: plan.path.clone(),
        name: if plan.title.is_empty() {
            remote
                .map(|plan| plan.title.clone())
                .unwrap_or_else(|| "План".into())
        } else {
            plan.title.clone()
        },
        action: action.to_string(),
        summary: plans::summarize(&plan.items),
        local_summary: plans::summarize(&plan.items),
        remote_summary: remote
            .map(|plan| plans::summarize(&plan.items))
            .unwrap_or_else(|| "на сервере плана нет".into()),
    }
}

fn plan_work_from(
    plan: &PlanIn,
    remote: Option<&crate::plans::RemotePlan>,
    action: &str,
    choices: &[PlanDecision],
) -> Result<PlanWork, String> {
    let choice = choices.iter().find(|choice| choice.key == plan.key);
    let (write, push, items) = match action {
        "unchanged" => (false, false, plan.items.clone()),
        "push_add" | "push_update" => (
            false,
            true,
            plans::items_for_push(&plan.format, &plan.base, &plan.items),
        ),
        "pull_add" | "pull_update" => (
            true,
            false,
            remote.map(|plan| plan.items.clone()).unwrap_or_default(),
        ),
        "conflict" => {
            let choice = choice.ok_or("Нужно выбрать, чью версию плана оставить")?;
            match choice.choice.as_str() {
                "keep_local" => (
                    false,
                    true,
                    plans::items_for_push(&plan.format, &plan.base, &plan.items),
                ),
                "take_server" => (
                    true,
                    false,
                    remote.map(|plan| plan.items.clone()).unwrap_or_default(),
                ),
                "keep_both" => {
                    let mut items = plan.items.clone();
                    if let Some(remote) = remote {
                        for item in &remote.items {
                            if !items.contains(item) {
                                items.push(item.clone());
                            }
                        }
                    }
                    (true, true, items)
                }
                _ => return Err("Нужно выбрать, чью версию плана оставить".into()),
            }
        }
        _ => (false, false, plan.items.clone()),
    };
    let server_id = remote
        .map(|plan| plan.id.clone())
        .filter(|id| !id.is_empty())
        .unwrap_or_else(|| plan.server_id.clone());
    Ok(PlanWork {
        path: plan.path.clone(),
        title: if plan.title.is_empty() {
            remote.map(|plan| plan.title.clone()).unwrap_or_default()
        } else {
            plan.title.clone()
        },
        server_id: server_id.clone(),
        dimension_id: plan.dimension_id.clone(),
        version: remote.map(|plan| plan.version).unwrap_or(plan.version),
        updated_at: remote
            .map(|plan| plan.updated_at.clone())
            .unwrap_or_else(|| plan.updated_at.clone()),
        create: push && server_id.is_empty(),
        write,
        push,
        items,
    })
}

fn match_plan<'a>(
    plan: &PlanIn,
    remotes: &'a [crate::plans::RemotePlan],
    used: &mut BTreeSet<String>,
) -> Option<&'a crate::plans::RemotePlan> {
    if !plan.server_id.is_empty() {
        if let Some(found) = remotes.iter().find(|remote| remote.id == plan.server_id) {
            used.insert(found.id.clone());
            return Some(found);
        }
    }
    let hits: Vec<_> = remotes
        .iter()
        .filter(|remote| {
            !used.contains(&remote.id)
                && !remote.deleted
                && !plan.title.is_empty()
                && remote.title == plan.title
        })
        .collect();
    if hits.len() == 1 {
        used.insert(hits[0].id.clone());
        return Some(hits[0]);
    }
    None
}

fn remote_as_plan(remote: &crate::plans::RemotePlan, plans_dir: &str) -> PlanIn {
    let stem = plans::file_stem(&remote.title);
    PlanIn {
        key: format!("remote:{}", remote.id),
        path: format!("{plans_dir}/{stem}.vplt"),
        title: remote.title.clone(),
        format: "vplt".into(),
        items: Vec::new(),
        base: Vec::new(),
        server_id: remote.id.clone(),
        dimension_id: remote.dimension_id.clone().unwrap_or_default(),
        version: remote.version,
        updated_at: remote.updated_at.clone(),
    }
}

pub fn percentage(completed: u32, total: u32) -> u32 {
    completed
        .saturating_mul(100)
        .checked_div(total)
        .unwrap_or(100)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::visio::Part;

    fn song(id: &str, title: &str, text: &str) -> Song {
        Song {
            fbid: id.into(),
            title: title.into(),
            properties: format!("$FBID$={id}\n"),
            parts: vec![Part {
                kind: "Куплет".into(),
                text: text.into(),
                align: 0,
            }],
            ..Song::default()
        }
    }

    fn book(songs: Vec<Song>, base: Vec<Song>) -> BookIn {
        BookIn {
            path: "book.vbm".into(),
            title: "Сборник".into(),
            short_name: "SYNTH".into(),
            publisher: String::new(),
            revision_date: String::new(),
            comment: String::new(),
            format_version: "0x102".into(),
            charset: "0x4b0".into(),
            flags: 0,
            header_short: "FB".into(),
            header_title: "Test".into(),
            original: songs.clone(),
            songs,
            base,
            server_id: "server-book".into(),
            book_fbid: "book".into(),
            groups: Vec::new(),
            base_groups: Vec::new(),
        }
    }

    fn remote(songs: Vec<Song>) -> RemoteBook {
        RemoteBook {
            server_id: "server-book".into(),
            fbid: "book".into(),
            title: "Сборник".into(),
            short_name: "SYNTH".into(),
            publisher: String::new(),
            revision_date: String::new(),
            comment: String::new(),
            format_version: "0x102".into(),
            charset: "0x4b0".into(),
            flags: 0,
            header_short: "FB".into(),
            header_title: "Test".into(),
            songs,
            order: Vec::new(),
            sections: Vec::new(),
        }
    }

    const A: &str = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
    const B: &str = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
    const C: &str = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
    const D: &str = "dddddddd-dddd-4ddd-8ddd-dddddddddddd";

    #[test]
    fn three_way_song_plan_lists_every_bucket() {
        let base_a = song(A, "А", "было");
        let base_b = song(B, "Б", "общее");
        let local = vec![song(A, "А", "моё"), base_b.clone(), song(C, "В", "новое")];
        let remote_songs = vec![
            song(A, "А", "сервер"),
            base_b.clone(),
            song(D, "Г", "оттуда"),
        ];
        let input = Input {
            books: vec![book(local, vec![base_a, base_b])],
            plans: Vec::new(),
            remote_books: vec![remote(remote_songs)],
            remote_plans: Vec::new(),
            modules_dir: "/modules".into(),
            plans_dir: "/plans".into(),
        };
        let preview = preview(&input).unwrap();
        let action = |id: &str| {
            preview
                .songs
                .iter()
                .find(|row| row.fbid == id)
                .unwrap()
                .action
                .clone()
        };
        assert_eq!(action(A), "conflict");
        assert_eq!(action(B), "unchanged");
        assert_eq!(action(C), "push_add");
        assert_eq!(action(D), "pull_add");
    }

    #[test]
    fn resolutions_keep_local_server_or_both() {
        let base = song(A, "А", "было");
        let local = vec![song(A, "А", "моё")];
        let remote_songs = vec![song(A, "А", "сервер")];
        let input = Input {
            books: vec![book(local, vec![base])],
            plans: Vec::new(),
            remote_books: vec![remote(remote_songs)],
            remote_plans: Vec::new(),
            modules_dir: String::new(),
            plans_dir: String::new(),
        };
        let err = apply(&input, &[], &[], &[], String::new).unwrap_err();
        assert!(err.contains("выбрать"));
        let keep = apply(
            &input,
            &[SongDecision {
                book_path: "book.vbm".into(),
                fbid: A.into(),
                choice: "keep_local".into(),
                title: String::new(),
                lyrics: String::new(),
            }],
            &[],
            &[],
            String::new,
        )
        .unwrap();
        assert_eq!(keep.books[0].push[0].parts[0].text, "моё");
        assert_eq!(keep.books[0].songs[0].parts[0].text, "моё");
        let take = apply(
            &input,
            &[SongDecision {
                book_path: "book.vbm".into(),
                fbid: A.into(),
                choice: "take_server".into(),
                title: String::new(),
                lyrics: String::new(),
            }],
            &[],
            &[],
            String::new,
        )
        .unwrap();
        assert!(take.books[0].push.is_empty());
        assert_eq!(take.books[0].songs[0].parts[0].text, "сервер");
        let both = apply(
            &input,
            &[SongDecision {
                book_path: "book.vbm".into(),
                fbid: A.into(),
                choice: "keep_both".into(),
                title: String::new(),
                lyrics: String::new(),
            }],
            &[],
            &[],
            || "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee".into(),
        )
        .unwrap();
        assert_eq!(both.books[0].songs.len(), 2);
        assert_eq!(both.books[0].songs[0].fbid, A);
        assert_eq!(
            both.books[0].songs[1].fbid,
            "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
        );
        assert_eq!(both.books[0].push.len(), 1);
        let manual = apply(
            &input,
            &[SongDecision {
                book_path: "book.vbm".into(),
                fbid: A.into(),
                choice: "manual".into(),
                title: "Своя".into(),
                lyrics: "текст".into(),
            }],
            &[],
            &[],
            String::new,
        )
        .unwrap();
        assert_eq!(manual.books[0].songs[0].title, "Своя");
        assert_eq!(manual.books[0].songs[0].parts[0].text, "текст");
    }

    #[test]
    fn delete_against_edit_is_a_conflict_and_plain_edits_are_not() {
        let base = song(A, "А", "было");
        let local = vec![song(A, "А", "моё")];
        let input = Input {
            books: vec![book(local, vec![base.clone()])],
            plans: Vec::new(),
            remote_books: vec![remote(vec![Song {
                deleted: true,
                ..base.clone()
            }])],
            remote_plans: Vec::new(),
            modules_dir: String::new(),
            plans_dir: String::new(),
        };
        let shown = preview(&input).unwrap();
        assert_eq!(shown.songs[0].action, "conflict");

        let only_local = book(vec![song(A, "А", "моё")], vec![song(A, "А", "было")]);
        let same_remote = remote(vec![song(A, "А", "было")]);
        let input = Input {
            books: vec![only_local],
            remote_books: vec![same_remote],
            ..empty_input()
        };
        assert_eq!(preview(&input).unwrap().songs[0].action, "push_update");

        let only_remote = book(vec![song(A, "А", "было")], vec![song(A, "А", "было")]);
        let changed = remote(vec![song(A, "А", "сервер")]);
        let input = Input {
            books: vec![only_remote],
            remote_books: vec![changed],
            ..empty_input()
        };
        assert_eq!(preview(&input).unwrap().songs[0].action, "pull_update");
    }

    fn empty_input() -> Input {
        Input {
            books: Vec::new(),
            plans: Vec::new(),
            remote_books: Vec::new(),
            remote_plans: Vec::new(),
            modules_dir: String::new(),
            plans_dir: String::new(),
        }
    }

    #[test]
    fn plan_push_and_pull_follow_the_same_three_way_rule() {
        let item = PlanItem {
            kind: "text".into(),
            caption: "Мир".into(),
            quote: "текст".into(),
            class: "-1".into(),
            ..PlanItem::default()
        };
        let mut changed = item.clone();
        changed.quote = "другой".into();
        let local = PlanIn {
            key: "plan.vplt".into(),
            path: "plan.vplt".into(),
            title: "Воскресенье".into(),
            format: "vplt".into(),
            items: vec![changed.clone()],
            base: vec![item.clone()],
            server_id: "plan-1".into(),
            dimension_id: String::new(),
            version: 1,
            updated_at: "2026-11-20T00:00:00Z".into(),
        };
        let remote_plan = crate::plans::RemotePlan {
            id: "plan-1".into(),
            title: "Воскресенье".into(),
            version: 2,
            updated_at: "2026-11-20T01:00:00Z".into(),
            items: vec![item.clone()],
            ..Default::default()
        };
        let input = Input {
            plans: vec![local],
            remote_plans: vec![remote_plan],
            ..empty_input()
        };
        let row = &preview(&input).unwrap().plans[0];
        assert_eq!(row.action, "push_update");
        let outcome = apply(&input, &[], &[], &[], String::new).unwrap();
        assert!(outcome.plans[0].push);
        assert_eq!(outcome.plans[0].version, 2);
    }

    #[test]
    fn written_songs_follow_snapshot_order_and_local_only_songs_stay_after() {
        let base_a = song(A, "А", "было");
        let base_b = song(B, "Б", "было");
        let local = vec![
            song(A, "А", "моё"),
            song(C, "В", "своя"),
            song(D, "Г", "ещё своя"),
            song(B, "Б", "тоже моё"),
        ];
        let mut remote_book = remote(vec![base_a.clone(), base_b.clone()]);
        remote_book.order = vec![B.into(), "missing".into(), A.into()];
        let input = Input {
            books: vec![book(local, vec![base_a, base_b])],
            plans: Vec::new(),
            remote_books: vec![remote_book],
            remote_plans: Vec::new(),
            modules_dir: String::new(),
            plans_dir: String::new(),
        };
        let outcome = apply(&input, &[], &[], &[], String::new).unwrap();
        let ids = |songs: &[Song]| {
            songs
                .iter()
                .map(|song| song.fbid.clone())
                .collect::<Vec<_>>()
        };
        assert_eq!(
            ids(&outcome.books[0].songs),
            vec![B.to_string(), A.to_string(), C.to_string(), D.to_string()]
        );
        assert_eq!(outcome.books[0].order, ids(&outcome.books[0].songs));
        assert_eq!(
            ids(&outcome.books[0].push),
            vec![A.to_string(), C.to_string(), D.to_string(), B.to_string()]
        );
    }

    #[test]
    fn pull_of_a_new_book_writes_snapshot_order_not_fbid_order() {
        let mut remote_book = remote(vec![
            song(A, "А", "1"),
            song(B, "Б", "2"),
            song(C, "В", "3"),
        ]);
        remote_book.order = vec![C.into(), A.into(), B.into()];
        let input = Input {
            books: Vec::new(),
            plans: Vec::new(),
            remote_books: vec![remote_book],
            remote_plans: Vec::new(),
            modules_dir: "/modules".into(),
            plans_dir: String::new(),
        };
        let outcome = apply(&input, &[], &[], &[], String::new).unwrap();
        let ids: Vec<_> = outcome.books[0]
            .songs
            .iter()
            .map(|song| song.fbid.as_str())
            .collect();
        assert_eq!(ids, vec![C, A, B]);
        assert!(outcome.books[0].push.is_empty());
        assert_eq!(
            outcome.books[0].order,
            vec![C.to_string(), A.to_string(), B.to_string()]
        );
    }

    #[test]
    fn empty_snapshot_order_keeps_the_local_sequence() {
        let local = vec![song(B, "Б", "текст"), song(A, "А", "текст")];
        let input = Input {
            books: vec![book(local.clone(), local)],
            plans: Vec::new(),
            remote_books: vec![remote(vec![song(A, "А", "текст"), song(B, "Б", "текст")])],
            remote_plans: Vec::new(),
            modules_dir: String::new(),
            plans_dir: String::new(),
        };
        let outcome = apply(&input, &[], &[], &[], String::new).unwrap();
        let ids: Vec<_> = outcome.books[0]
            .songs
            .iter()
            .map(|song| song.fbid.as_str())
            .collect();
        assert_eq!(ids, vec![B, A]);
    }

    #[test]
    fn groups_union_by_name_and_conflict_only_when_both_renamed() {
        let song_a = song(A, "А", "текст");
        let song_b = song(B, "Б", "текст");
        let mut local = book(
            vec![song_a.clone(), song_b.clone()],
            vec![song_a.clone(), song_b.clone()],
        );
        local.groups = vec![Group {
            name: "Хор А".into(),
            songs: vec![0],
        }];
        local.base_groups = vec![groups::Baseline {
            section_id: "sec-1".into(),
            name: "Хор".into(),
            songs: vec![A.into()],
        }];
        let mut remote_book = remote(vec![song_a, song_b]);
        remote_book.sections = vec![groups::Section {
            id: "sec-1".into(),
            name: "Хор Б".into(),
            songs: vec![A.into(), B.into()],
        }];
        let input = Input {
            books: vec![local],
            remote_books: vec![remote_book],
            ..empty_input()
        };
        let shown = preview(&input).unwrap();
        assert!(shown.warnings.is_empty());
        assert_eq!(shown.groups[0].action, "conflict");
        assert_eq!(shown.groups[0].local_name, "Хор А");
        assert_eq!(shown.groups[0].remote_name, "Хор Б");
        assert!(apply(&input, &[], &[], &[], String::new).is_err());
        let outcome = apply(
            &input,
            &[],
            &[],
            &[GroupDecision {
                book_path: "book.vbm".into(),
                section_id: "sec-1".into(),
                choice: "keep_local".into(),
            }],
            String::new,
        )
        .unwrap();
        assert!(outcome.books[0].write);
        assert_eq!(outcome.books[0].groups[0].name, "Хор А");
        assert_eq!(outcome.books[0].groups[0].songs, vec![0, 1]);
        assert!(outcome.books[0].group_ops[0].rename);
        assert_eq!(
            outcome.books[0].group_state[0].songs,
            vec![A.to_string(), B.to_string()]
        );

        let mut same = book(vec![song(A, "А", "текст")], vec![song(A, "А", "текст")]);
        same.groups = vec![Group {
            name: "Хор".into(),
            songs: vec![0],
        }];
        same.base_groups = vec![groups::Baseline {
            section_id: "sec-1".into(),
            name: "Хор".into(),
            songs: vec![A.into()],
        }];
        let mut server = remote(vec![song(A, "А", "текст"), song(B, "Б", "текст")]);
        server.sections = vec![groups::Section {
            id: "sec-1".into(),
            name: "Хор".into(),
            songs: vec![B.into()],
        }];
        let input = Input {
            books: vec![same],
            remote_books: vec![server],
            ..empty_input()
        };
        let row = &preview(&input).unwrap().groups[0];
        assert_eq!(row.action, "push_update");
        assert_ne!(row.action, "conflict");
    }

    #[test]
    fn percentage_moves_with_completed_work() {
        assert_eq!(percentage(0, 4), 0);
        assert_eq!(percentage(1, 4), 25);
        assert_eq!(percentage(4, 4), 100);
        assert_eq!(percentage(0, 0), 100);
    }
}
