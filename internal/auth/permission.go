package auth

import (
	"errors"
	"strings"
)

// Common errors for permission operations.
var (
	ErrPermissionDenied = errors.New("permission denied")
	ErrInvalidRole      = errors.New("invalid role")
)

// Role represents a user role in the system.
type Role string

// Predefined roles.
const (
	RoleAdmin      Role = "admin"
	RoleModerator  Role = "moderator"
	RolePublisher  Role = "publisher"
	RoleSubscriber Role = "subscriber"
)

// ParseRole parses a string into a Role.
// Returns ErrInvalidRole if the role is not recognized.
func ParseRole(s string) (Role, error) {
	role := Role(strings.ToLower(s))
	switch role {
	case RoleAdmin, RoleModerator, RolePublisher, RoleSubscriber:
		return role, nil
	default:
		return "", ErrInvalidRole
	}
}

// IsValid checks if the role is a valid predefined role.
func (r Role) IsValid() bool {
	switch r {
	case RoleAdmin, RoleModerator, RolePublisher, RoleSubscriber:
		return true
	default:
		return false
	}
}

// String returns the string representation of the role.
func (r Role) String() string {
	return string(r)
}

// Permission represents a specific permission in the system.
type Permission string

// Predefined permissions.
const (
	// Room management permissions
	PermRoomCreate Permission = "room:create"
	PermRoomDelete Permission = "room:delete"
	PermRoomLock   Permission = "room:lock"
	PermRoomUnlock Permission = "room:unlock"
	PermRoomJoin   Permission = "room:join"
	PermRoomLeave  Permission = "room:leave"

	// Media permissions
	PermMediaPublish     Permission = "media:publish"
	PermMediaUnpublish   Permission = "media:unpublish"
	PermMediaSubscribe   Permission = "media:subscribe"
	PermMediaUnsubscribe Permission = "media:unsubscribe"

	// Participant management permissions
	PermParticipantKick    Permission = "participant:kick"
	PermParticipantMute    Permission = "participant:mute"
	PermParticipantUnmute  Permission = "participant:unmute"
	PermParticipantPromote Permission = "participant:promote"
	PermParticipantDemote  Permission = "participant:demote"

	// Recording permissions
	PermRecordingStart Permission = "recording:start"
	PermRecordingStop  Permission = "recording:stop"

	// Metadata permissions
	PermMetadataUpdate Permission = "metadata:update"
)

// rolePermissions defines the default permissions for each role.
var rolePermissions = map[Role][]Permission{
	RoleAdmin: {
		// Room management
		PermRoomCreate, PermRoomDelete, PermRoomLock, PermRoomUnlock,
		PermRoomJoin, PermRoomLeave,
		// Media
		PermMediaPublish, PermMediaUnpublish, PermMediaSubscribe, PermMediaUnsubscribe,
		// Participant management
		PermParticipantKick, PermParticipantMute, PermParticipantUnmute,
		PermParticipantPromote, PermParticipantDemote,
		// Recording
		PermRecordingStart, PermRecordingStop,
		// Metadata
		PermMetadataUpdate,
	},
	RoleModerator: {
		// Room management (limited)
		PermRoomLock, PermRoomUnlock, PermRoomJoin, PermRoomLeave,
		// Media
		PermMediaPublish, PermMediaUnpublish, PermMediaSubscribe, PermMediaUnsubscribe,
		// Participant management
		PermParticipantKick, PermParticipantMute, PermParticipantUnmute,
		// Recording
		PermRecordingStart, PermRecordingStop,
		// Metadata
		PermMetadataUpdate,
	},
	RolePublisher: {
		// Room access
		PermRoomJoin, PermRoomLeave,
		// Media (can publish and subscribe)
		PermMediaPublish, PermMediaUnpublish, PermMediaSubscribe, PermMediaUnsubscribe,
		// Metadata (own)
		PermMetadataUpdate,
	},
	RoleSubscriber: {
		// Room access
		PermRoomJoin, PermRoomLeave,
		// Media (can only subscribe)
		PermMediaSubscribe, PermMediaUnsubscribe,
	},
}

// PermissionChecker checks if a user has the required permissions.
type PermissionChecker struct {
	// customPermissions allows overriding role-based permissions
	customPermissions map[Role][]Permission
}

// NewPermissionChecker creates a new permission checker with default role permissions.
func NewPermissionChecker() *PermissionChecker {
	return &PermissionChecker{
		customPermissions: make(map[Role][]Permission),
	}
}

// SetRolePermissions sets custom permissions for a role.
// This overrides the default permissions for the role.
func (pc *PermissionChecker) SetRolePermissions(role Role, permissions []Permission) {
	pc.customPermissions[role] = permissions
}

// GetRolePermissions returns the permissions for a role.
func (pc *PermissionChecker) GetRolePermissions(role Role) []Permission {
	// Check custom permissions first
	if perms, ok := pc.customPermissions[role]; ok {
		return perms
	}
	// Fall back to default permissions
	if perms, ok := rolePermissions[role]; ok {
		return perms
	}
	return nil
}

// HasPermission checks if a role has a specific permission.
func (pc *PermissionChecker) HasPermission(role Role, permission Permission) bool {
	permissions := pc.GetRolePermissions(role)
	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if a role has any of the specified permissions.
func (pc *PermissionChecker) HasAnyPermission(role Role, permissions ...Permission) bool {
	for _, perm := range permissions {
		if pc.HasPermission(role, perm) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if a role has all of the specified permissions.
func (pc *PermissionChecker) HasAllPermissions(role Role, permissions ...Permission) bool {
	for _, perm := range permissions {
		if !pc.HasPermission(role, perm) {
			return false
		}
	}
	return true
}

// CheckPermission returns an error if the role doesn't have the required permission.
func (pc *PermissionChecker) CheckPermission(role Role, permission Permission) error {
	if !pc.HasPermission(role, permission) {
		return ErrPermissionDenied
	}
	return nil
}

// CheckClaimsPermission checks if the claims have the required permission.
// It first checks explicit permissions in claims (if non-empty), then falls back to role-based permissions.
// An empty permissions slice will fall back to role-based permissions, not deny access.
func (pc *PermissionChecker) CheckClaimsPermission(claims *Claims, permission Permission) error {
	// Check explicit permissions in claims (only if non-empty slice)
	if len(claims.Permissions) > 0 {
		for _, p := range claims.Permissions {
			if Permission(p) == permission {
				return nil
			}
		}
		// If explicit permissions exist but don't include this permission,
		// still fall through to check role-based permissions
	}

	// Check role-based permissions
	if claims.Role != "" {
		role, err := ParseRole(claims.Role)
		if err != nil {
			return ErrPermissionDenied
		}
		if pc.HasPermission(role, permission) {
			return nil
		}
	}

	return ErrPermissionDenied
}

// ClaimsHasPermission checks if the claims have the required permission.
// Returns true if the permission is granted, false otherwise.
func (pc *PermissionChecker) ClaimsHasPermission(claims *Claims, permission Permission) bool {
	return pc.CheckClaimsPermission(claims, permission) == nil
}

// RoleHierarchy returns true if role1 is equal to or higher than role2 in the hierarchy.
// Hierarchy: admin > moderator > publisher > subscriber
func RoleHierarchy(role1, role2 Role) bool {
	hierarchy := map[Role]int{
		RoleAdmin:      4,
		RoleModerator:  3,
		RolePublisher:  2,
		RoleSubscriber: 1,
	}

	level1, ok1 := hierarchy[role1]
	level2, ok2 := hierarchy[role2]

	if !ok1 || !ok2 {
		return false
	}

	return level1 >= level2
}

// CanManageRole returns true if the manager role can manage (promote/demote/kick) the target role.
// Admin can manage all roles except other admins.
// Moderator can manage publishers and subscribers.
// Publishers and subscribers cannot manage others.
func CanManageRole(managerRole, targetRole Role) bool {
	switch managerRole {
	case RoleAdmin:
		// Admin can manage anyone except other admins
		return targetRole != RoleAdmin
	case RoleModerator:
		// Moderator can manage publishers and subscribers
		return targetRole == RolePublisher || targetRole == RoleSubscriber
	default:
		return false
	}
}
