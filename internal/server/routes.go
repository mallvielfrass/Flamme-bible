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
	mux.HandleFunc("GET /api/communities/{id}/members", s.communityMembers)
	mux.HandleFunc("POST /api/communities/{id}/members/{userID}/exclude", s.apiExcludeMember)
	mux.HandleFunc("POST /api/communities/{id}/members/{userID}/include", s.apiIncludeMember)
	mux.HandleFunc("POST /api/communities/{id}/members/{userID}/role", s.setMemberRole)
	mux.HandleFunc("GET /api/communities/{id}/invites", s.listInvites)
	mux.HandleFunc("DELETE /api/communities/{id}/invites/{digest}", s.revokeInvite)
	mux.HandleFunc("GET /api/admin/communities", s.adminCommunities)
	mux.HandleFunc("DELETE /api/admin/communities/{id}", s.adminDeleteCommunity)
	mux.HandleFunc("GET /api/admin/communities/{id}/members", s.communityMembers)
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

	mux.HandleFunc("GET /api/songbooks/tree", s.songbookTree)
	mux.HandleFunc("GET /api/songbooks/{id}/sections", s.listSections)
	mux.HandleFunc("POST /api/songbooks/{id}/sections", s.createSection)
	mux.HandleFunc("PUT /api/songbooks/{id}/sections/{sectionID}", s.renameSection)
	mux.HandleFunc("DELETE /api/songbooks/{id}/sections/{sectionID}", s.deleteSection)
	mux.HandleFunc("PUT /api/sections/{id}/songs", s.setSectionSongs)

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
	mux.HandleFunc("GET /api/admin/summary", s.adminSummary)
	mux.HandleFunc("GET /api/admin/users", s.adminUsers)
	mux.HandleFunc("POST /api/admin/users/{id}/role", s.adminSetRole)
	mux.HandleFunc("POST /api/admin/users/{id}/block", s.adminBlockUser)
	mux.HandleFunc("POST /api/admin/users/{id}/unblock", s.adminUnblockUser)
	mux.HandleFunc("POST /api/admin/users/{id}/sessions/revoke", s.adminRevokeSessions)
	mux.HandleFunc("DELETE /api/admin/users/{id}", s.adminDeleteUser)
	mux.HandleFunc("GET /api/admin/songbooks/{id}", s.adminSongbookView)
	mux.HandleFunc("GET /api/admin/modules/{id}", s.adminModuleView)
	mux.HandleFunc("DELETE /api/admin/songbooks/{id}", s.adminDeleteSongbook)
	mux.HandleFunc("POST /api/admin/songbooks/{id}/unhide", s.adminUnhideSongbook)
	mux.HandleFunc("POST /api/admin/songbooks/{id}/approve", s.adminApproveSongbook)
	mux.HandleFunc("POST /api/admin/songbooks/{id}/reject", s.adminRejectSongbook)
	mux.HandleFunc("POST /api/admin/songbooks/{id}/revoke", s.adminRevokeSongbook)
	mux.HandleFunc("DELETE /api/admin/modules/{id}", s.adminDeleteModule)
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
