//! VisioBible groups and server sections are the same lists.
//! A shared name is one group. Songs merge as a union.
//! The only conflict is the same group renamed to two different titles
//! since the last sync.

use std::collections::{BTreeMap, BTreeSet};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Side {
    pub name: String,
    pub songs: Vec<String>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Section {
    pub id: String,
    pub name: String,
    pub songs: Vec<String>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Baseline {
    pub section_id: String,
    pub name: String,
    pub songs: Vec<String>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Choice {
    pub section_id: String,
    pub choice: String,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Applied {
    pub action: &'static str,
    pub section_id: String,
    pub name: String,
    pub local_name: String,
    pub remote_name: String,
    pub songs: Vec<String>,
    pub create: bool,
    pub rename: bool,
    pub push_songs: bool,
    pub write_local: bool,
}

struct Local {
    name: String,
    songs: Vec<String>,
    set: BTreeSet<String>,
}

struct Remote {
    id: String,
    name: String,
    songs: Vec<String>,
    set: BTreeSet<String>,
}

struct Base {
    section_id: String,
    name: String,
    set: BTreeSet<String>,
}

enum How {
    Name,
    Identity,
}

struct Pair {
    local: usize,
    remote: usize,
    how: How,
}

pub fn merge(
    local: &[Side],
    remote: &[Section],
    baseline: &[Baseline],
    choices: &[Choice],
    require: bool,
) -> Result<Vec<Applied>, String> {
    let local = normalize_local(local)?;
    let remote = normalize_remote(remote)?;
    let baseline = normalize_base(baseline);
    let mut used_local = vec![false; local.len()];
    let mut used_remote = vec![false; remote.len()];
    let mut pairs: Vec<Pair> = Vec::new();

    for (li, group) in local.iter().enumerate() {
        if let Some(ri) = remote.iter().position(|section| section.name == group.name) {
            take(
                &mut pairs,
                &mut used_local,
                &mut used_remote,
                li,
                ri,
                How::Name,
            );
        }
    }
    for base in &baseline {
        let Some(ri) = remote
            .iter()
            .position(|section| section.id == base.section_id)
        else {
            continue;
        };
        if used_remote[ri] {
            continue;
        }
        let Some(li) = local
            .iter()
            .enumerate()
            .find(|(index, group)| !used_local[*index] && group.name == base.name)
            .map(|(index, _)| index)
        else {
            continue;
        };
        take(
            &mut pairs,
            &mut used_local,
            &mut used_remote,
            li,
            ri,
            How::Identity,
        );
    }
    pair_equal_sets(
        &local,
        &remote,
        &baseline,
        &mut used_local,
        &mut used_remote,
        &mut pairs,
    );
    pair_overlap(
        &local,
        &remote,
        &baseline,
        &mut used_local,
        &mut used_remote,
        &mut pairs,
    );

    let mut out = Vec::new();
    for (li, group) in local.iter().enumerate() {
        if let Some(pair) = pairs.iter().find(|pair| pair.local == li) {
            out.push(resolve_pair(
                group,
                &remote[pair.remote],
                baseline
                    .iter()
                    .find(|base| base.section_id == remote[pair.remote].id),
                matches!(pair.how, How::Identity),
                choices,
                require,
            )?);
        } else {
            out.push(Applied {
                action: "push_add",
                section_id: String::new(),
                name: group.name.clone(),
                local_name: group.name.clone(),
                remote_name: String::new(),
                songs: group.songs.clone(),
                create: true,
                rename: false,
                push_songs: !group.songs.is_empty(),
                write_local: false,
            });
        }
    }
    for (ri, section) in remote.iter().enumerate() {
        if used_remote[ri] {
            continue;
        }
        out.push(Applied {
            action: "pull_add",
            section_id: section.id.clone(),
            name: section.name.clone(),
            local_name: String::new(),
            remote_name: section.name.clone(),
            songs: section.songs.clone(),
            create: false,
            rename: false,
            push_songs: false,
            write_local: true,
        });
    }
    Ok(out)
}

fn take(
    pairs: &mut Vec<Pair>,
    used_local: &mut [bool],
    used_remote: &mut [bool],
    local: usize,
    remote: usize,
    how: How,
) {
    used_local[local] = true;
    used_remote[remote] = true;
    pairs.push(Pair { local, remote, how });
}

fn pair_equal_sets(
    local: &[Local],
    remote: &[Remote],
    baseline: &[Base],
    used_local: &mut [bool],
    used_remote: &mut [bool],
    pairs: &mut Vec<Pair>,
) {
    let mut hits: Vec<(usize, usize)> = Vec::new();
    for (ri, section) in remote.iter().enumerate() {
        if used_remote[ri] {
            continue;
        }
        let Some(base) = baseline.iter().find(|base| base.section_id == section.id) else {
            continue;
        };
        let matched: Vec<usize> = local
            .iter()
            .enumerate()
            .filter(|(index, group)| !used_local[*index] && group.set == base.set)
            .map(|(index, _)| index)
            .collect();
        if matched.len() == 1 {
            hits.push((ri, matched[0]));
        }
    }
    let mut seen = BTreeMap::<usize, usize>::new();
    for (_, li) in &hits {
        *seen.entry(*li).or_insert(0) += 1;
    }
    for (ri, li) in hits {
        if seen.get(&li) == Some(&1) && !used_local[li] && !used_remote[ri] {
            take(pairs, used_local, used_remote, li, ri, How::Identity);
        }
    }
}

fn pair_overlap(
    local: &[Local],
    remote: &[Remote],
    baseline: &[Base],
    used_local: &mut [bool],
    used_remote: &mut [bool],
    pairs: &mut Vec<Pair>,
) {
    let pending: Vec<usize> = remote
        .iter()
        .enumerate()
        .filter(|(ri, section)| {
            !used_remote[*ri]
                && baseline
                    .iter()
                    .any(|base| base.section_id == section.id && !base.set.is_empty())
        })
        .map(|(ri, _)| ri)
        .collect();
    let free: Vec<usize> = (0..local.len()).filter(|li| !used_local[*li]).collect();
    let mut claim = vec![None; pending.len()];
    for (pi, ri) in pending.iter().enumerate() {
        let Some(base) = baseline
            .iter()
            .find(|base| base.section_id == remote[*ri].id)
        else {
            continue;
        };
        let mut best = 0usize;
        let mut at = None;
        let mut tie = false;
        for li in &free {
            let score = local[*li].set.intersection(&base.set).count();
            if score > best {
                best = score;
                at = Some(*li);
                tie = false;
            } else if score == best && score > 0 && at.is_some() {
                tie = true;
            }
        }
        if best > 0 && !tie {
            claim[pi] = at;
        }
    }
    let mut seen = BTreeMap::<usize, usize>::new();
    for li in claim.iter().flatten() {
        *seen.entry(*li).or_insert(0) += 1;
    }
    for (pi, at) in claim.into_iter().enumerate() {
        let Some(li) = at else { continue };
        if seen.get(&li) != Some(&1) {
            continue;
        }
        let ri = pending[pi];
        if !used_local[li] && !used_remote[ri] {
            take(pairs, used_local, used_remote, li, ri, How::Identity);
        }
    }
}

fn resolve_pair(
    local: &Local,
    remote: &Remote,
    base: Option<&Base>,
    identity: bool,
    choices: &[Choice],
    require: bool,
) -> Result<Applied, String> {
    let songs = union(&local.songs, &remote.songs);
    let base_name = base.map(|base| base.name.as_str()).unwrap_or("");
    let both_renamed = identity
        && local.name != remote.name
        && !base_name.is_empty()
        && local.name != base_name
        && remote.name != base_name;
    if both_renamed && !require {
        return Ok(Applied {
            action: "conflict",
            section_id: remote.id.clone(),
            name: local.name.clone(),
            local_name: local.name.clone(),
            remote_name: remote.name.clone(),
            songs,
            create: false,
            rename: false,
            push_songs: false,
            write_local: false,
        });
    }
    let name = if both_renamed {
        match choices
            .iter()
            .find(|choice| choice.section_id == remote.id)
            .map(|choice| choice.choice.as_str())
        {
            Some("keep_local") => local.name.clone(),
            Some("take_server") => remote.name.clone(),
            _ => return Err("Нужно выбрать, чью версию группы оставить".into()),
        }
    } else if !base_name.is_empty() && local.name == base_name && remote.name != base_name {
        remote.name.clone()
    } else {
        local.name.clone()
    };
    let push_songs = song_set(&songs) != remote.set;
    let write_local = name != local.name || song_set(&songs) != local.set;
    let rename = name != remote.name;
    let action = if rename || push_songs {
        "push_update"
    } else if write_local {
        "pull_update"
    } else {
        "unchanged"
    };
    Ok(Applied {
        action,
        section_id: remote.id.clone(),
        name,
        local_name: local.name.clone(),
        remote_name: remote.name.clone(),
        songs,
        create: false,
        rename,
        push_songs,
        write_local,
    })
}

fn union(local: &[String], remote: &[String]) -> Vec<String> {
    let mut out = Vec::new();
    let mut seen = BTreeSet::new();
    for id in local.iter().chain(remote) {
        if seen.insert(id.clone()) {
            out.push(id.clone());
        }
    }
    out
}

fn song_set(songs: &[String]) -> BTreeSet<String> {
    songs.iter().cloned().collect()
}

fn normalize_local(groups: &[Side]) -> Result<Vec<Local>, String> {
    let mut out = Vec::new();
    for group in groups {
        let (name, songs, set) = songs_of(&group.name, &group.songs)?;
        out.push(Local { name, songs, set });
    }
    unique(
        &out.iter()
            .map(|group| group.name.as_str())
            .collect::<Vec<_>>(),
        "две группы с одним названием",
    )?;
    Ok(out)
}

fn normalize_remote(sections: &[Section]) -> Result<Vec<Remote>, String> {
    let mut out = Vec::new();
    for section in sections {
        let (name, songs, set) = songs_of(&section.name, &section.songs)?;
        out.push(Remote {
            id: section.id.clone(),
            name,
            songs,
            set,
        });
    }
    unique(
        &out.iter()
            .map(|section| section.name.as_str())
            .collect::<Vec<_>>(),
        "два раздела с одним названием",
    )?;
    Ok(out)
}

fn songs_of(
    name: &str,
    songs: &[String],
) -> Result<(String, Vec<String>, BTreeSet<String>), String> {
    let name = name.trim().to_string();
    if name.is_empty() {
        return Err("У группы нет названия".into());
    }
    let mut order = Vec::new();
    let mut set = BTreeSet::new();
    for id in songs {
        let id = id.trim();
        if id.is_empty() || !set.insert(id.to_string()) {
            continue;
        }
        order.push(id.to_string());
    }
    Ok((name, order, set))
}

fn unique(names: &[&str], message: &str) -> Result<(), String> {
    let mut seen = BTreeSet::new();
    for name in names {
        if !seen.insert(*name) {
            return Err(message.into());
        }
    }
    Ok(())
}

fn normalize_base(baseline: &[Baseline]) -> Vec<Base> {
    let mut out = Vec::new();
    let mut seen = BTreeSet::new();
    for link in baseline {
        let section_id = link.section_id.trim().to_string();
        let name = link.name.trim().to_string();
        if section_id.is_empty() || name.is_empty() || !seen.insert(section_id.clone()) {
            continue;
        }
        out.push(Base {
            section_id,
            name,
            set: link
                .songs
                .iter()
                .map(|id| id.trim().to_string())
                .filter(|id| !id.is_empty())
                .collect(),
        });
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn side(name: &str, songs: &[&str]) -> Side {
        Side {
            name: name.into(),
            songs: songs.iter().map(|id| (*id).to_string()).collect(),
        }
    }

    fn section(id: &str, name: &str, songs: &[&str]) -> Section {
        Section {
            id: id.into(),
            name: name.into(),
            songs: songs.iter().map(|song| (*song).to_string()).collect(),
        }
    }

    fn base(id: &str, name: &str, songs: &[&str]) -> Baseline {
        Baseline {
            section_id: id.into(),
            name: name.into(),
            songs: songs.iter().map(|song| (*song).to_string()).collect(),
        }
    }

    #[test]
    fn same_name_unions_songs_without_a_conflict() {
        let merged = merge(
            &[side("Хор", &["a"])],
            &[section("s1", "Хор", &["b"])],
            &[base("s1", "Хор", &["a"])],
            &[],
            false,
        )
        .unwrap();
        assert_eq!(merged.len(), 1);
        assert_eq!(merged[0].action, "push_update");
        assert_eq!(merged[0].songs, vec!["a".to_string(), "b".to_string()]);
        assert!(merged[0].push_songs);
        assert!(merged[0].write_local);
        assert!(!merged[0].rename);
        assert_eq!(merged[0].name, "Хор");
    }

    #[test]
    fn equal_group_is_unchanged() {
        let merged = merge(
            &[side("Хор", &["a", "b"])],
            &[section("s1", "Хор", &["b", "a"])],
            &[base("s1", "Хор", &["a"])],
            &[],
            true,
        )
        .unwrap();
        assert_eq!(merged[0].action, "unchanged");
        assert!(!merged[0].push_songs);
        assert!(!merged[0].write_local);
    }

    #[test]
    fn local_only_and_server_only_are_adds() {
        let merged = merge(
            &[side("Новая", &["a"])],
            &[section("s2", "С сервера", &["b"])],
            &[],
            &[],
            true,
        )
        .unwrap();
        assert_eq!(merged[0].action, "push_add");
        assert!(merged[0].create);
        assert_eq!(merged[0].songs, vec!["a".to_string()]);
        assert_eq!(merged[1].action, "pull_add");
        assert_eq!(merged[1].section_id, "s2");
        assert!(merged[1].write_local);
        assert!(!merged[1].push_songs);
    }

    #[test]
    fn server_rename_alone_takes_the_server_title() {
        let merged = merge(
            &[side("Хор", &["a"])],
            &[section("s1", "Хор Б", &["a"])],
            &[base("s1", "Хор", &["a"])],
            &[],
            true,
        )
        .unwrap();
        assert_eq!(merged[0].action, "pull_update");
        assert_eq!(merged[0].name, "Хор Б");
        assert!(!merged[0].rename);
        assert!(merged[0].write_local);
        assert!(!merged[0].push_songs);
    }

    #[test]
    fn local_rename_alone_pushes_the_local_title() {
        let merged = merge(
            &[side("Хор А", &["a"])],
            &[section("s1", "Хор", &["a"])],
            &[base("s1", "Хор", &["a"])],
            &[],
            true,
        )
        .unwrap();
        assert_eq!(merged[0].action, "push_update");
        assert_eq!(merged[0].name, "Хор А");
        assert!(merged[0].rename);
        assert!(!merged[0].write_local);
    }

    #[test]
    fn both_renames_conflict_until_a_title_is_chosen() {
        let local = [side("Хор А", &["a"])];
        let remote = [section("s1", "Хор Б", &["a", "b"])];
        let baseline = [base("s1", "Хор", &["a"])];
        let shown = merge(&local, &remote, &baseline, &[], false).unwrap();
        assert_eq!(shown[0].action, "conflict");
        assert_eq!(shown[0].local_name, "Хор А");
        assert_eq!(shown[0].remote_name, "Хор Б");
        assert_eq!(shown[0].section_id, "s1");
        let err = merge(&local, &remote, &baseline, &[], true).unwrap_err();
        assert!(err.contains("выбрать"));
        let keep = merge(
            &local,
            &remote,
            &baseline,
            &[Choice {
                section_id: "s1".into(),
                choice: "keep_local".into(),
            }],
            true,
        )
        .unwrap();
        assert_eq!(keep[0].name, "Хор А");
        assert!(keep[0].rename);
        assert_eq!(keep[0].songs, vec!["a".to_string(), "b".to_string()]);
        let take = merge(
            &local,
            &remote,
            &baseline,
            &[Choice {
                section_id: "s1".into(),
                choice: "take_server".into(),
            }],
            true,
        )
        .unwrap();
        assert_eq!(take[0].name, "Хор Б");
        assert!(!take[0].rename);
        assert!(take[0].write_local);
        assert!(!take[0].push_songs);
    }

    #[test]
    fn the_same_new_name_on_both_sides_is_not_a_conflict() {
        let merged = merge(
            &[side("Хор А", &["a"])],
            &[section("s1", "Хор А", &["a"])],
            &[base("s1", "Хор", &["a"])],
            &[],
            true,
        )
        .unwrap();
        assert_eq!(merged[0].action, "unchanged");
        assert_eq!(merged[0].section_id, "s1");
    }

    #[test]
    fn a_new_group_does_not_swallow_a_renamed_one() {
        let merged = merge(
            &[side("Хор А", &["a"]), side("Новая", &["c"])],
            &[section("s1", "Хор", &["a"])],
            &[base("s1", "Хор", &["a"])],
            &[],
            true,
        )
        .unwrap();
        assert_eq!(merged.len(), 2);
        assert_eq!(merged[0].name, "Хор А");
        assert_eq!(merged[0].section_id, "s1");
        assert!(merged[0].rename);
        assert_eq!(merged[1].action, "push_add");
        assert_eq!(merged[1].name, "Новая");
    }

    #[test]
    fn disjoint_songs_are_two_groups_not_one_rename() {
        let merged = merge(
            &[side("Новая", &["b"])],
            &[section("s1", "Хор", &["a"])],
            &[base("s1", "Хор", &["a"])],
            &[],
            true,
        )
        .unwrap();
        assert_eq!(merged[0].action, "push_add");
        assert_eq!(merged[0].name, "Новая");
        assert_eq!(merged[1].action, "pull_add");
        assert_eq!(merged[1].name, "Хор");
    }

    #[test]
    fn two_renames_stay_apart_by_their_songs() {
        let merged = merge(
            &[side("Хор А", &["a"]), side("Дети Б", &["c"])],
            &[
                section("s1", "Хор В", &["a"]),
                section("s2", "Дети Г", &["c"]),
            ],
            &[base("s1", "Хор", &["a"]), base("s2", "Дети", &["c"])],
            &[],
            false,
        )
        .unwrap();
        assert_eq!(merged.len(), 2);
        assert!(merged.iter().all(|row| row.action == "conflict"));
        assert_eq!(merged[0].section_id, "s1");
        assert_eq!(merged[1].section_id, "s2");
    }

    #[test]
    fn duplicate_names_are_refused() {
        let err = merge(
            &[side("Хор", &["a"]), side("Хор", &["b"])],
            &[],
            &[],
            &[],
            true,
        )
        .unwrap_err();
        assert!(err.contains("группы"));
        let err = merge(
            &[],
            &[section("s1", "Хор", &["a"]), section("s2", "Хор", &["b"])],
            &[],
            &[],
            true,
        )
        .unwrap_err();
        assert!(err.contains("раздела"));
    }
}
