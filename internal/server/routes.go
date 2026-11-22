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
	mux.HandleFunc("GET /api/songs/text", s.songTextHandler)
	mux.HandleFunc("GET /api/bible/verse", s.verseTextHandler)
	mux.HandleFunc("GET /api/bible/semantic", s.searchBibleSemantic)
	mux.HandleFunc("GET /api/bible/translations", s.bibleTranslations)
	mux.HandleFunc("GET /api/bible/books", s.bibleBooks)
	mux.HandleFunc("GET /api/bible/chapter", s.bibleChapterHandler)
	mux.HandleFunc("GET /api/bible/search", s.searchBibleWords)

	mux.HandleFunc("POST /api/modules", s.createModule)
	mux.HandleFunc("POST /api/modules/exists", s.bibleModuleExists)
	mux.HandleFunc("POST /api/modules/upload", s.uploadBibleModule)
	mux.HandleFunc("GET /api/modules", s.listModules)
	mux.HandleFunc("GET /api/modules/{id}", s.getModule)
	mux.HandleFunc("POST /api/modules/{id}/submit", s.submitModule)
	mux.HandleFunc("POST /api/songbooks/{id}/submit", s.submitSongbook)
	mux.HandleFunc("GET /api/catalog/songbooks", s.publicSongbooks)
	mux.HandleFunc("GET /api/catalog/modules", s.publicModules)
	mux.HandleFunc("GET /api/admin/embed", s.adminEmbedQueue)
	mux.HandleFunc("PUT /api/admin/embed", s.adminPutEmbedSettings)
	mux.HandleFunc("POST /api/admin/embed/retry", s.adminRetryEmbed)
	mux.HandleFunc("POST /api/admin/embed/reindex", s.adminReindexEmbed)
	mux.HandleFunc("PUT /api/admin/embed/model", s.adminSetEmbedModel)
	mux.HandleFunc("POST /api/admin/embed/model/reset", s.adminResetEmbedModel)
	mux.HandleFunc("POST /api/admin/embed/gguf", s.adminUploadGGUF)
	mux.HandleFunc("GET /api/admin/submissions", s.adminSubmissions)
	mux.HandleFunc("GET /api/admin/summary", s.adminSummary)
	mux.HandleFunc("GET /api/admin/audit", s.adminAudit)
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
	mux.HandleFunc("PATCH /api/admin/releases/{version}", s.patchReleaseNotes)
	mux.HandleFunc("GET /api/admin/releases/{version}/download/{platform}", s.adminDownloadRelease)
	mux.HandleFunc("POST /api/admin/releases/{version}/publish", s.publishRelease)
	mux.HandleFunc("DELETE /api/admin/releases/{version}", s.deleteRelease)
	mux.HandleFunc("GET /api/releases/latest", s.latestRelease)
	mux.HandleFunc("GET /api/releases/{version}/download/{platform}", s.downloadRelease)

	mux.HandleFunc("GET /api/ai/providers", s.listAIProviders)
	mux.HandleFunc("POST /api/ai/providers", s.createAIProvider)
	mux.HandleFunc("PATCH /api/ai/providers/{id}", s.patchAIProvider)
	mux.HandleFunc("PUT /api/ai/providers/{id}/token", s.putAIProviderToken)
	mux.HandleFunc("DELETE /api/ai/providers/{id}", s.deleteAIProvider)
	mux.HandleFunc("POST /api/ai/providers/{id}/test", s.testAIProvider)
	mux.HandleFunc("GET /api/ai/providers/{id}/remote-models", s.remoteAIModels)
	mux.HandleFunc("PUT /api/ai/providers/{id}/models", s.putAIModels)
	mux.HandleFunc("PUT /api/ai/tasks", s.putAITasks)

	mux.HandleFunc("GET /api/ai/chats", s.listAIChats)
	mux.HandleFunc("POST /api/ai/chats", s.createAIChat)
	mux.HandleFunc("GET /api/ai/chats/{id}", s.getAIChat)
	mux.HandleFunc("PATCH /api/ai/chats/{id}", s.patchAIChat)
	mux.HandleFunc("DELETE /api/ai/chats/{id}", s.deleteAIChat)
	mux.HandleFunc("POST /api/ai/chats/{id}/messages", s.postAIMessage)
	mux.HandleFunc("POST /api/ai/chats/{id}/stop", s.stopAIChat)

	mux.HandleFunc("GET /api/admin/ai", s.adminAI)
	mux.HandleFunc("PUT /api/admin/ai", s.adminPutAI)
}

// JSON-маршруты остаются на maxBody. Релиз, модуль, GGUF и вопрос чата
// с картинками больше этого предела, поэтому у их путей лимит свой.
func (s *Server) dispatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(maxBody)
		if r.Method == http.MethodPost && r.URL.Path == "/api/admin/releases" {
			limit = maxReleaseBody
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/admin/embed/gguf" {
			limit = ggufMaxBytes()
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/modules/upload" {
			limit = s.moduleBodyLimit()
		}
		if r.Method == http.MethodPost && isAIMessagePath(r.URL.Path) {
			limit = maxAIMessageBody
		}
		http.MaxBytesHandler(next, limit).ServeHTTP(w, r)
	})
}
