package server

import "net/http"

func (s *Server) mountMVP(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/me", s.me)

	mux.HandleFunc("POST /api/dimensions", s.createCommunity)
	mux.HandleFunc("GET /api/dimensions", s.listDimensions)
	mux.HandleFunc("GET /api/dimensions/{id}", s.getDimension)
	mux.HandleFunc("POST /api/dimensions/{id}/invites", s.createInvite)

	mux.HandleFunc("POST /api/communities", s.createCommunity)
	mux.HandleFunc("GET /api/communities", s.listCommunities)
	mux.HandleFunc("GET /api/communities/{id}", s.getDimension)
	mux.HandleFunc("POST /api/communities/{id}/invites", s.createInvite)
	mux.HandleFunc("POST /api/invites/{token}/accept", s.acceptInvite)

	mux.HandleFunc("POST /api/plans", s.createPlan)
	mux.HandleFunc("GET /api/plans", s.listPlans)
	mux.HandleFunc("POST /api/plans/import", s.importPlan)
	mux.HandleFunc("POST /api/plans/parse", s.parsePlan)
	mux.HandleFunc("GET /api/plans/{id}", s.getPlan)
	mux.HandleFunc("PUT /api/plans/{id}", s.updatePlan)
	mux.HandleFunc("DELETE /api/plans/{id}", s.deletePlan)
	mux.HandleFunc("POST /api/plans/{id}/push", s.pushPlan)
	mux.HandleFunc("GET /api/plans/{id}/pull", s.pullPlan)
	mux.HandleFunc("GET /api/plans/{id}/export", s.exportPlan)

	mux.HandleFunc("GET /api/songs/search", s.searchSongs)
	mux.HandleFunc("GET /api/songs/semantic", s.searchSongsSemantic)
	mux.HandleFunc("GET /api/bible/semantic", s.searchBibleSemantic)

	mux.HandleFunc("POST /api/modules", s.createModule)
	mux.HandleFunc("GET /api/modules", s.listModules)
	mux.HandleFunc("GET /api/modules/{id}", s.getModule)
	mux.HandleFunc("POST /api/modules/{id}/submit", s.submitModule)
	mux.HandleFunc("POST /api/songbooks/{id}/submit", s.submitSongbook)
	mux.HandleFunc("GET /api/catalog/songbooks", s.publicSongbooks)
	mux.HandleFunc("GET /api/catalog/modules", s.publicModules)
	mux.HandleFunc("GET /api/admin/submissions", s.adminSubmissions)
	mux.HandleFunc("POST /api/admin/songbooks/{id}/approve", s.adminApproveSongbook)
	mux.HandleFunc("POST /api/admin/songbooks/{id}/reject", s.adminRejectSongbook)
	mux.HandleFunc("POST /api/admin/songbooks/{id}/revoke", s.adminRevokeSongbook)
	mux.HandleFunc("POST /api/admin/modules/{id}/approve", s.adminApproveModule)
	mux.HandleFunc("POST /api/admin/modules/{id}/reject", s.adminRejectModule)
	mux.HandleFunc("POST /api/admin/modules/{id}/revoke", s.adminRevokeModule)

	mux.HandleFunc("POST /api/admin/releases", s.uploadRelease)
	mux.HandleFunc("GET /api/admin/releases", s.listReleases)
	mux.HandleFunc("POST /api/admin/releases/{version}/publish", s.publishRelease)
	mux.HandleFunc("DELETE /api/admin/releases/{version}", s.deleteRelease)
	mux.HandleFunc("GET /api/releases/latest", s.latestRelease)
	mux.HandleFunc("GET /api/releases/{version}/download/{platform}", s.downloadRelease)
}

// JSON-маршруты остаются на maxBody. Бинарник релиза больше этого предела,
// поэтому для одного фиксированного пути лимит отдельный.
func (s *Server) dispatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(maxBody)
		if r.Method == http.MethodPost && r.URL.Path == "/api/admin/releases" {
			limit = maxReleaseBody
		}
		http.MaxBytesHandler(next, limit).ServeHTTP(w, r)
	})
}
