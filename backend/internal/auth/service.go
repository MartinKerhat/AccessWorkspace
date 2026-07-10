package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var ErrUnauthenticated = errors.New("unauthenticated")
var ErrBlocked = errors.New("user is blocked")
var ErrNotFound = errors.New("not found")
var ErrInvalidInput = errors.New("invalid input")
var ErrVaultLocked = errors.New("personal vault is locked")

type Bootstrap struct {
	AuthMode           Mode `json:"authMode"`
	LocalLoginEnabled  bool `json:"localLoginEnabled"`
	MicrosoftLoginHint bool `json:"microsoftLoginHint"`
}

type LoginResult struct {
	Token        string                `json:"token"`
	User         User                  `json:"user"`
	AuthMode     Mode                  `json:"authMode"`
	Capabilities WorkspaceCapabilities `json:"capabilities"`
}

type BrowserExtensionConnectToken struct {
	Token     string    `json:"token"`
	User      User      `json:"user"`
	AuthMode  Mode      `json:"authMode"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Authenticator interface {
	Bootstrap() Bootstrap
	CurrentUser(ctx context.Context, r *http.Request) (User, error)
	Login(ctx context.Context, username, password string) (LoginResult, error)
	IssueSession(ctx context.Context, user User, mode Mode) (LoginResult, error)
	IssueBrowserExtensionConnectToken(ctx context.Context, user User, mode Mode) (BrowserExtensionConnectToken, error)
	ExchangeBrowserExtensionConnectToken(ctx context.Context, token string, installationID string) (LoginResult, error)
	Logout(ctx context.Context, token string) error
	ChangeOwnPassword(ctx context.Context, user User, currentPassword, newPassword string) error
	AcceptInvite(ctx context.Context, token, password string) (LoginResult, error)
	GetVaultStatus(ctx context.Context, user User) (VaultStatus, error)
	SetupVault(ctx context.Context, user User, token, passphrase string) error
	UnlockVault(ctx context.Context, user User, token, secret string) error
	AddVaultPassphrase(ctx context.Context, user User, passphrase string) error
	ListLocalGroups(ctx context.Context) ([]LocalGroup, error)
	ListUsers(ctx context.Context) ([]UserSummary, error)
	CreateUser(ctx context.Context, input CreateUserInput) (UserAccessDetail, error)
	GetUserAccess(ctx context.Context, id string) (UserAccessDetail, error)
	UpdateUserAccess(ctx context.Context, id string, input UserAccessUpdateInput) (UserAccessDetail, error)
	CreateLocalGroup(ctx context.Context, input LocalGroupInput) error
	UpdateLocalGroup(ctx context.Context, name string, input LocalGroupInput) error
}

type Service struct {
	repo                    *Repository
	mode                    Mode
	sessionTTL              time.Duration
	browserExtensionTTL     time.Duration
	connectTokenTTL         time.Duration
	defaultDirectRightsJSON string
	inviteMailer            InviteMailer
	inviteBaseURL           string
}

func NewService(repo *Repository, mode Mode, defaultDirectRightsJSON string) *Service {
	return &Service{
		repo:                    repo,
		mode:                    mode,
		sessionTTL:              24 * time.Hour,
		browserExtensionTTL:     30 * 24 * time.Hour,
		connectTokenTTL:         5 * time.Minute,
		defaultDirectRightsJSON: defaultDirectRightsJSON,
	}
}

func (s *Service) Bootstrap() Bootstrap {
	return Bootstrap{
		AuthMode:           s.mode,
		LocalLoginEnabled:  true,
		MicrosoftLoginHint: true,
	}
}

func (s *Service) Login(ctx context.Context, username, password string) (LoginResult, error) {
	user, err := s.repo.Authenticate(ctx, username, password)
	if err != nil {
		return LoginResult{}, err
	}
	result, err := s.issueSession(ctx, user, s.mode, false)
	if err != nil {
		return LoginResult{}, err
	}
	// Local login carries the password: create the personal vault on first
	// login and unlock it for this session. The password itself is never
	// stored — it only derives the key that wraps the vault's private key.
	vaultKey, err := s.repo.EnsureLocalUserVault(ctx, result.User.ID, password)
	if err != nil {
		return LoginResult{}, err
	}
	if len(vaultKey) > 0 {
		if err := s.repo.attachVaultKeyToSession(ctx, "auth_sessions", result.Token, vaultKey); err != nil {
			return LoginResult{}, err
		}
		result.User.VaultPrivateKey = vaultKey
	}
	return result, nil
}

func (s *Service) IssueSession(ctx context.Context, user User, mode Mode) (LoginResult, error) {
	return s.issueSession(ctx, user, mode, true)
}

func (s *Service) IssueBrowserExtensionConnectToken(ctx context.Context, user User, mode Mode) (BrowserExtensionConnectToken, error) {
	resolvedUser, err := s.repo.ResolveAuthorization(ctx, user, s.defaultDirectRightsJSON)
	if err != nil {
		return BrowserExtensionConnectToken{}, err
	}
	blocked, err := s.repo.IsBlocked(ctx, resolvedUser.ID)
	if err != nil {
		return BrowserExtensionConnectToken{}, err
	}
	if blocked {
		return BrowserExtensionConnectToken{}, ErrBlocked
	}
	token, expiresAt, err := s.repo.CreateBrowserExtensionConnectToken(ctx, resolvedUser.ID, mode, s.connectTokenTTL, user.VaultPrivateKey)
	if err != nil {
		return BrowserExtensionConnectToken{}, err
	}
	return BrowserExtensionConnectToken{
		Token:     token,
		User:      resolvedUser,
		AuthMode:  mode,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) ExchangeBrowserExtensionConnectToken(ctx context.Context, token string, installationID string) (LoginResult, error) {
	token = strings.TrimSpace(token)
	installationID = strings.TrimSpace(installationID)
	if token == "" || installationID == "" {
		return LoginResult{}, ErrInvalidInput
	}
	userID, mode, vaultKey, err := s.repo.ExchangeBrowserExtensionConnectToken(ctx, token)
	if err != nil {
		return LoginResult{}, err
	}
	user, err := s.repo.userByID(ctx, userID)
	if err != nil {
		return LoginResult{}, err
	}
	resolvedUser, err := s.repo.ResolveAuthorization(ctx, user, s.defaultDirectRightsJSON)
	if err != nil {
		return LoginResult{}, err
	}
	blocked, err := s.repo.IsBlocked(ctx, resolvedUser.ID)
	if err != nil {
		return LoginResult{}, err
	}
	if blocked {
		return LoginResult{}, ErrBlocked
	}
	sessionToken, err := s.repo.UpsertBrowserExtensionSession(ctx, resolvedUser.ID, installationID, s.browserExtensionTTL, vaultKey)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		Token:        sessionToken,
		User:         resolvedUser,
		AuthMode:     mode,
		Capabilities: CapabilitiesForUser(resolvedUser),
	}, nil
}

func (s *Service) issueSession(ctx context.Context, user User, mode Mode, upsert bool) (LoginResult, error) {
	resolvedUser, err := s.repo.ResolveAuthorization(ctx, user, s.defaultDirectRightsJSON)
	if err != nil {
		return LoginResult{}, err
	}
	blocked, err := s.repo.IsBlocked(ctx, resolvedUser.ID)
	if err != nil {
		return LoginResult{}, err
	}
	if blocked {
		return LoginResult{}, ErrBlocked
	}
	if upsert {
		if err := s.repo.UpsertExternalUser(ctx, resolvedUser); err != nil {
			return LoginResult{}, err
		}
	}
	token, err := s.repo.CreateSession(ctx, resolvedUser.ID, s.sessionTTL)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		Token:        token,
		User:         resolvedUser,
		AuthMode:     mode,
		Capabilities: CapabilitiesForUser(resolvedUser),
	}, nil
}

func (s *Service) CurrentUser(ctx context.Context, r *http.Request) (User, error) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return User{}, ErrUnauthenticated
	}
	user, err := s.repo.UserByToken(ctx, token)
	if err != nil {
		return User{}, err
	}
	return s.repo.ResolveAuthorization(ctx, user, s.defaultDirectRightsJSON)
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.repo.DeleteSession(ctx, token)
}

func (s *Service) ListLocalGroups(ctx context.Context) ([]LocalGroup, error) {
	return s.repo.ListLocalGroups(ctx)
}

func (s *Service) ListUsers(ctx context.Context) ([]UserSummary, error) {
	users, err := s.repo.listStoredUsers(ctx)
	if err != nil {
		return nil, err
	}
	localGroups, err := s.repo.ListLocalGroups(ctx)
	if err != nil {
		return nil, err
	}
	directRights, err := s.repo.loadDirectRightsMap(ctx, s.defaultDirectRightsJSON)
	if err != nil {
		return nil, err
	}

	items := make([]UserSummary, 0, len(users))
	for _, user := range users {
		resolved := resolveAuthorizationFromInputs(user, localGroups, directRights)
		items = append(items, UserSummary{
			ID:                 resolved.ID,
			Name:               resolved.Name,
			Email:              resolved.Email,
			IsAdmin:            resolved.IsAdmin,
			Blocked:            user.Blocked,
			LocalGroups:        append([]string{}, resolved.LocalGroups...),
			ExternalGroupCount: len(user.Groups),
			RightsCount:        len(resolved.Rights),
		})
	}
	return items, nil
}

func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) (UserAccessDetail, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Password = strings.TrimSpace(input.Password)
	input.DirectLocalGroups = uniqueNormalized(input.DirectLocalGroups)

	switch {
	case input.Username == "":
		return UserAccessDetail{}, fmt.Errorf("%w: username is required", ErrInvalidInput)
	case input.DisplayName == "":
		return UserAccessDetail{}, fmt.Errorf("%w: display name is required", ErrInvalidInput)
	case input.Email == "":
		return UserAccessDetail{}, fmt.Errorf("%w: email is required", ErrInvalidInput)
	case !strings.Contains(input.Email, "@"):
		return UserAccessDetail{}, fmt.Errorf("%w: email address must be valid", ErrInvalidInput)
	case input.Invite && input.Password != "":
		return UserAccessDetail{}, fmt.Errorf("%w: invited users set their own password via the invite link", ErrInvalidInput)
	case !input.Invite && len(input.Password) < 8:
		return UserAccessDetail{}, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidInput)
	}

	id, err := s.repo.CreateUser(ctx, input)
	if err != nil {
		return UserAccessDetail{}, err
	}
	// With a direct password the vault exists from day one (known gap: the
	// admin who typed the password could unlock it — prefer invites, where
	// the vault is born from a password no admin has seen).
	if !input.Invite {
		if _, err := s.repo.EnsureLocalUserVault(ctx, id, input.Password); err != nil {
			return UserAccessDetail{}, err
		}
	}
	return s.GetUserAccess(ctx, id)
}

// DeleteUser removes the target user. Their shared resources are transferred to
// actor (the admin performing the deletion) so nothing is left owned by a deleted
// account; their personal resources are deleted by the repository cascade.
func (s *Service) DeleteUser(ctx context.Context, actor User, id string) (DeleteUserResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return DeleteUserResult{}, ErrNotFound
	}
	if strings.TrimSpace(actor.Name) == "" {
		return DeleteUserResult{}, fmt.Errorf("%w: acting administrator is required", ErrInvalidInput)
	}

	target, err := s.repo.userByID(ctx, id)
	if err != nil {
		return DeleteUserResult{}, err
	}

	// Never delete the last remaining admin, or the workspace becomes unmanageable.
	if target.IsAdmin {
		admins, err := s.repo.countAdmins(ctx)
		if err != nil {
			return DeleteUserResult{}, err
		}
		if admins <= 1 {
			return DeleteUserResult{}, fmt.Errorf("%w: cannot delete the last administrator", ErrInvalidInput)
		}
	}

	return s.repo.DeleteUser(ctx, id, target.Name, actor.ID, actor.Name)
}

func (s *Service) GetUserAccess(ctx context.Context, id string) (UserAccessDetail, error) {
	user, err := s.repo.userByID(ctx, id)
	if err != nil {
		return UserAccessDetail{}, err
	}
	localGroups, err := s.repo.ListLocalGroups(ctx)
	if err != nil {
		return UserAccessDetail{}, err
	}
	directRights, err := s.repo.loadDirectRightsMap(ctx, s.defaultDirectRightsJSON)
	if err != nil {
		return UserAccessDetail{}, err
	}

	resolved := resolveAuthorizationFromInputs(user, localGroups, directRights)
	matches := resolveLocalGroupMatches(user, localGroups)
	resolvedGroups := make([]ResolvedLocalGroup, 0, len(matches))
	for _, match := range matches {
		resolvedGroups = append(resolvedGroups, ResolvedLocalGroup{
			Name:                 match.Group.Name,
			AssignmentSource:     match.AssignmentSource,
			MatchedExternalGroup: match.MatchedExternalGroup,
			Rights:               append([]string{}, match.Group.Rights...),
		})
	}

	return UserAccessDetail{
		ID:                        resolved.ID,
		Name:                      resolved.Name,
		Email:                     resolved.Email,
		IsAdmin:                   resolved.IsAdmin,
		Blocked:                   user.Blocked,
		ExternalGroups:            append([]string{}, user.Groups...),
		ResolvedLocalGroups:       resolvedGroups,
		DirectAssignedLocalGroups: directAssignedLocalGroupNames(user, localGroups),
		DirectRights:              append([]string{}, user.DirectRights...),
		Rights:                    append([]string{}, resolved.Rights...),
		Capabilities:              CapabilitiesForUser(resolved),
	}, nil
}

func (s *Service) UpdateUserAccess(ctx context.Context, id string, input UserAccessUpdateInput) (UserAccessDetail, error) {
	input.DirectLocalGroups = uniqueNormalized(input.DirectLocalGroups)
	input.DirectRights = uniqueNormalized(input.DirectRights)
	if err := s.repo.UpdateUserAccess(ctx, id, input); err != nil {
		return UserAccessDetail{}, err
	}
	return s.GetUserAccess(ctx, id)
}

func (s *Service) CreateLocalGroup(ctx context.Context, input LocalGroupInput) error {
	return s.repo.CreateLocalGroup(ctx, input)
}

func (s *Service) UpdateLocalGroup(ctx context.Context, name string, input LocalGroupInput) error {
	return s.repo.UpdateLocalGroup(ctx, name, input)
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}
