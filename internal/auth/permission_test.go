package auth

import (
	"errors"
	"testing"
)

func TestParseRole(t *testing.T) {
	tests := []struct {
		input    string
		expected Role
		hasError bool
	}{
		{"admin", RoleAdmin, false},
		{"ADMIN", RoleAdmin, false},
		{"Admin", RoleAdmin, false},
		{"moderator", RoleModerator, false},
		{"MODERATOR", RoleModerator, false},
		{"publisher", RolePublisher, false},
		{"subscriber", RoleSubscriber, false},
		{"invalid", "", true},
		{"", "", true},
		{"guest", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			role, err := ParseRole(tt.input)
			if tt.hasError {
				if err == nil {
					t.Error("expected error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if role != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, role)
				}
			}
		})
	}
}

func TestRole_IsValid(t *testing.T) {
	tests := []struct {
		role  Role
		valid bool
	}{
		{RoleAdmin, true},
		{RoleModerator, true},
		{RolePublisher, true},
		{RoleSubscriber, true},
		{Role("invalid"), false},
		{Role(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if tt.role.IsValid() != tt.valid {
				t.Errorf("expected IsValid() = %v for role %s", tt.valid, tt.role)
			}
		})
	}
}

func TestRole_String(t *testing.T) {
	if RoleAdmin.String() != "admin" {
		t.Errorf("expected 'admin', got %s", RoleAdmin.String())
	}
}

func TestPermissionChecker_HasPermission(t *testing.T) {
	pc := NewPermissionChecker()

	t.Run("admin has all permissions", func(t *testing.T) {
		if !pc.HasPermission(RoleAdmin, PermRoomCreate) {
			t.Error("admin should have room:create permission")
		}
		if !pc.HasPermission(RoleAdmin, PermParticipantKick) {
			t.Error("admin should have participant:kick permission")
		}
		if !pc.HasPermission(RoleAdmin, PermRecordingStart) {
			t.Error("admin should have recording:start permission")
		}
	})

	t.Run("moderator has limited permissions", func(t *testing.T) {
		if !pc.HasPermission(RoleModerator, PermRoomLock) {
			t.Error("moderator should have room:lock permission")
		}
		if !pc.HasPermission(RoleModerator, PermParticipantKick) {
			t.Error("moderator should have participant:kick permission")
		}
		if pc.HasPermission(RoleModerator, PermRoomCreate) {
			t.Error("moderator should NOT have room:create permission")
		}
		if pc.HasPermission(RoleModerator, PermRoomDelete) {
			t.Error("moderator should NOT have room:delete permission")
		}
	})

	t.Run("publisher can publish and subscribe", func(t *testing.T) {
		if !pc.HasPermission(RolePublisher, PermMediaPublish) {
			t.Error("publisher should have media:publish permission")
		}
		if !pc.HasPermission(RolePublisher, PermMediaSubscribe) {
			t.Error("publisher should have media:subscribe permission")
		}
		if pc.HasPermission(RolePublisher, PermParticipantKick) {
			t.Error("publisher should NOT have participant:kick permission")
		}
	})

	t.Run("subscriber can only subscribe", func(t *testing.T) {
		if !pc.HasPermission(RoleSubscriber, PermMediaSubscribe) {
			t.Error("subscriber should have media:subscribe permission")
		}
		if pc.HasPermission(RoleSubscriber, PermMediaPublish) {
			t.Error("subscriber should NOT have media:publish permission")
		}
		if pc.HasPermission(RoleSubscriber, PermParticipantKick) {
			t.Error("subscriber should NOT have participant:kick permission")
		}
	})

	t.Run("invalid role has no permissions", func(t *testing.T) {
		if pc.HasPermission(Role("invalid"), PermRoomJoin) {
			t.Error("invalid role should have no permissions")
		}
	})
}

func TestPermissionChecker_HasAnyPermission(t *testing.T) {
	pc := NewPermissionChecker()

	t.Run("has any when one matches", func(t *testing.T) {
		if !pc.HasAnyPermission(RoleSubscriber, PermMediaPublish, PermMediaSubscribe) {
			t.Error("subscriber should have at least one of these permissions")
		}
	})

	t.Run("no match returns false", func(t *testing.T) {
		if pc.HasAnyPermission(RoleSubscriber, PermMediaPublish, PermParticipantKick) {
			t.Error("subscriber should NOT have any of these permissions")
		}
	})
}

func TestPermissionChecker_HasAllPermissions(t *testing.T) {
	pc := NewPermissionChecker()

	t.Run("has all when all match", func(t *testing.T) {
		if !pc.HasAllPermissions(RolePublisher, PermRoomJoin, PermMediaPublish, PermMediaSubscribe) {
			t.Error("publisher should have all of these permissions")
		}
	})

	t.Run("missing one returns false", func(t *testing.T) {
		if pc.HasAllPermissions(RolePublisher, PermRoomJoin, PermParticipantKick) {
			t.Error("publisher should NOT have all of these permissions")
		}
	})
}

func TestPermissionChecker_CheckPermission(t *testing.T) {
	pc := NewPermissionChecker()

	t.Run("returns nil on success", func(t *testing.T) {
		err := pc.CheckPermission(RoleAdmin, PermRoomCreate)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("returns error on failure", func(t *testing.T) {
		err := pc.CheckPermission(RoleSubscriber, PermMediaPublish)
		if err == nil {
			t.Error("expected permission denied error")
		}
		if !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})
}

func TestPermissionChecker_CustomPermissions(t *testing.T) {
	pc := NewPermissionChecker()

	// Set custom permissions for subscriber (allow publishing)
	pc.SetRolePermissions(RoleSubscriber, []Permission{
		PermRoomJoin, PermRoomLeave,
		PermMediaPublish, PermMediaSubscribe,
	})

	t.Run("custom permissions override defaults", func(t *testing.T) {
		if !pc.HasPermission(RoleSubscriber, PermMediaPublish) {
			t.Error("subscriber with custom permissions should have media:publish")
		}
	})

	t.Run("other roles unaffected", func(t *testing.T) {
		if !pc.HasPermission(RoleAdmin, PermRoomCreate) {
			t.Error("admin should still have default permissions")
		}
	})
}

func TestPermissionChecker_CheckClaimsPermission(t *testing.T) {
	pc := NewPermissionChecker()

	t.Run("explicit permission in claims", func(t *testing.T) {
		claims := &Claims{
			Role:        "subscriber",
			Permissions: []string{"media:publish"}, // Explicit override
		}

		err := pc.CheckClaimsPermission(claims, PermMediaPublish)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("role-based permission", func(t *testing.T) {
		claims := &Claims{
			Role: "publisher",
		}

		err := pc.CheckClaimsPermission(claims, PermMediaPublish)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("permission denied", func(t *testing.T) {
		claims := &Claims{
			Role: "subscriber",
		}

		err := pc.CheckClaimsPermission(claims, PermMediaPublish)
		if err == nil {
			t.Error("expected permission denied")
		}
	})

	t.Run("invalid role in claims", func(t *testing.T) {
		claims := &Claims{
			Role: "invalid",
		}

		err := pc.CheckClaimsPermission(claims, PermRoomJoin)
		if err == nil {
			t.Error("expected permission denied for invalid role")
		}
	})

	t.Run("empty claims", func(t *testing.T) {
		claims := &Claims{}

		err := pc.CheckClaimsPermission(claims, PermRoomJoin)
		if err == nil {
			t.Error("expected permission denied for empty claims")
		}
	})

	t.Run("empty permissions slice falls back to role", func(t *testing.T) {
		claims := &Claims{
			Role:        "publisher",
			Permissions: []string{}, // Empty slice should not block role-based check
		}

		err := pc.CheckClaimsPermission(claims, PermMediaPublish)
		if err != nil {
			t.Errorf("expected role-based permission to be granted, got: %v", err)
		}
	})

	t.Run("explicit permissions combined with role fallback", func(t *testing.T) {
		claims := &Claims{
			Role:        "publisher",
			Permissions: []string{"custom:permission"}, // Has explicit but not the one we need
		}

		// Should still get role-based permission
		err := pc.CheckClaimsPermission(claims, PermMediaPublish)
		if err != nil {
			t.Errorf("expected role-based fallback to grant permission, got: %v", err)
		}
	})
}

func TestPermissionChecker_ClaimsHasPermission(t *testing.T) {
	pc := NewPermissionChecker()

	claims := &Claims{
		Role: "admin",
	}

	if !pc.ClaimsHasPermission(claims, PermRoomCreate) {
		t.Error("admin claims should have room:create permission")
	}

	if pc.ClaimsHasPermission(claims, Permission("invalid:permission")) {
		t.Error("should not have invalid permission")
	}
}

func TestRoleHierarchy(t *testing.T) {
	tests := []struct {
		role1    Role
		role2    Role
		expected bool
	}{
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleModerator, true},
		{RoleAdmin, RolePublisher, true},
		{RoleAdmin, RoleSubscriber, true},
		{RoleModerator, RoleAdmin, false},
		{RoleModerator, RoleModerator, true},
		{RoleModerator, RolePublisher, true},
		{RoleModerator, RoleSubscriber, true},
		{RolePublisher, RoleAdmin, false},
		{RolePublisher, RoleModerator, false},
		{RolePublisher, RolePublisher, true},
		{RolePublisher, RoleSubscriber, true},
		{RoleSubscriber, RoleAdmin, false},
		{RoleSubscriber, RolePublisher, false},
		{RoleSubscriber, RoleSubscriber, true},
		{Role("invalid"), RoleAdmin, false},
		{RoleAdmin, Role("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role1)+">="+string(tt.role2), func(t *testing.T) {
			result := RoleHierarchy(tt.role1, tt.role2)
			if result != tt.expected {
				t.Errorf("RoleHierarchy(%s, %s) = %v, expected %v",
					tt.role1, tt.role2, result, tt.expected)
			}
		})
	}
}

func TestCanManageRole(t *testing.T) {
	tests := []struct {
		manager  Role
		target   Role
		expected bool
	}{
		// Admin can manage everyone except admins
		{RoleAdmin, RoleModerator, true},
		{RoleAdmin, RolePublisher, true},
		{RoleAdmin, RoleSubscriber, true},
		{RoleAdmin, RoleAdmin, false},

		// Moderator can manage publishers and subscribers
		{RoleModerator, RolePublisher, true},
		{RoleModerator, RoleSubscriber, true},
		{RoleModerator, RoleModerator, false},
		{RoleModerator, RoleAdmin, false},

		// Publisher cannot manage anyone
		{RolePublisher, RoleSubscriber, false},
		{RolePublisher, RolePublisher, false},

		// Subscriber cannot manage anyone
		{RoleSubscriber, RoleSubscriber, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.manager)+"-manages-"+string(tt.target), func(t *testing.T) {
			result := CanManageRole(tt.manager, tt.target)
			if result != tt.expected {
				t.Errorf("CanManageRole(%s, %s) = %v, expected %v",
					tt.manager, tt.target, result, tt.expected)
			}
		})
	}
}

func TestPermissionChecker_GetRolePermissions(t *testing.T) {
	pc := NewPermissionChecker()

	t.Run("returns default permissions for admin", func(t *testing.T) {
		perms := pc.GetRolePermissions(RoleAdmin)
		if perms == nil || len(perms) == 0 {
			t.Error("expected non-empty permissions for admin")
		}
	})

	t.Run("returns nil for invalid role", func(t *testing.T) {
		perms := pc.GetRolePermissions(Role("invalid"))
		if perms != nil {
			t.Error("expected nil permissions for invalid role")
		}
	})

	t.Run("returns custom permissions when set", func(t *testing.T) {
		pc.SetRolePermissions(Role("custom"), []Permission{PermRoomJoin})
		perms := pc.GetRolePermissions(Role("custom"))
		if len(perms) != 1 || perms[0] != PermRoomJoin {
			t.Error("expected custom permissions to be returned")
		}
	})
}
