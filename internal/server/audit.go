package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func writeAudit(ctx context.Context, tx pgx.Tx, actor, action, kind, id, title, note string) error {
	var actorID any
	if strings.TrimSpace(actor) != "" {
		actorID = actor
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO admin_audit (id, actor_id, action, target_kind, target_id, target_title, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.NewString(), actorID, action, kind, id, title, note)
	return err
}

type auditEntry struct {
	ID          string    `json:"id"`
	At          time.Time `json:"at"`
	ActorID     *string   `json:"actor_id"`
	ActorEmail  string    `json:"actor_email"`
	Action      string    `json:"action"`
	TargetKind  string    `json:"target_kind"`
	TargetID    string    `json:"target_id"`
	TargetTitle string    `json:"target_title"`
	Note        string    `json:"note"`
	Summary     string    `json:"summary"`
}

func auditText(action, kind, title, note string) string {
	what := map[string]string{
		"songbook": "сборник", "module": "модуль", "user": "пользователя",
		"release": "сборку", "community": "общину", "member": "участника", "invite": "приглашение",
		"embed": "индексацию", "ai": "ИИ",
	}[kind]
	if what == "" {
		what = kind
	}
	name := title
	if name != "" {
		name = "«" + name + "»"
	}
	verb := map[string]string{
		"approve": "одобрил", "reject": "отклонил", "revoke": "снял с публикации",
		"hide": "скрыл", "unhide": "вернул", "delete": "удалил",
		"role": "сменил роль", "block": "заблокировал", "unblock": "разблокировал",
		"sessions_revoke": "завершил сессии",
		"release_upload":  "загрузил", "release_publish": "опубликовал",
		"release_rollback": "вернул людям", "release_delete": "удалил черновик",
		"release_notes":  "поправил заметки",
		"member_exclude": "исключил", "member_include": "вернул", "member_role": "сменил роль",
		"invite_revoke": "отозвал", "publish_decision": "решил публикацию",
		"embed_settings": "изменил скорость", "embed_retry": "повторил",
		"embed_reindex": "запустил пересчёт", "embed_switch": "переключил индекс",
		"embed_model": "выбрал модель", "embed_model_reset": "вернул модель",
		"embed_gguf":  "загрузил модель",
		"ai_settings": "изменил настройки",
	}[action]
	if verb == "" {
		verb = action
	}
	text := strings.TrimSpace(verb + " " + what + " " + name)
	if note != "" {
		text += " — " + note
	}
	return text
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireModerator(w, r); !ok {
		return
	}
	actor := strings.TrimSpace(r.URL.Query().Get("actor"))
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	kind := strings.TrimSpace(r.URL.Query().Get("target_kind"))
	limit, offset, err := pageWindow(r, 50, 200)
	if err != nil {
		writeMapped(w, err)
		return
	}
	where := []string{"TRUE"}
	args := []any{}
	if actor != "" {
		args = append(args, actor)
		if _, err := uuid.Parse(actor); err == nil {
			where = append(where, fmt.Sprintf("a.actor_id = $%d", len(args)))
		} else {
			args[len(args)-1] = likeContains(actor)
			where = append(where, fmt.Sprintf("u.email ILIKE $%d", len(args)))
		}
	}
	if action != "" {
		args = append(args, action)
		where = append(where, fmt.Sprintf("a.action = $%d", len(args)))
	}
	if kind != "" {
		args = append(args, kind)
		where = append(where, fmt.Sprintf("a.target_kind = $%d", len(args)))
	}
	filter := strings.Join(where, " AND ")
	ctx := r.Context()
	var total int
	if err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM admin_audit a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE `+filter, args...).Scan(&total); err != nil {
		writeMapped(w, err)
		return
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, a.at, a.actor_id::text, COALESCE(u.email, ''),
		       a.action, a.target_kind, a.target_id, a.target_title, a.note
		FROM admin_audit a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE `+filter+`
		ORDER BY a.at DESC, a.id
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []auditEntry{}
	for rows.Next() {
		var item auditEntry
		if err := rows.Scan(&item.ID, &item.At, &item.ActorID, &item.ActorEmail, &item.Action, &item.TargetKind, &item.TargetID, &item.TargetTitle, &item.Note); err != nil {
			writeMapped(w, err)
			return
		}
		item.At = item.At.UTC()
		item.Summary = auditText(item.Action, item.TargetKind, item.TargetTitle, item.Note)
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": list, "total": total})
}

func (s *Server) recentAudit(ctx context.Context, targetID string) ([]auditEntry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, a.at, a.actor_id::text, COALESCE(u.email, ''),
		       a.action, a.target_kind, a.target_id, a.target_title, a.note
		FROM admin_audit a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.target_id = $1
		ORDER BY a.at DESC
		LIMIT 5
	`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []auditEntry{}
	for rows.Next() {
		var item auditEntry
		if err := rows.Scan(&item.ID, &item.At, &item.ActorID, &item.ActorEmail, &item.Action, &item.TargetKind, &item.TargetID, &item.TargetTitle, &item.Note); err != nil {
			return nil, err
		}
		item.At = item.At.UTC()
		item.Summary = auditText(item.Action, item.TargetKind, item.TargetTitle, item.Note)
		list = append(list, item)
	}
	return list, rows.Err()
}
