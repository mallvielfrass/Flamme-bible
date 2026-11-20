package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 2
	argonMemory  = 19 * 1024
	argonThreads = 1
	argonKeyLen  = 32
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	), nil
}

func checkPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return false
	}
	var memory, timeCost, threads int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return false
	}
	if memory != argonMemory || timeCost != argonTime || threads != argonThreads {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	sum, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(timeCost), uint32(memory), uint8(threads), uint32(len(sum)))
	return subtle.ConstantTimeCompare(got, sum) == 1
}

func newSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func tokenDigest(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func bearer(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	DeviceID string `json:"device_id"`
}

func validCredentials(body credentials) bool {
	email := strings.TrimSpace(body.Email)
	if email == "" || len(email) > 320 || body.Password == "" || len(body.Password) > 1024 {
		return false
	}
	if body.DeviceID != "" {
		if _, err := uuid.Parse(body.DeviceID); err != nil {
			return false
		}
	}
	return true
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := readJSON(r, &body); err != nil || !validCredentials(body) {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	hash, err := hashPassword(body.Password)
	if err != nil {
		writeMapped(w, err)
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	userID := uuid.NewString()
	_, err = tx.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`, userID, strings.TrimSpace(body.Email), hash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSON(w, http.StatusConflict, errJSON("email"))
			return
		}
		writeMapped(w, err)
		return
	}
	dimID, err := createDimension(ctx, tx, "personal", "", "", "", "", "", userID, "admin")
	if err != nil {
		writeMapped(w, err)
		return
	}
	token, deviceID, err := issueSession(ctx, tx, userID, body.DeviceID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id":      userID,
		"device_id":    deviceID,
		"dimension_id": dimID,
		"token":        token,
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := readJSON(r, &body); err != nil || !validCredentials(body) {
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
		return
	}
	ctx := r.Context()
	var userID, hash string
	err := s.db.QueryRow(ctx, `SELECT id::text, password_hash FROM users WHERE email = $1`, strings.TrimSpace(body.Email)).Scan(&userID, &hash)
	if err != nil || !checkPassword(hash, body.Password) {
		writeJSON(w, http.StatusUnauthorized, errJSON("не авторизован"))
		return
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	token, deviceID, err := issueSession(ctx, tx, userID, body.DeviceID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	var dimID string
	err = tx.QueryRow(ctx, `
		SELECT d.id::text
		FROM dimensions d
		JOIN memberships m ON m.dimension_id = d.id
		WHERE m.user_id = $1 AND d.kind = 'personal' AND NOT m.excluded
	`, userID).Scan(&dimID)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id":      userID,
		"device_id":    deviceID,
		"dimension_id": dimID,
		"token":        token,
	})
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, errJSON("не авторизован"))
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var familyID string
	var replaced, revoked *time.Time
	err = tx.QueryRow(ctx, `
		SELECT f.id::text, t.replaced_at, f.revoked_at
		FROM session_tokens t
		JOIN session_families f ON f.id = t.family_id
		WHERE t.digest = $1
		FOR UPDATE OF t, f
	`, tokenDigest(token)).Scan(&familyID, &replaced, &revoked)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && revoked != nil) {
		writeJSON(w, http.StatusUnauthorized, errJSON("не авторизован"))
		return
	}
	if err != nil {
		writeMapped(w, err)
		return
	}
	if replaced != nil {
		if _, err := tx.Exec(ctx, `UPDATE session_families SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, familyID); err != nil {
			writeMapped(w, err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeMapped(w, err)
			return
		}
		writeJSON(w, http.StatusUnauthorized, errJSON("не авторизован"))
		return
	}
	next, err := newSecret()
	if err != nil {
		writeMapped(w, err)
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE session_tokens SET replaced_at = now() WHERE digest = $1 AND replaced_at IS NULL`, tokenDigest(token))
	if err != nil {
		writeMapped(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeJSON(w, http.StatusUnauthorized, errJSON("не авторизован"))
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO session_tokens (digest, family_id) VALUES ($1, $2)`, tokenDigest(next), familyID); err != nil {
		writeMapped(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": next})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE session_families SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, p.FamilyID); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) principal(r *http.Request) (principal, error) {
	token := bearer(r)
	if token == "" {
		return principal{}, errUnauthorized
	}
	var p principal
	var replaced, revoked *time.Time
	err := s.db.QueryRow(r.Context(), `
		SELECT f.id::text, f.user_id::text, f.device_id::text, t.replaced_at, f.revoked_at
		FROM session_tokens t
		JOIN session_families f ON f.id = t.family_id
		WHERE t.digest = $1
	`, tokenDigest(token)).Scan(&p.FamilyID, &p.UserID, &p.DeviceID, &replaced, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return principal{}, errUnauthorized
	}
	if err != nil {
		return principal{}, err
	}
	if replaced != nil || revoked != nil {
		return principal{}, errUnauthorized
	}
	return p, nil
}

func issueSession(ctx context.Context, tx pgx.Tx, userID, deviceID string) (string, string, error) {
	if deviceID == "" {
		deviceID = uuid.NewString()
		if _, err := tx.Exec(ctx, `INSERT INTO devices (id, user_id) VALUES ($1, $2)`, deviceID, userID); err != nil {
			return "", "", err
		}
	} else {
		var owner string
		err := tx.QueryRow(ctx, `SELECT user_id::text FROM devices WHERE id = $1`, deviceID).Scan(&owner)
		if errors.Is(err, pgx.ErrNoRows) {
			if _, err := tx.Exec(ctx, `INSERT INTO devices (id, user_id) VALUES ($1, $2)`, deviceID, userID); err != nil {
				return "", "", err
			}
		} else if err != nil {
			return "", "", err
		} else if owner != userID {
			return "", "", errForbidden
		}
	}
	secret, err := newSecret()
	if err != nil {
		return "", "", err
	}
	familyID := uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO session_families (id, user_id, device_id) VALUES ($1, $2, $3)`, familyID, userID, deviceID); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO session_tokens (digest, family_id) VALUES ($1, $2)`, tokenDigest(secret), familyID); err != nil {
		return "", "", err
	}
	return secret, deviceID, nil
}
