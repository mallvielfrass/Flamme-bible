package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

type planItem struct {
	Caption   string
	Reference string
	Quote     string
	Class     string
}

type planSyncItem struct {
	ID      string `json:"id"`
	Hash    string `json:"hash"`
	Deleted bool   `json:"deleted,omitempty"`
}

type planSync struct {
	ID    string         `json:"id"`
	Items []planSyncItem `json:"items"`
}

func syncPlan(path string) (planSync, error) {
	items, err := readPlan(path)
	if err != nil {
		return planSync{}, err
	}
	side := path + ".sync"
	current, err := loadPlanSync(side)
	if err != nil {
		return planSync{}, err
	}
	if current.ID == "" {
		current.ID, err = newUUID()
		if err != nil {
			return planSync{}, err
		}
	}
	hashes := make([]string, len(items))
	for i, item := range items {
		hashes[i] = planItemHash(item)
	}
	next, err := alignPlan(current.Items, hashes)
	if err != nil {
		return planSync{}, err
	}
	current.Items = next
	raw, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return planSync{}, err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(side, raw, 0o644); err != nil {
		return planSync{}, err
	}
	return current, nil
}

func readPlan(path string) ([]planItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text, err := decodePlan(data)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Journals []struct {
			Items []struct {
				Caption   string `xml:"Caption,attr"`
				Reference string `xml:"Reference,attr"`
				Quote     string `xml:"Quote,attr"`
				Class     string `xml:"Class,attr"`
			} `xml:"Item"`
		} `xml:"Journal"`
	}
	if err := xml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	var items []planItem
	for _, journal := range doc.Journals {
		for _, item := range journal.Items {
			items = append(items, planItem{
				Caption: item.Caption, Reference: item.Reference, Quote: item.Quote, Class: item.Class,
			})
		}
	}
	return items, nil
}

func decodePlan(data []byte) (string, error) {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	if utf8.Valid(data) {
		return string(data), nil
	}
	decoded, err := charmap.Windows1251.NewDecoder().Bytes(data)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func planItemHash(item planItem) string {
	sum := sha256.Sum256([]byte(item.Caption + "\x00" + item.Reference + "\x00" + item.Quote + "\x00" + item.Class))
	return hex.EncodeToString(sum[:])
}

func loadPlanSync(path string) (planSync, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return planSync{}, nil
		}
		return planSync{}, err
	}
	var current planSync
	if err := json.Unmarshal(data, &current); err != nil {
		return planSync{}, err
	}
	if current.ID != "" && !validUUID(current.ID) {
		return planSync{}, fmt.Errorf("план")
	}
	return current, nil
}

func alignPlan(old []planSyncItem, hashes []string) ([]planSyncItem, error) {
	var active, deleted []planSyncItem
	for _, item := range old {
		if item.Deleted {
			deleted = append(deleted, item)
			continue
		}
		active = append(active, item)
	}
	usedOld := make([]bool, len(active))
	usedNew := make([]bool, len(hashes))
	assigned := make([]string, len(hashes))
	limit := len(active)
	if len(hashes) < limit {
		limit = len(hashes)
	}
	for i := 0; i < limit; i++ {
		if active[i].Hash == hashes[i] {
			usedOld[i] = true
			usedNew[i] = true
			assigned[i] = active[i].ID
		}
	}
	oldByHash := map[string][]int{}
	for i, item := range active {
		if !usedOld[i] {
			oldByHash[item.Hash] = append(oldByHash[item.Hash], i)
		}
	}
	newByHash := map[string][]int{}
	for i, hash := range hashes {
		if !usedNew[i] {
			newByHash[hash] = append(newByHash[hash], i)
		}
	}
	for hash, oldIdx := range oldByHash {
		newIdx := newByHash[hash]
		if len(oldIdx) == 1 && len(newIdx) == 1 {
			usedOld[oldIdx[0]] = true
			usedNew[newIdx[0]] = true
			assigned[newIdx[0]] = active[oldIdx[0]].ID
		}
	}
	var leftOld, leftNew []int
	for i := range active {
		if !usedOld[i] {
			leftOld = append(leftOld, i)
		}
	}
	for i := range hashes {
		if !usedNew[i] {
			leftNew = append(leftNew, i)
		}
	}
	if len(leftOld) == 1 && len(leftNew) == 1 {
		assigned[leftNew[0]] = active[leftOld[0]].ID
		usedOld[leftOld[0]] = true
	}
	for i, item := range active {
		if usedOld[i] {
			continue
		}
		item.Deleted = true
		deleted = append(deleted, item)
	}
	current := make([]planSyncItem, 0, len(hashes)+len(deleted))
	for i, hash := range hashes {
		id := assigned[i]
		if id == "" {
			var err error
			id, err = newUUID()
			if err != nil {
				return nil, err
			}
		}
		current = append(current, planSyncItem{ID: id, Hash: hash})
	}
	return append(current, deleted...), nil
}

func formatPlan(sync planSync) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "план %s\n", sync.ID)
	for _, item := range sync.Items {
		if item.Deleted {
			fmt.Fprintf(&buf, "удалён %s\n", item.ID)
			continue
		}
		fmt.Fprintf(&buf, "пункт %s\n", item.ID)
	}
	return buf.String()
}
