package auth

import "slices"

type Mode string

const (
	ModeDev   Mode = "dev"
	ModeEntra Mode = "entra"
)

type User struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Groups      []string `json:"groups"`
	LocalGroups []string `json:"localGroups"`
	Rights      []string `json:"rights"`
	DirectRights []string `json:"directRights,omitempty"`
	IsAdmin     bool     `json:"isAdmin"`
	Blocked     bool     `json:"blocked,omitempty"`
}

type LocalGroup struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Rights               []string `json:"rights"`
	MappedExternalGroups []string `json:"mappedExternalGroups"`
	AssignedUserIDs      []string `json:"assignedUserIds"`
}

type LocalGroupInput struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Rights               []string `json:"rights"`
	MappedExternalGroups []string `json:"mappedExternalGroups"`
	AssignedUserIDs      []string `json:"assignedUserIds"`
}

type UserSummary struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Email              string   `json:"email"`
	IsAdmin            bool     `json:"isAdmin"`
	Blocked            bool     `json:"blocked"`
	LocalGroups        []string `json:"localGroups"`
	ExternalGroupCount int      `json:"externalGroupCount"`
	RightsCount        int      `json:"rightsCount"`
}

type ResolvedLocalGroup struct {
	Name                 string   `json:"name"`
	AssignmentSource     string   `json:"assignmentSource"`
	MatchedExternalGroup string   `json:"matchedExternalGroup,omitempty"`
	Rights               []string `json:"rights"`
}

type UserAccessDetail struct {
	ID                      string                `json:"id"`
	Name                    string                `json:"name"`
	Email                   string                `json:"email"`
	IsAdmin                 bool                  `json:"isAdmin"`
	Blocked                 bool                  `json:"blocked"`
	ExternalGroups          []string              `json:"externalGroups"`
	ResolvedLocalGroups     []ResolvedLocalGroup  `json:"resolvedLocalGroups"`
	DirectAssignedLocalGroups []string            `json:"directAssignedLocalGroups"`
	DirectRights            []string              `json:"directRights"`
	Rights                  []string              `json:"rights"`
	Capabilities            WorkspaceCapabilities `json:"capabilities"`
}

type UserAccessUpdateInput struct {
	Blocked           bool     `json:"blocked"`
	DirectLocalGroups []string `json:"directLocalGroups"`
	DirectRights      []string `json:"directRights"`
}

type DeleteUserResult struct {
	PersonalResourcesDeleted  int `json:"personalResourcesDeleted"`
	SharedResourcesReassigned int `json:"sharedResourcesReassigned"`
}

type CreateUserInput struct {
	Username          string   `json:"username"`
	DisplayName       string   `json:"displayName"`
	Email             string   `json:"email"`
	Password          string   `json:"password"`
	IsAdmin           bool     `json:"isAdmin"`
	Blocked           bool     `json:"blocked"`
	DirectLocalGroups []string `json:"directLocalGroups"`
}

type CategoryCapabilities struct {
	View   bool `json:"view"`
	Create bool `json:"create"`
	Import bool `json:"import"`
	Edit   bool `json:"edit"`
	Reveal bool `json:"reveal"`
	Launch bool `json:"launch"`
}

type WorkspaceCapabilities struct {
	Categories      map[string]CategoryCapabilities `json:"categories"`
	CanViewActivity bool                            `json:"canViewActivity"`
	CanViewAudit    bool                            `json:"canViewAudit"`
	CanViewAdmin    bool                            `json:"canViewAdmin"`
}

func CapabilitiesForUser(user User) WorkspaceCapabilities {
	if user.IsAdmin {
		return WorkspaceCapabilities{
			Categories: map[string]CategoryCapabilities{
				"connections":      {View: true, Create: true, Edit: true, Launch: true},
				"keyvault":         {View: true, Import: true, Edit: true, Reveal: true},
				"appregistrations": {View: true, Import: true, Edit: true},
				"passwords":        {View: true, Create: true, Edit: true, Reveal: true, Launch: true},
			},
			CanViewActivity: true,
			CanViewAudit:    true,
			CanViewAdmin:    true,
		}
	}

	has := func(right string) bool {
		return slices.Contains(user.Rights, right)
	}

	categories := map[string]CategoryCapabilities{
		"connections": {
			View:   has("connections.read") || has("connections.edit"),
			Create: has("connections.edit"),
			Edit:   has("connections.edit"),
			Launch: has("connections.read") || has("connections.edit"),
		},
		"keyvault": {
			View:   has("keyvault.read") || has("keyvault.edit"),
			Import: has("keyvault.edit"),
			Edit:   has("keyvault.edit"),
			Reveal: has("keyvault.read") || has("keyvault.edit"),
		},
		"appregistrations": {
			View:   has("appregistrations.read") || has("appregistrations.edit"),
			Import: has("appregistrations.edit"),
			Edit:   has("appregistrations.edit"),
		},
		"passwords": {
			View:   has("passwords.read") || has("passwords.edit"),
			Create: has("passwords.edit"),
			Edit:   has("passwords.edit"),
			Reveal: has("passwords.read") || has("passwords.edit"),
			Launch: has("passwords.read") || has("passwords.edit"),
		},
	}

	return WorkspaceCapabilities{
		Categories:      categories,
		CanViewActivity: true,
		CanViewAudit:    has("audit.read"),
		CanViewAdmin:    has("admin.access"),
	}
}
