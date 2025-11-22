package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/123jjck/avito-trainee-assignment/internal/models"
	"github.com/123jjck/avito-trainee-assignment/internal/service"
)

type Server struct {
	svc *service.Service
	mux *http.ServeMux
}

func New(svc *service.Service) *Server {
	s := &Server{
		svc: svc,
		mux: http.NewServeMux(),
	}

	s.mux.HandleFunc("/health", s.healthHandler)
	s.mux.HandleFunc("/team/add", s.teamAddHandler)
	s.mux.HandleFunc("/team/get", s.teamGetHandler)
	s.mux.HandleFunc("/users/setIsActive", s.setActiveHandler)
	s.mux.HandleFunc("/pullRequest/create", s.prCreateHandler)
	s.mux.HandleFunc("/pullRequest/merge", s.prMergeHandler)
	s.mux.HandleFunc("/pullRequest/reassign", s.prReassignHandler)
	s.mux.HandleFunc("/users/getReview", s.userReviewsHandler)
	s.mux.HandleFunc("/stats", s.statsHandler)

	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) teamAddHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.Team
	if err := decodeJSON(r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	teamReq, err := sanitizeTeam(req)
	if err != nil {
		writeDecodeError(w, err)
		return
	}

	team, err := s.svc.CreateTeam(r.Context(), teamReq)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"team": team})
}

func (s *Server) teamGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	teamName := r.URL.Query().Get("team_name")
	if teamName == "" {
		writeDecodeError(w, errors.New("team_name query parameter is required"))
		return
	}
	team, err := s.svc.GetTeam(r.Context(), teamName)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, team)
}

func (s *Server) setActiveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		UserID   string `json:"user_id"`
		IsActive bool   `json:"is_active"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" {
		writeDecodeError(w, errors.New("user_id is required"))
		return
	}

	user, err := s.svc.SetUserActive(r.Context(), req.UserID, req.IsActive)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) prCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID     string `json:"pull_request_id"`
		Name   string `json:"pull_request_name"`
		Author string `json:"author_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.Author = strings.TrimSpace(req.Author)
	if req.ID == "" || req.Name == "" || req.Author == "" {
		writeDecodeError(w, errors.New("pull_request_id, pull_request_name and author_id are required"))
		return
	}

	pr, err := s.svc.CreatePullRequest(r.Context(), service.CreatePRInput{
		ID:     req.ID,
		Name:   req.Name,
		Author: req.Author,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"pr": pr})
}

func (s *Server) prMergeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"pull_request_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		writeDecodeError(w, errors.New("pull_request_id is required"))
		return
	}

	pr, err := s.svc.MergePullRequest(r.Context(), req.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pr": pr})
}

func (s *Server) prReassignHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PRID     string `json:"pull_request_id"`
		OldUser  string `json:"old_user_id"`
		AltField string `json:"old_reviewer_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if req.OldUser == "" && req.AltField != "" {
		req.OldUser = req.AltField // support example field name
	}
	req.PRID = strings.TrimSpace(req.PRID)
	req.OldUser = strings.TrimSpace(req.OldUser)
	if req.PRID == "" || req.OldUser == "" {
		writeDecodeError(w, errors.New("pull_request_id and old_user_id are required"))
		return
	}

	pr, replacedBy, err := s.svc.ReassignReviewer(r.Context(), req.PRID, req.OldUser)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pr": pr, "replaced_by": replacedBy})
}

func (s *Server) userReviewsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID := r.URL.Query().Get("user_id")
	userID = strings.TrimSpace(userID)
	if userID == "" {
		writeDecodeError(w, errors.New("user_id query parameter is required"))
		return
	}

	prs, err := s.svc.ListUserReviews(r.Context(), userID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":       userID,
		"pull_requests": prs,
	})
}

func (s *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	stats, err := s.svc.Stats(r.Context())
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func decodeJSON(r *http.Request, v any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

func sanitizeTeam(team models.Team) (models.Team, error) {
	team.TeamName = strings.TrimSpace(team.TeamName)
	if team.TeamName == "" {
		return models.Team{}, errors.New("team_name is required")
	}
	if len(team.Members) == 0 {
		return models.Team{}, errors.New("members must not be empty")
	}
	for i, m := range team.Members {
		m.UserID = strings.TrimSpace(m.UserID)
		m.Username = strings.TrimSpace(m.Username)
		if m.UserID == "" || m.Username == "" {
			return models.Team{}, errors.New("member user_id and username are required")
		}
		team.Members[i] = m
	}
	return team, nil
}

func writeDecodeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": map[string]any{
			"code":    "BAD_REQUEST",
			"message": err.Error(),
		},
	})
}

func writeAppError(w http.ResponseWriter, err error) {
	var appErr *service.AppError
	if errors.As(err, &appErr) {
		writeJSON(w, appErr.Status, map[string]any{
			"error": map[string]any{
				"code":    appErr.Code,
				"message": appErr.Message,
			},
		})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"error": map[string]any{
			"code":    "INTERNAL",
			"message": err.Error(),
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
