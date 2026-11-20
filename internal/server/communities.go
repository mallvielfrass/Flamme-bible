package server

import (
	"net/http"
)

func (s *Server) listDimensions(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	s.writeDimensions(w, r, p.UserID, "")
}

func (s *Server) listCommunities(w http.ResponseWriter, r *http.Request) {
	p, err := s.principal(r)
	if err != nil {
		writeMapped(w, err)
		return
	}
	s.writeDimensions(w, r, p.UserID, "community")
}

func (s *Server) writeDimensions(w http.ResponseWriter, r *http.Request, userID, kind string) {
	q := `
		SELECT d.id::text, d.kind, d.name, d.address, d.email, d.confession, d.description
		FROM dimensions d
		JOIN memberships m ON m.dimension_id = d.id
		WHERE m.user_id = $1 AND NOT m.excluded`
	args := []any{userID}
	if kind != "" {
		q += ` AND d.kind = $2`
		args = append(args, kind)
	}
	q += ` ORDER BY d.kind, d.name, d.id`
	rows, err := s.db.Query(r.Context(), q, args...)
	if err != nil {
		writeMapped(w, err)
		return
	}
	defer rows.Close()
	list := []dimensionView{}
	for rows.Next() {
		var view dimensionView
		if err := rows.Scan(&view.ID, &view.Kind, &view.Name, &view.Address, &view.Email, &view.Confession, &view.Description); err != nil {
			writeMapped(w, err)
			return
		}
		list = append(list, view)
	}
	if err := rows.Err(); err != nil {
		writeMapped(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
