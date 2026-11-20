package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"flammebible/internal/vbm"
)

// Layer — снимок одного сборника цепочки. Decisions относятся только к этому сборнику.
type Layer struct {
	ID        string            `json:"id"`
	ParentID  string            `json:"parent_id"`
	Songs     []songDTO         `json:"songs,omitempty"`
	Order     []string          `json:"order,omitempty"`
	Decisions map[string]string `json:"-"`
}

type bookView struct {
	by    map[string]songDTO
	order []string
}

func validateChain(layers []Layer) error {
	parent := map[string]string{}
	for _, layer := range layers {
		if layer.ID == "" {
			return fmt.Errorf("нет сборника")
		}
		if _, dup := parent[layer.ID]; dup {
			return errCycle
		}
		parent[layer.ID] = layer.ParentID
	}
	if len(layers) == 0 {
		return fmt.Errorf("нет сборника")
	}
	id := layers[len(layers)-1].ID
	seen := map[string]bool{}
	for id != "" {
		if seen[id] {
			return errCycle
		}
		seen[id] = true
		if len(seen) > 8 {
			return errDepth
		}
		next, ok := parent[id]
		if !ok {
			return fmt.Errorf("нет сборника")
		}
		id = next
	}
	return nil
}

func Assemble(layers []Layer) ([]vbm.Song, string, error) {
	if err := validateChain(layers); err != nil {
		return nil, "", err
	}
	for i := range layers {
		if layers[i].Decisions == nil {
			layers[i].Decisions = map[string]string{}
		}
	}
	view := rootView(layers[0])
	var report strings.Builder
	for i := 1; i < len(layers); i++ {
		next, text, err := fold(view, &layers[i])
		report.WriteString(text)
		if err != nil {
			return nil, report.String(), err
		}
		view = next
	}
	return assignIDs(view), report.String(), nil
}

func rootView(layer Layer) bookView {
	by := map[string]songDTO{}
	for _, song := range layer.Songs {
		by[song.FBID] = song
	}
	return bookView{by: by, order: liveIDs(layer)}
}

func liveIDs(layer Layer) []string {
	by := map[string]bool{}
	deleted := map[string]bool{}
	for _, song := range layer.Songs {
		by[song.FBID] = true
		if song.Deleted {
			deleted[song.FBID] = true
		}
	}
	src := layer.Order
	if len(src) == 0 {
		src = make([]string, len(layer.Songs))
		for i, song := range layer.Songs {
			src[i] = song.FBID
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, id := range src {
		if !by[id] || deleted[id] || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func fold(base bookView, child *Layer) (bookView, string, error) {
	byChild := map[string]songDTO{}
	for _, song := range child.Songs {
		byChild[song.FBID] = song
	}
	custom := len(child.Order) > 0
	inCustom := map[string]bool{}
	for _, id := range child.Order {
		inCustom[id] = true
	}
	var order []string
	seen := map[string]bool{}
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		order = append(order, id)
	}
	var report strings.Builder
	if custom {
		for _, id := range child.Order {
			add(id)
		}
		for _, id := range base.order {
			if seen[id] {
				continue
			}
			song := base.by[id]
			if song.Deleted {
				continue
			}
			add(id)
			fmt.Fprintf(&report, "в конец %s %s\n", id, song.Title)
		}
	} else {
		for _, id := range base.order {
			if base.by[id].Deleted {
				continue
			}
			add(id)
		}
		for _, song := range child.Songs {
			if song.Deleted || seen[song.FBID] {
				continue
			}
			if parent, ok := base.by[song.FBID]; ok && parent.Deleted {
				continue
			}
			add(song.FBID)
		}
	}
	var extra []string
	for id, song := range base.by {
		if !song.Deleted || seen[id] {
			continue
		}
		childSong, inChild := byChild[id]
		if child.Decisions[id] != "" || childChanged(song, childSong, inChild) || inCustom[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	for _, id := range extra {
		add(id)
	}

	outBy := map[string]songDTO{}
	var outOrder []string
	var questions []string
	for _, id := range order {
		baseSong, inBase := base.by[id]
		childSong, inChild := byChild[id]
		decision := child.Decisions[id]
		if decision == "suppress" {
			continue
		}
		if inBase && baseSong.Deleted {
			switch decision {
			case "retain":
				kept := baseSong
				kept.Deleted = false
				if inChild && !childSong.Deleted {
					kept = childSong
				}
				outBy[id] = kept
				outOrder = append(outOrder, id)
			case "drop":
				continue
			default:
				if !childChanged(baseSong, childSong, inChild) && !inCustom[id] {
					continue
				}
				title := baseSong.Title
				if inChild && childSong.Title != "" {
					title = childSong.Title
				}
				fmt.Fprintf(&report, "вопрос %s %s\n", id, title)
				questions = append(questions, id)
			}
			continue
		}
		if decision == "drop" {
			continue
		}
		if inChild && !childSong.Deleted {
			if decision == "retain" && inBase && !baseSong.Deleted && sameText(baseSong, childSong) {
				delete(child.Decisions, id)
				outBy[id] = baseSong
				outOrder = append(outOrder, id)
				continue
			}
			outBy[id] = childSong
			outOrder = append(outOrder, id)
			continue
		}
		if inChild && childSong.Deleted && !inBase {
			continue
		}
		if inBase && !baseSong.Deleted {
			outBy[id] = baseSong
			outOrder = append(outOrder, id)
		}
	}
	for id, song := range base.by {
		if song.Deleted {
			if _, ok := outBy[id]; !ok {
				outBy[id] = song
			}
		}
	}
	for id, song := range byChild {
		if song.Deleted {
			if _, ok := outBy[id]; !ok {
				outBy[id] = song
			}
		}
	}
	if len(questions) > 0 {
		return bookView{}, report.String(), errChoose
	}
	if outOrder == nil {
		outOrder = []string{}
	}
	return bookView{by: outBy, order: outOrder}, report.String(), nil
}

func childChanged(parent, child songDTO, ok bool) bool {
	return ok && !child.Deleted && !sameText(parent, child)
}

func sameText(a, b songDTO) bool {
	return songEqual(stripID(a.vbm()), stripID(b.vbm()))
}

func assignIDs(view bookView) []vbm.Song {
	out := make([]vbm.Song, 0, len(view.order))
	for _, id := range view.order {
		song, ok := view.by[id]
		if !ok || song.Deleted {
			continue
		}
		item := song.vbm()
		item.Properties = setProp(item.Properties, "$ID$", strconv.Itoa(len(out)+1))
		out = append(out, item)
	}
	return out
}

func buildLocal(path string, layers []Layer, apply bool) (string, error) {
	songs, report, err := Assemble(layers)
	if err != nil {
		return report, err
	}
	if !apply {
		return report, nil
	}
	if err := saveModuleSongs(path, songs); err != nil {
		return report, err
	}
	return report, nil
}

func saveModuleSongs(path string, songs []vbm.Song) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	book, err := vbm.Parse(data)
	if err != nil {
		return err
	}
	if songs == nil {
		songs = []vbm.Song{}
	}
	book.Songs = songs
	payload, err := vbm.Write(book, data)
	if err != nil {
		return err
	}
	if err := backupModule(path); err != nil {
		return err
	}
	return commitModule(path, payload)
}

func setProp(properties, key, value string) string {
	sep, lines := propLines(properties)
	prefix := key + "="
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = prefix + value
			found = true
			break
		}
	}
	if !found {
		switch {
		case len(lines) == 1 && lines[0] == "":
			lines = []string{prefix + value}
		case len(lines) > 0 && lines[len(lines)-1] == "":
			lines = append(lines[:len(lines)-1], prefix+value, "")
		default:
			lines = append(lines, prefix+value)
		}
	}
	return strings.Join(lines, sep)
}

func removeProp(properties, key string) string {
	sep, lines := propLines(properties)
	prefix := key + "="
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, sep)
}

func propLines(properties string) (string, []string) {
	sep := "\n"
	if strings.Contains(properties, "\r\n") {
		sep = "\r\n"
	}
	norm := strings.ReplaceAll(properties, "\r\n", "\n")
	norm = strings.ReplaceAll(norm, "\r", "\n")
	return sep, strings.Split(norm, "\n")
}

func stripID(song vbm.Song) vbm.Song {
	song.Properties = removeProp(song.Properties, "$ID$")
	return song
}
