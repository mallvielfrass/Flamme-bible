package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"flammebible/internal/vbm"
)

var (
	errUnresolved = errors.New("конфликт")
	errChoose     = errors.New("нужен ответ")
	errCycle      = errors.New("цикл")
	errDepth      = errors.New("глубина")
	errReload     = errors.New("reload")
)

type partDTO struct {
	Kind  string `json:"kind"`
	Text  string `json:"text"`
	Align uint32 `json:"align"`
}

type songDTO struct {
	FBID       string    `json:"fbid"`
	Title      string    `json:"title"`
	AltTitle   string    `json:"alt_title"`
	Author     string    `json:"author"`
	Composer   string    `json:"composer"`
	Note       string    `json:"note"`
	Properties string    `json:"properties"`
	Parts      []partDTO `json:"parts"`
	Deleted    bool      `json:"deleted"`
}

func (s songDTO) vbm() vbm.Song {
	parts := make([]vbm.Part, len(s.Parts))
	for i, part := range s.Parts {
		parts[i] = vbm.Part{Kind: part.Kind, Text: part.Text, Align: part.Align}
	}
	if parts == nil {
		parts = []vbm.Part{}
	}
	props := s.Properties
	if s.FBID != "" {
		props = setProp(props, "$FBID$", s.FBID)
	}
	return vbm.Song{
		Title:      s.Title,
		AltTitle:   s.AltTitle,
		Author:     s.Author,
		Composer:   s.Composer,
		Note:       s.Note,
		Properties: props,
		Parts:      parts,
	}
}

func songFromVBM(song vbm.Song, deleted bool) (songDTO, error) {
	fbid, ok, err := songFBID(song.Properties)
	if err != nil {
		return songDTO{}, err
	}
	if !ok {
		return songDTO{}, fmt.Errorf("нет FBID")
	}
	parts := make([]partDTO, len(song.Parts))
	for i, part := range song.Parts {
		parts[i] = partDTO{Kind: part.Kind, Text: part.Text, Align: part.Align}
	}
	if parts == nil {
		parts = []partDTO{}
	}
	return songDTO{
		FBID:       fbid,
		Title:      song.Title,
		AltTitle:   song.AltTitle,
		Author:     song.Author,
		Composer:   song.Composer,
		Note:       song.Note,
		Properties: song.Properties,
		Parts:      parts,
		Deleted:    deleted,
	}, nil
}

type pushOp struct {
	OperationID string    `json:"operation_id"`
	Song        songDTO   `json:"song"`
	Order       *[]string `json:"order,omitempty"`
}

type opResult struct {
	OperationID string          `json:"operation_id"`
	Status      string          `json:"status"`
	Revision    int64           `json:"revision"`
	Server      json.RawMessage `json:"server"`
	Client      json.RawMessage `json:"client"`
}

type changeEvent struct {
	Seq        int64  `json:"seq"`
	SongbookID string `json:"songbook_id"`
	Body       struct {
		Song  songDTO   `json:"song"`
		Order *[]string `json:"order"`
	} `json:"body"`
}

type snapView struct {
	Cursor   int64     `json:"cursor"`
	Songbook Layer     `json:"songbook"`
	Songs    []songDTO `json:"songs"`
	Order    []string  `json:"order"`
}

type remoteState struct {
	ServerID string
	Cursor   int64
	Loaded   bool
}

type conflictRow struct {
	FBID   string
	Server string
	Client string
}

type remoteClient struct {
	base  string
	token string
	http  *http.Client
}

func login(serverURL, email, password string) (Session, error) {
	httpClient, err := newHTTPClient()
	if err != nil {
		return Session{}, err
	}
	cli := &remoteClient{base: strings.TrimRight(serverURL, "/"), http: httpClient}
	var resp struct {
		UserID      string `json:"user_id"`
		DeviceID    string `json:"device_id"`
		DimensionID string `json:"dimension_id"`
		Token       string `json:"token"`
	}
	err = cli.do(context.Background(), http.MethodPost, "/login", map[string]string{
		"email":    email,
		"password": password,
	}, &resp)
	if err != nil {
		return Session{}, err
	}
	return Session{
		Server:      cli.base,
		Token:       resp.Token,
		DeviceID:    resp.DeviceID,
		UserID:      resp.UserID,
		DimensionID: resp.DimensionID,
	}, nil
}

func newClient(sess Session) (*remoteClient, error) {
	httpClient, err := newHTTPClient()
	if err != nil {
		return nil, err
	}
	return &remoteClient{base: strings.TrimRight(sess.Server, "/"), token: sess.Token, http: httpClient}, nil
}

func (c *remoteClient) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		var wrapped struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &wrapped) == nil && wrapped.Error != "" {
			msg = wrapped.Error
		}
		if msg == "reload" {
			return errReload
		}
		if msg == "цикл" {
			return errCycle
		}
		if msg == "" {
			msg = res.Status
		}
		return fmt.Errorf("%s", msg)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *remoteClient) ensureBook(ctx context.Context, dim string, book *vbm.Book, identity string) (string, error) {
	var list []struct {
		ID   string `json:"id"`
		FBID string `json:"fbid"`
	}
	if err := c.do(ctx, http.MethodGet, "/dimensions/"+dim+"/songbooks", nil, &list); err != nil {
		return "", err
	}
	for _, item := range list {
		if item.FBID == identity {
			return item.ID, nil
		}
	}
	var created struct {
		ID string `json:"id"`
	}
	body := map[string]any{
		"fbid":          identity,
		"title":         book.Title,
		"short_name":    book.ShortName,
		"publisher":     book.Publisher,
		"revision_date": book.RevisionDate,
		"comment":       book.Comment,
		"header": map[string]any{
			"format_version": book.Header.FormatVersion,
			"flags":          book.Header.Flags,
			"charset":        book.Header.Charset,
			"header_short":   book.Header.HeaderShort,
			"header_title":   book.Header.HeaderTitle,
		},
	}
	if err := c.do(ctx, http.MethodPost, "/dimensions/"+dim+"/songbooks", body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

func (c *remoteClient) push(ctx context.Context, dim, book string, base int64, ops []pushOp) ([]opResult, error) {
	var resp struct {
		Results []opResult `json:"results"`
	}
	err := c.do(ctx, http.MethodPost, "/dimensions/"+dim+"/songbooks/"+book+"/push", map[string]any{
		"base_revision": base,
		"operations":    ops,
	}, &resp)
	return resp.Results, err
}

func (c *remoteClient) changes(ctx context.Context, dim string, cursor int64) ([]changeEvent, error) {
	var resp struct {
		Events []changeEvent `json:"events"`
	}
	path := fmt.Sprintf("/dimensions/%s/changes?cursor=%d&limit=50", dim, cursor)
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Events == nil {
		resp.Events = []changeEvent{}
	}
	return resp.Events, nil
}

func (c *remoteClient) snapshot(ctx context.Context, dim, book string) (snapView, error) {
	var snap snapView
	err := c.do(ctx, http.MethodGet, "/dimensions/"+dim+"/songbooks/"+book+"/snapshot", nil, &snap)
	if err != nil {
		return snapView{}, err
	}
	if snap.Songs == nil {
		snap.Songs = []songDTO{}
	}
	if snap.Order == nil {
		snap.Order = []string{}
	}
	return snap, nil
}

func (c *remoteClient) loadChain(ctx context.Context, dim, id string) ([]Layer, error) {
	var chain []Layer
	seen := map[string]bool{}
	for id != "" {
		if seen[id] {
			return nil, errCycle
		}
		seen[id] = true
		if len(seen) > 8 {
			return nil, errDepth
		}
		snap, err := c.snapshot(ctx, dim, id)
		if err != nil {
			return nil, err
		}
		parent := ""
		if snap.Songbook.ParentID != "" {
			parent = snap.Songbook.ParentID
		}
		chain = append(chain, Layer{
			ID:       snap.Songbook.ID,
			ParentID: parent,
			Songs:    snap.Songs,
			Order:    snap.Order,
		})
		id = parent
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// RemoteSync отправляет очередь до pull: так сервер успевает вернуть конфликт.
// Снимок и курсор пишутся только после записи страницы в файл.
func (s *Store) RemoteSync(ctx context.Context, path string, apply bool) (string, error) {
	if !apply {
		changes, err := s.Diff(path)
		return formatChanges(changes), err
	}
	sess, err := loadSession()
	if err != nil {
		return "", err
	}
	if sess.DimensionID == "" {
		return "", fmt.Errorf("пространство не выбрано")
	}
	cli, err := newClient(sess)
	if err != nil {
		return "", err
	}
	binding, err := s.Bind(path)
	if err != nil {
		return "", err
	}
	rows, err := s.listConflicts(binding.ID)
	if err != nil {
		return "", err
	}
	if len(rows) > 0 {
		return formatConflictRows(rows), errUnresolved
	}
	book, err := vbm.ParseFile(path)
	if err != nil {
		return "", err
	}
	state, err := s.ensureServerBook(ctx, cli, sess.DimensionID, binding, book)
	if err != nil {
		return "", err
	}
	if !state.Loaded {
		if err := s.pull(ctx, cli, sess.DimensionID, binding.ID, state.ServerID, path); err != nil {
			return "", err
		}
		if err := s.markLoaded(binding.ID); err != nil {
			return "", err
		}
		state, err = s.remoteOf(binding.ID)
		if err != nil {
			return "", err
		}
	}
	changes, err := s.Diff(path)
	if err != nil {
		return "", err
	}
	if err := writeAssigned(path, changes); err != nil {
		return "", err
	}
	report, err := s.pushOps(ctx, cli, sess.DimensionID, state.ServerID, state.Cursor, binding.ID, path)
	if err != nil {
		return report, err
	}
	if err := s.pull(ctx, cli, sess.DimensionID, binding.ID, state.ServerID, path); err != nil {
		return report, err
	}
	return report, nil
}

func (s *Store) ensureServerBook(ctx context.Context, cli *remoteClient, dim string, binding BookBinding, book *vbm.Book) (remoteState, error) {
	state, err := s.remoteOf(binding.ID)
	if err != nil {
		return remoteState{}, err
	}
	if state.ServerID != "" {
		return state, nil
	}
	serverID, err := cli.ensureBook(ctx, dim, book, binding.Identity)
	if err != nil {
		return remoteState{}, err
	}
	if err := s.linkBook(binding.ID, serverID); err != nil {
		return remoteState{}, err
	}
	return s.remoteOf(binding.ID)
}

func (s *Store) pushOps(ctx context.Context, cli *remoteClient, dim, serverID string, cursor int64, localID, path string) (string, error) {
	ops, err := s.Operations(localID)
	if err != nil {
		return "", err
	}
	if len(ops) == 0 {
		return "", nil
	}
	fileBook, err := vbm.ParseFile(path)
	if err != nil {
		return "", err
	}
	snap, err := s.Snapshot(localID)
	if err != nil {
		return "", err
	}
	fileBy := indexSongs(fileBook.Songs)
	snapBy := indexSongs(snap)
	order := songOrder(fileBook.Songs)
	var report strings.Builder
	base := cursor
	for len(ops) > 0 {
		n := len(ops)
		if n > 100 {
			n = 100
		}
		batch := make([]pushOp, 0, n)
		fbids := map[string]string{}
		for _, op := range ops[:n] {
			song, err := songForOp(op, fileBy, snapBy)
			if err != nil {
				return report.String(), err
			}
			ord := append([]string{}, order...)
			batch = append(batch, pushOp{OperationID: op.OpID, Song: song, Order: &ord})
			fbids[op.OpID] = song.FBID
		}
		results, err := cli.push(ctx, dim, serverID, base, batch)
		if err != nil {
			return report.String(), err
		}
		conflict := false
		for _, result := range results {
			if result.Revision > base {
				base = result.Revision
			}
			if result.Status != "conflict" {
				if result.Status != "applied" {
					return report.String(), fmt.Errorf("ответ %s", result.Status)
				}
				continue
			}
			conflict = true
			fbid := fbids[result.OperationID]
			serverRaw := string(result.Server)
			clientRaw := string(result.Client)
			if err := s.saveConflict(localID, fbid, serverRaw, clientRaw); err != nil {
				return report.String(), err
			}
			report.WriteString(formatConflict(fbid, serverRaw, clientRaw))
		}
		if conflict {
			return report.String(), errUnresolved
		}
		ops = ops[n:]
	}
	return report.String(), nil
}

func songForOp(op Operation, fileBy, snapBy map[string]vbm.Song) (songDTO, error) {
	if op.Kind == "deleted" {
		song, ok := snapBy[strings.ToLower(op.SongFBID)]
		if !ok {
			song = vbm.Song{Properties: setProp("", "$FBID$", op.SongFBID)}
		}
		return songFromVBM(song, true)
	}
	song, ok := fileBy[strings.ToLower(op.SongFBID)]
	if !ok {
		return songDTO{}, fmt.Errorf("нет песни %s", op.SongFBID)
	}
	return songFromVBM(song, false)
}

func (s *Store) pull(ctx context.Context, cli *remoteClient, dim, localID, serverID, path string) error {
	for {
		state, err := s.remoteOf(localID)
		if err != nil {
			return err
		}
		events, err := cli.changes(ctx, dim, state.Cursor)
		if errors.Is(err, errReload) {
			return s.reload(ctx, cli, dim, localID, serverID, path)
		}
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		book, err := vbm.Parse(data)
		if err != nil {
			return err
		}
		snap, err := s.Snapshot(localID)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if ev.SongbookID != serverID {
				continue
			}
			book.Songs = applyEvent(book.Songs, ev)
			snap = applyEvent(snap, ev)
		}
		if err := saveParsed(path, book, data); err != nil {
			return err
		}
		if err := s.SaveSnapshot(localID, snap); err != nil {
			return err
		}
		if err := s.setCursor(localID, events[len(events)-1].Seq); err != nil {
			return err
		}
	}
}

func (s *Store) reload(ctx context.Context, cli *remoteClient, dim, localID, serverID, path string) error {
	snap, err := cli.snapshot(ctx, dim, serverID)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	book, err := vbm.Parse(data)
	if err != nil {
		return err
	}
	old, err := s.Snapshot(localID)
	if err != nil {
		return err
	}
	book.Songs = mergeReload(old, book.Songs, snap.Songs)
	if err := saveParsed(path, book, data); err != nil {
		return err
	}
	live := make([]vbm.Song, 0, len(snap.Songs))
	for _, song := range snap.Songs {
		if song.Deleted {
			continue
		}
		live = append(live, song.vbm())
	}
	if err := s.SaveSnapshot(localID, live); err != nil {
		return err
	}
	return s.setCursor(localID, snap.Cursor)
}

func mergeReload(old, file []vbm.Song, server []songDTO) []vbm.Song {
	oldBy := indexSongs(old)
	fileBy := indexSongs(file)
	var out []vbm.Song
	seen := map[string]bool{}
	for _, dto := range server {
		if dto.Deleted || dto.FBID == "" {
			continue
		}
		seen[strings.ToLower(dto.FBID)] = true
		remote := dto.vbm()
		local, inFile := fileBy[strings.ToLower(dto.FBID)]
		prev, inOld := oldBy[strings.ToLower(dto.FBID)]
		dirty := inFile && (!inOld || !songEqual(prev, local)) && !songEqual(local, remote)
		if dirty {
			out = append(out, local)
			continue
		}
		out = append(out, remote)
	}
	for _, song := range file {
		id, ok, err := songFBID(song.Properties)
		if err != nil || !ok || seen[id] {
			continue
		}
		out = append(out, song)
	}
	return out
}

func applyEvent(songs []vbm.Song, ev changeEvent) []vbm.Song {
	if ev.Body.Song.FBID == "" {
		return songs
	}
	if ev.Body.Song.Deleted {
		songs = removeSong(songs, ev.Body.Song.FBID)
	} else {
		songs = upsertSong(songs, ev.Body.Song.vbm())
	}
	if ev.Body.Order != nil {
		songs = reorderSongs(songs, *ev.Body.Order)
	}
	return songs
}

func (s *Store) Resolve(ctx context.Context, path, fbid, take string, apply bool) (string, error) {
	if take != "server" && take != "local" {
		return "", fmt.Errorf("нужен --take server или local")
	}
	fbid = strings.ToLower(fbid)
	binding, err := s.Bind(path)
	if err != nil {
		return "", err
	}
	row, err := s.oneConflict(binding.ID, fbid)
	if err != nil {
		return "", err
	}
	report := formatConflict(fbid, row.Server, row.Client)
	if !apply {
		return report, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}
	book, err := vbm.Parse(data)
	if err != nil {
		return report, err
	}
	var serverSong songDTO
	var clientSong songDTO
	if row.Server != "" && row.Server != "null" {
		if err := json.Unmarshal([]byte(row.Server), &serverSong); err != nil {
			return report, err
		}
	}
	if row.Client != "" && row.Client != "null" {
		if err := json.Unmarshal([]byte(row.Client), &clientSong); err != nil {
			return report, err
		}
	}
	book.Songs = removeSong(book.Songs, fbid)
	if take == "server" {
		if serverSong.FBID != "" && !serverSong.Deleted {
			book.Songs = upsertSong(book.Songs, serverSong.vbm())
		}
	} else if !clientSong.Deleted && (clientSong.Title != "" || len(clientSong.Parts) > 0 || clientSong.Properties != "") {
		if serverSong.FBID != "" && !serverSong.Deleted {
			book.Songs = upsertSong(book.Songs, serverSong.vbm())
		}
		next, err := newUUID()
		if err != nil {
			return report, err
		}
		clientSong.FBID = next
		clientSong.Deleted = false
		book.Songs = upsertSong(book.Songs, clientSong.vbm())
	}
	if err := saveParsed(path, book, data); err != nil {
		return report, err
	}
	if err := s.clearConflict(binding.ID, fbid); err != nil {
		return report, err
	}
	left, err := s.listConflicts(binding.ID)
	if err != nil {
		return report, err
	}
	if len(left) > 0 {
		return report, nil
	}
	sess, err := loadSession()
	if err != nil {
		return report, err
	}
	cli, err := newClient(sess)
	if err != nil {
		return report, err
	}
	state, err := s.remoteOf(binding.ID)
	if err != nil {
		return report, err
	}
	if state.ServerID == "" {
		return report, nil
	}
	if err := s.pull(ctx, cli, sess.DimensionID, binding.ID, state.ServerID, path); err != nil {
		return report, err
	}
	return report, nil
}

func (s *Store) Build(ctx context.Context, path, bookID string, answers map[string]string, apply bool) (string, error) {
	sess, err := loadSession()
	if err != nil {
		return "", err
	}
	if sess.DimensionID == "" {
		return "", fmt.Errorf("пространство не выбрано")
	}
	cli, err := newClient(sess)
	if err != nil {
		return "", err
	}
	layers, err := cli.loadChain(ctx, sess.DimensionID, bookID)
	if err != nil {
		return "", err
	}
	for i := range layers {
		dec, err := s.loadDecisions(layers[i].ID)
		if err != nil {
			return "", err
		}
		layers[i].Decisions = dec
	}
	leaf := &layers[len(layers)-1]
	if err := applyAnswers(leaf.Decisions, answers); err != nil {
		return "", err
	}
	report, err := buildLocal(path, layers, apply)
	if saveErr := s.saveDecisions(leaf.ID, leaf.Decisions); saveErr != nil {
		return report, saveErr
	}
	return report, err
}

func applyAnswers(dec map[string]string, answers map[string]string) error {
	for id, kind := range answers {
		if !validUUID(id) {
			return fmt.Errorf("ответ")
		}
		if kind != "retain" && kind != "drop" && kind != "suppress" {
			return fmt.Errorf("ответ")
		}
		dec[strings.ToLower(id)] = kind
	}
	return nil
}

func writeAssigned(path string, changes []Change) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	book, err := vbm.Parse(data)
	if err != nil {
		return err
	}
	before := propKey(book.Songs)
	if err := putNewFBIDs(book, changes); err != nil {
		return err
	}
	if propKey(book.Songs) == before {
		return nil
	}
	return saveParsed(path, book, data)
}

func propKey(songs []vbm.Song) string {
	var b strings.Builder
	for _, song := range songs {
		b.WriteString(song.Properties)
		b.WriteByte(0)
	}
	return b.String()
}

func saveParsed(path string, book *vbm.Book, template []byte) error {
	payload, err := vbm.Write(book, template)
	if err != nil {
		return err
	}
	if err := backupModule(path); err != nil {
		return err
	}
	return commitModule(path, payload)
}

func indexSongs(songs []vbm.Song) map[string]vbm.Song {
	out := map[string]vbm.Song{}
	for _, song := range songs {
		id, ok, err := songFBID(song.Properties)
		if err != nil || !ok {
			continue
		}
		out[id] = song
	}
	return out
}

func songOrder(songs []vbm.Song) []string {
	order := make([]string, 0, len(songs))
	for _, song := range songs {
		id, ok, err := songFBID(song.Properties)
		if err != nil || !ok {
			continue
		}
		order = append(order, id)
	}
	return order
}

func upsertSong(songs []vbm.Song, song vbm.Song) []vbm.Song {
	id, ok, err := songFBID(song.Properties)
	if err != nil || !ok {
		return append(songs, song)
	}
	for i := range songs {
		got, ok, err := songFBID(songs[i].Properties)
		if err == nil && ok && got == id {
			songs[i] = song
			return songs
		}
	}
	return append(songs, song)
}

func removeSong(songs []vbm.Song, fbid string) []vbm.Song {
	fbid = strings.ToLower(fbid)
	dst := make([]vbm.Song, 0, len(songs))
	for _, song := range songs {
		id, ok, err := songFBID(song.Properties)
		if err == nil && ok && id == fbid {
			continue
		}
		dst = append(dst, song)
	}
	return dst
}

func reorderSongs(songs []vbm.Song, order []string) []vbm.Song {
	by := indexSongs(songs)
	used := map[string]bool{}
	out := make([]vbm.Song, 0, len(songs))
	for _, id := range order {
		id = strings.ToLower(id)
		song, ok := by[id]
		if !ok || used[id] {
			continue
		}
		used[id] = true
		out = append(out, song)
	}
	for _, song := range songs {
		id, ok, err := songFBID(song.Properties)
		if err != nil || !ok || used[id] {
			if err != nil || !ok {
				out = append(out, song)
			}
			continue
		}
		out = append(out, song)
	}
	return out
}

func formatConflict(fbid, serverRaw, clientRaw string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "конфликт %s\nсервер:\n%sлокально:\n%s", fbid, sideText(serverRaw), sideText(clientRaw))
	return b.String()
}

func formatConflictRows(rows []conflictRow) string {
	var b strings.Builder
	for _, row := range rows {
		b.WriteString(formatConflict(row.FBID, row.Server, row.Client))
	}
	return b.String()
}

func sideText(raw string) string {
	if raw == "" || raw == "null" {
		return "нет песни\n"
	}
	var song songDTO
	if err := json.Unmarshal([]byte(raw), &song); err != nil {
		return raw + "\n"
	}
	var b strings.Builder
	if song.Deleted {
		b.WriteString("удаление\n")
	}
	fmt.Fprintf(&b, "%s\n", song.Title)
	for _, part := range song.Parts {
		fmt.Fprintf(&b, "%s\n", part.Text)
	}
	return b.String()
}

func (s *Store) linkBook(localID, serverID string) error {
	_, err := s.db.Exec(`
INSERT INTO remote_books(book_id, server_id, cursor, loaded) VALUES(?, ?, 0, 0)
ON CONFLICT(book_id) DO NOTHING
`, localID, serverID)
	if err != nil {
		return err
	}
	var got string
	if err := s.db.QueryRow(`SELECT server_id FROM remote_books WHERE book_id = ?`, localID).Scan(&got); err != nil {
		return err
	}
	if got != serverID {
		return fmt.Errorf("книга привязана к другому сборнику")
	}
	return nil
}

func (s *Store) remoteOf(bookID string) (remoteState, error) {
	var state remoteState
	var loaded int
	err := s.db.QueryRow(`SELECT server_id, cursor, loaded FROM remote_books WHERE book_id = ?`, bookID).
		Scan(&state.ServerID, &state.Cursor, &loaded)
	if err == sql.ErrNoRows {
		return remoteState{}, nil
	}
	if err != nil {
		return remoteState{}, err
	}
	state.Loaded = loaded != 0
	return state, nil
}

func (s *Store) setCursor(bookID string, cursor int64) error {
	_, err := s.db.Exec(`UPDATE remote_books SET cursor = ? WHERE book_id = ?`, cursor, bookID)
	return err
}

func (s *Store) markLoaded(bookID string) error {
	_, err := s.db.Exec(`UPDATE remote_books SET loaded = 1 WHERE book_id = ?`, bookID)
	return err
}

func (s *Store) saveConflict(bookID, fbid, serverRaw, clientRaw string) error {
	_, err := s.db.Exec(`
INSERT INTO song_conflicts(book_id, song_fbid, server_body, client_body) VALUES(?, ?, ?, ?)
ON CONFLICT(book_id, song_fbid) DO UPDATE SET
  server_body = excluded.server_body,
  client_body = excluded.client_body
`, bookID, strings.ToLower(fbid), serverRaw, clientRaw)
	return err
}

func (s *Store) listConflicts(bookID string) ([]conflictRow, error) {
	rows, err := s.db.Query(`
SELECT song_fbid, server_body, client_body FROM song_conflicts WHERE book_id = ? ORDER BY song_fbid
`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []conflictRow
	for rows.Next() {
		var row conflictRow
		if err := rows.Scan(&row.FBID, &row.Server, &row.Client); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if out == nil {
		out = []conflictRow{}
	}
	return out, rows.Err()
}

func (s *Store) oneConflict(bookID, fbid string) (conflictRow, error) {
	var row conflictRow
	err := s.db.QueryRow(`
SELECT song_fbid, server_body, client_body FROM song_conflicts WHERE book_id = ? AND song_fbid = ?
`, bookID, fbid).Scan(&row.FBID, &row.Server, &row.Client)
	if err == sql.ErrNoRows {
		return conflictRow{}, fmt.Errorf("конфликт не найден")
	}
	return row, err
}

func (s *Store) clearConflict(bookID, fbid string) error {
	_, err := s.db.Exec(`DELETE FROM song_conflicts WHERE book_id = ? AND song_fbid = ?`, bookID, fbid)
	return err
}

func (s *Store) loadDecisions(serverID string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT song_fbid, decision FROM patch_decisions WHERE server_id = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, kind string
		if err := rows.Scan(&id, &kind); err != nil {
			return nil, err
		}
		out[id] = kind
	}
	return out, rows.Err()
}

func (s *Store) saveDecisions(serverID string, dec map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM patch_decisions WHERE server_id = ?`, serverID); err != nil {
		return err
	}
	for id, kind := range dec {
		if _, err := tx.Exec(`
INSERT INTO patch_decisions(server_id, song_fbid, decision) VALUES(?, ?, ?)
`, serverID, id, kind); err != nil {
			return err
		}
	}
	return tx.Commit()
}
