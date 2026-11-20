package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxBody           = 1 << 20
	defaultSessionTTL = 720 * time.Hour
	cleanupEvery      = time.Minute
)

var (
	errUnauthorized = errors.New("не авторизован")
	errForbidden    = errors.New("запрещено")
	errNotFound     = errors.New("не найдено")
	errCycle        = errors.New("цикл")
	errDepth        = errors.New("глубина")
	errOpBody       = errors.New("operation_id")
	errReload       = errors.New("reload")
	errMember       = errors.New("уже участник")
	errConflict     = errors.New("конфликт")
	errRequest      = errors.New("запрос")
	errUnmappable   = errors.New("windows-1251")
)

type Server struct {
	db         *pgxpool.Pool
	sessionTTL time.Duration
	trustProxy bool
	authLimit  *ipLimiter
	cleanMu    sync.Mutex
	lastClean  time.Time
}

type principal struct {
	FamilyID string
	UserID   string
	DeviceID string
}

func New(db *pgxpool.Pool) *Server {
	return &Server{
		db:         db,
		sessionTTL: defaultSessionTTL,
		authLimit:  newAuthLimiter(),
	}
}

func (s *Server) SetSessionTTL(d time.Duration) {
	if d > 0 {
		s.sessionTTL = d
	}
}

func (s *Server) SetTrustProxy(trust bool) {
	s.trustProxy = trust
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.mountMVP(mux)
	mux.HandleFunc("POST /register", s.register)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /refresh", s.refresh)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("POST /dimensions", s.createCommunity)
	mux.HandleFunc("GET /dimensions/{id}", s.getDimension)
	mux.HandleFunc("POST /dimensions/{id}/invites", s.createInvite)
	mux.HandleFunc("POST /invites/{token}/accept", s.acceptInvite)
	mux.HandleFunc("POST /dimensions/{id}/members/{userID}/exclude", s.excludeMember)
	mux.HandleFunc("POST /dimensions/{id}/songbooks", s.createSongbook)
	mux.HandleFunc("GET /dimensions/{id}/songbooks", s.listSongbooks)
	mux.HandleFunc("POST /songbooks/{id}/hide", s.hideSongbook)
	mux.HandleFunc("POST /songbooks/{id}/parent", s.setParent)
	mux.HandleFunc("POST /dimensions/{id}/songbooks/{bookID}/push", s.push)
	mux.HandleFunc("GET /dimensions/{id}/songbooks/{bookID}/snapshot", s.snapshot)
	mux.HandleFunc("GET /dimensions/{id}/changes", s.changes)
	mux.HandleFunc("POST /songbooks/{id}/publish", s.requestPublish)
	mux.HandleFunc("POST /songbooks/{id}/publish/decision", s.decidePublish)
	mux.HandleFunc("GET /catalog/matches", s.catalogMatches)
	mux.HandleFunc("POST /catalog/hide", s.hideCatalogCopy)
	return s.dispatch(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applySecurityHeaders(w, r)
		if r.Method == http.MethodPost && isAuthPath(r.URL.Path) && !s.authLimit.allow(s.clientIP(r), r.URL.Path, time.Now()) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, errJSON("слишком много запросов"))
			return
		}
		s.maybeCleanup(r.Context())
		mux.ServeHTTP(w, r)
	}))
}

func isAuthPath(path string) bool {
	switch path {
	case "/register", "/login", "/refresh":
		return true
	default:
		return false
	}
}

func applySecurityHeaders(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	h.Set("Cache-Control", "no-store")
	if r.TLS != nil {
		h.Set("Strict-Transport-Security", "max-age=15552000")
	}
}

func (s *Server) clientIP(r *http.Request) string {
	if s.trustProxy {
		if ip := parseIP(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
	}
	if ip := parseIP(r.RemoteAddr); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

func parseIP(value string) string {
	value = strings.TrimSpace(strings.Split(value, ",")[0])
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	if net.ParseIP(value) == nil {
		return ""
	}
	return value
}

func (s *Server) maybeCleanup(ctx context.Context) {
	s.cleanMu.Lock()
	if !s.lastClean.IsZero() && time.Since(s.lastClean) < cleanupEvery {
		s.cleanMu.Unlock()
		return
	}
	s.lastClean = time.Now()
	s.cleanMu.Unlock()
	if err := CleanupSessions(ctx, s.db); err != nil {
		log.Printf("сессии: %v", err)
	}
}

func CleanupSessions(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `DELETE FROM session_tokens WHERE expires_at IS NULL OR expires_at <= now()`); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `
		DELETE FROM session_families f
		WHERE NOT EXISTS (SELECT 1 FROM session_tokens t WHERE t.family_id = f.id)
	`)
	return err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(msg string) map[string]string {
	return map[string]string{"error": msg}
}

func readJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func writeMapped(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errUnauthorized):
		writeJSON(w, http.StatusUnauthorized, errJSON("не авторизован"))
	case errors.Is(err, errForbidden):
		writeJSON(w, http.StatusForbidden, errJSON("запрещено"))
	case errors.Is(err, errNotFound):
		writeJSON(w, http.StatusNotFound, errJSON("не найдено"))
	case errors.Is(err, errCycle):
		writeJSON(w, http.StatusConflict, errJSON("цикл"))
	case errors.Is(err, errDepth):
		writeJSON(w, http.StatusConflict, errJSON("глубина"))
	case errors.Is(err, errOpBody):
		writeJSON(w, http.StatusConflict, errJSON("operation_id"))
	case errors.Is(err, errReload):
		writeJSON(w, http.StatusConflict, errJSON("reload"))
	case errors.Is(err, errMember):
		writeJSON(w, http.StatusConflict, errJSON("уже участник"))
	case errors.Is(err, errConflict):
		writeJSON(w, http.StatusConflict, errJSON("конфликт"))
	case errors.Is(err, errRequest):
		writeJSON(w, http.StatusBadRequest, errJSON("запрос"))
	case errors.Is(err, errUnmappable):
		writeJSON(w, http.StatusBadRequest, errJSON("в тексте есть символы, которых нет в windows-1251"))
	default:
		log.Printf("server: %v", err)
		writeJSON(w, http.StatusInternalServerError, errJSON("ошибка"))
	}
}
