package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

const maxBody = 1 << 20

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
)

type Server struct {
	db *pgxpool.Pool
}

type principal struct {
	FamilyID string
	UserID   string
	DeviceID string
}

func New(db *pgxpool.Pool) *Server {
	return &Server{db: db}
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
	return s.dispatch(mux)
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
	default:
		log.Printf("server: %v", err)
		writeJSON(w, http.StatusInternalServerError, errJSON("ошибка"))
	}
}
