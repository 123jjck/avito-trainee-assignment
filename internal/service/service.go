package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/123jjck/avito-trainee-assignment/internal/models"
)

const (
	CodeTeamExists  = "TEAM_EXISTS"
	CodePRExists    = "PR_EXISTS"
	CodePRMerged    = "PR_MERGED"
	CodeNotAssigned = "NOT_ASSIGNED"
	CodeNoCandidate = "NO_CANDIDATE"
	CodeNotFound    = "NOT_FOUND"
)

type Stats struct {
	TotalPRs    int              `json:"total_prs"`
	OpenPRs     int              `json:"open_prs"`
	MergedPRs   int              `json:"merged_prs"`
	Assignments []AssignmentStat `json:"assignments"`
}

type AssignmentStat struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Count    int    `json:"count"`
}

type AppError struct {
	Code    string
	Message string
	Status  int
}

func (e *AppError) Error() string {
	return e.Message
}

func newAppError(status int, code, msg string) *AppError {
	return &AppError{Status: status, Code: code, Message: msg}
}

type Service struct {
	db  *sql.DB
	rnd *rand.Rand
}

func New(db *sql.DB) *Service {
	return &Service{
		db:  db,
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Service) CreateTeam(ctx context.Context, team models.Team) (models.Team, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Team{}, err
	}
	defer tx.Rollback()

	var exists string
	err = tx.QueryRowContext(ctx, "SELECT team_name FROM teams WHERE team_name = $1", team.TeamName).Scan(&exists)
	if err == nil {
		return models.Team{}, newAppError(400, CodeTeamExists, "team_name already exists")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return models.Team{}, err
	}

	if _, err := tx.ExecContext(ctx, "INSERT INTO teams(team_name) VALUES ($1)", team.TeamName); err != nil {
		return models.Team{}, fmt.Errorf("insert team: %w", err)
	}

	for _, member := range team.Members {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO users (user_id, username, team_name, is_active)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (user_id)
			 DO UPDATE SET username = EXCLUDED.username,
			               team_name = EXCLUDED.team_name,
			               is_active = EXCLUDED.is_active`,
			member.UserID, member.Username, team.TeamName, member.IsActive,
		)
		if err != nil {
			return models.Team{}, fmt.Errorf("upsert user %s: %w", member.UserID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return models.Team{}, err
	}
	return team, nil
}

func (s *Service) GetTeam(ctx context.Context, teamName string) (models.Team, error) {
	var team models.Team
	err := s.db.QueryRowContext(ctx, "SELECT team_name FROM teams WHERE team_name = $1", teamName).Scan(&team.TeamName)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Team{}, newAppError(404, CodeNotFound, "team not found")
	}
	if err != nil {
		return models.Team{}, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT user_id, username, is_active FROM users WHERE team_name = $1 ORDER BY user_id`, teamName)
	if err != nil {
		return models.Team{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var m models.TeamMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.IsActive); err != nil {
			return models.Team{}, err
		}
		team.Members = append(team.Members, m)
	}
	if rows.Err() != nil {
		return models.Team{}, rows.Err()
	}
	return team, nil
}

func (s *Service) SetUserActive(ctx context.Context, userID string, isActive bool) (models.User, error) {
	var u models.User
	err := s.db.QueryRowContext(
		ctx,
		`UPDATE users SET is_active = $2 WHERE user_id = $1
		 RETURNING user_id, username, team_name, is_active`,
		userID, isActive,
	).Scan(&u.UserID, &u.Username, &u.TeamName, &u.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, newAppError(404, CodeNotFound, "user not found")
	}
	if err != nil {
		return models.User{}, err
	}
	return u, nil
}

type CreatePRInput struct {
	ID     string
	Name   string
	Author string
}

func (s *Service) CreatePullRequest(ctx context.Context, input CreatePRInput) (models.PullRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.PullRequest{}, err
	}
	defer tx.Rollback()

	var exists string
	if err := tx.QueryRowContext(ctx, "SELECT pull_request_id FROM pull_requests WHERE pull_request_id = $1", input.ID).Scan(&exists); err == nil {
		return models.PullRequest{}, newAppError(409, CodePRExists, "PR id already exists")
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return models.PullRequest{}, err
	}

	var author models.User
	err = tx.QueryRowContext(ctx,
		`SELECT user_id, username, team_name, is_active FROM users WHERE user_id = $1`,
		input.Author,
	).Scan(&author.UserID, &author.Username, &author.TeamName, &author.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return models.PullRequest{}, newAppError(404, CodeNotFound, "author not found")
	}
	if err != nil {
		return models.PullRequest{}, err
	}

	var createdAt time.Time
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO pull_requests (pull_request_id, pull_request_name, author_id, status)
		 VALUES ($1, $2, $3, $4)
		 RETURNING created_at`,
		input.ID, input.Name, input.Author, models.StatusOpen,
	).Scan(&createdAt); err != nil {
		return models.PullRequest{}, fmt.Errorf("insert pr: %w", err)
	}

	candidates, err := s.activeTeamMembers(ctx, tx, author.TeamName, input.Author)
	if err != nil {
		return models.PullRequest{}, err
	}
	assignments := pickRandom(s.rnd, candidates, 2)
	for _, reviewer := range assignments {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pr_reviewers (pull_request_id, user_id) VALUES ($1, $2)`,
			input.ID, reviewer,
		); err != nil {
			return models.PullRequest{}, fmt.Errorf("assign reviewer %s: %w", reviewer, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return models.PullRequest{}, err
	}

	return models.PullRequest{
		ID:                input.ID,
		Name:              input.Name,
		AuthorID:          input.Author,
		Status:            models.StatusOpen,
		AssignedReviewers: assignments,
		CreatedAt:         &createdAt,
	}, nil
}

func (s *Service) MergePullRequest(ctx context.Context, prID string) (models.PullRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.PullRequest{}, err
	}
	defer tx.Rollback()

	var pr models.PullRequest
	var createdAt time.Time
	var mergedAt sql.NullTime
	err = tx.QueryRowContext(ctx,
		`SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
		 FROM pull_requests WHERE pull_request_id = $1 FOR UPDATE`,
		prID,
	).Scan(&pr.ID, &pr.Name, &pr.AuthorID, &pr.Status, &createdAt, &mergedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.PullRequest{}, newAppError(404, CodeNotFound, "pull request not found")
	}
	if err != nil {
		return models.PullRequest{}, err
	}
	pr.CreatedAt = &createdAt
	if mergedAt.Valid {
		pr.MergedAt = &mergedAt.Time
	}

	if pr.Status != models.StatusMerged {
		var updated time.Time
		err = tx.QueryRowContext(ctx,
			`UPDATE pull_requests SET status = $2, merged_at = COALESCE(merged_at, now())
			 WHERE pull_request_id = $1
			 RETURNING merged_at`,
			prID, models.StatusMerged,
		).Scan(&updated)
		if err != nil {
			return models.PullRequest{}, err
		}
		pr.Status = models.StatusMerged
		pr.MergedAt = &updated
	}

	pr.AssignedReviewers, err = s.loadReviewers(ctx, tx, prID)
	if err != nil {
		return models.PullRequest{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.PullRequest{}, err
	}
	return pr, nil
}

func (s *Service) ReassignReviewer(ctx context.Context, prID, oldUserID string) (models.PullRequest, string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.PullRequest{}, "", err
	}
	defer tx.Rollback()

	var pr models.PullRequest
	var createdAt time.Time
	var mergedAt sql.NullTime
	err = tx.QueryRowContext(ctx,
		`SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
		 FROM pull_requests WHERE pull_request_id = $1 FOR UPDATE`,
		prID,
	).Scan(&pr.ID, &pr.Name, &pr.AuthorID, &pr.Status, &createdAt, &mergedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.PullRequest{}, "", newAppError(404, CodeNotFound, "pull request not found")
	}
	if err != nil {
		return models.PullRequest{}, "", err
	}
	pr.CreatedAt = &createdAt
	if mergedAt.Valid {
		pr.MergedAt = &mergedAt.Time
	}

	if pr.Status == models.StatusMerged {
		return models.PullRequest{}, "", newAppError(409, CodePRMerged, "cannot reassign on merged PR")
	}

	assigned, err := s.loadReviewers(ctx, tx, prID)
	if err != nil {
		return models.PullRequest{}, "", err
	}
	if !contains(assigned, oldUserID) {
		return models.PullRequest{}, "", newAppError(409, CodeNotAssigned, "reviewer is not assigned to this PR")
	}

	var user models.User
	err = tx.QueryRowContext(ctx,
		`SELECT user_id, username, team_name, is_active FROM users WHERE user_id = $1`,
		oldUserID,
	).Scan(&user.UserID, &user.Username, &user.TeamName, &user.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return models.PullRequest{}, "", newAppError(404, CodeNotFound, "user not found")
	}
	if err != nil {
		return models.PullRequest{}, "", err
	}

	candidates, err := s.activeTeamMembers(ctx, tx, user.TeamName, oldUserID)
	if err != nil {
		return models.PullRequest{}, "", err
	}
	assignedSet := make(map[string]struct{}, len(assigned))
	for _, id := range assigned {
		assignedSet[id] = struct{}{}
	}
	filtered := make([]string, 0, len(candidates))
	for _, id := range candidates {
		if _, already := assignedSet[id]; already {
			continue
		}
		if id == pr.AuthorID {
			continue // avoid self-review on reassignment as well
		}
		filtered = append(filtered, id)
	}

	if len(filtered) == 0 {
		return models.PullRequest{}, "", newAppError(409, CodeNoCandidate, "no active replacement candidate in team")
	}
	newReviewer := filtered[s.rnd.Intn(len(filtered))]

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM pr_reviewers WHERE pull_request_id = $1 AND user_id = $2`,
		prID, oldUserID,
	); err != nil {
		return models.PullRequest{}, "", err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO pr_reviewers (pull_request_id, user_id) VALUES ($1, $2)`,
		prID, newReviewer,
	); err != nil {
		return models.PullRequest{}, "", err
	}

	pr.AssignedReviewers, err = s.loadReviewers(ctx, tx, prID)
	if err != nil {
		return models.PullRequest{}, "", err
	}

	if err := tx.Commit(); err != nil {
		return models.PullRequest{}, "", err
	}

	return pr, newReviewer, nil
}

func (s *Service) ListUserReviews(ctx context.Context, userID string) ([]models.PullRequestShort, error) {
	var exists string
	err := s.db.QueryRowContext(ctx, "SELECT user_id FROM users WHERE user_id = $1", userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, newAppError(404, CodeNotFound, "user not found")
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT pr.pull_request_id, pr.pull_request_name, pr.author_id, pr.status
		 FROM pull_requests pr
		 JOIN pr_reviewers r ON pr.pull_request_id = r.pull_request_id
		 WHERE r.user_id = $1
		 ORDER BY pr.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.PullRequestShort
	for rows.Next() {
		var pr models.PullRequestShort
		if err := rows.Scan(&pr.ID, &pr.Name, &pr.AuthorID, &pr.Status); err != nil {
			return nil, err
		}
		result = append(result, pr)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return result, nil
}

func (s *Service) activeTeamMembers(ctx context.Context, tx *sql.Tx, teamName, excludedID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT user_id FROM users WHERE team_name = $1 AND is_active = true AND user_id <> $2`,
		teamName, excludedID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return ids, nil
}

func (s *Service) loadReviewers(ctx context.Context, tx *sql.Tx, prID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT user_id FROM pr_reviewers WHERE pull_request_id = $1 ORDER BY user_id`,
		prID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviewers []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		reviewers = append(reviewers, id)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return reviewers, nil
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN status = 'OPEN' THEN 1 ELSE 0 END), 0) AS open,
			COALESCE(SUM(CASE WHEN status = 'MERGED' THEN 1 ELSE 0 END), 0) AS merged
		 FROM pull_requests`,
	).Scan(&st.TotalPRs, &st.OpenPRs, &st.MergedPRs)
	if err != nil {
		return Stats{}, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT u.user_id, u.username, COUNT(r.pull_request_id) AS cnt
		 FROM users u
		 LEFT JOIN pr_reviewers r ON u.user_id = r.user_id
		 GROUP BY u.user_id, u.username
		 ORDER BY cnt DESC, u.user_id`,
	)
	if err != nil {
		return Stats{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var a AssignmentStat
		if err := rows.Scan(&a.UserID, &a.Username, &a.Count); err != nil {
			return Stats{}, err
		}
		st.Assignments = append(st.Assignments, a)
	}
	if rows.Err() != nil {
		return Stats{}, rows.Err()
	}
	return st, nil
}

func pickRandom(rnd *rand.Rand, ids []string, limit int) []string {
	if len(ids) == 0 || limit <= 0 {
		return nil
	}
	rnd.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	if len(ids) > limit {
		return append([]string{}, ids[:limit]...)
	}
	return append([]string{}, ids...)
}

func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
