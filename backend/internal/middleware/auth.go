package middleware

import (
	"net/http"
	"strings"

	"marketingflow/internal/authmw"
	"marketingflow/internal/model"

	"github.com/gin-gonic/gin"
)

const (
	ctxUserID = "auth_user_id"
	ctxRole   = "auth_role"
	ctxEmail  = "auth_email"
)

// Auth validates the Bearer token and stores the identity in the gin context.
// Accepts the native marketing JWT OR (when sso != nil) the unified dashboard's
// Ed25519 SSO login token — so the dashboard can call us with ONE login.
func Auth(tm *TokenManager, sso *authmw.Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		raw := strings.TrimPrefix(header, "Bearer ")
		if claims, err := tm.Parse(raw); err == nil {
			c.Set(ctxUserID, claims.UserID)
			c.Set(ctxRole, claims.Role)
			c.Set(ctxEmail, claims.Email)
			c.Next()
			return
		}
		if role, email, ok := SSOIdentity(sso, raw); ok {
			c.Set(ctxUserID, uint(0)) // SSO user has no native marketing id
			c.Set(ctxRole, role)
			c.Set(ctxEmail, email)
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
	}
}

// SSOIdentity verifies a unified dashboard SSO token for the marketing
// department and returns the mapped role + email. Super users map to kadep.
func SSOIdentity(sso *authmw.Verifier, raw string) (model.Role, string, bool) {
	if sso == nil {
		return "", "", false
	}
	cl, err := sso.Verify(raw)
	if err != nil || !cl.CanAccess("marketing") {
		return "", "", false
	}
	role := model.Role(cl.Role("marketing"))
	if role == "" || cl.Super {
		role = model.RoleKadep
	}
	email := cl.Email
	if email == "" {
		email = cl.Username
	}
	return role, email, true
}

// RequireRole restricts a route to the given roles.
func RequireRole(roles ...model.Role) gin.HandlerFunc {
	allowed := make(map[model.Role]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, _ := c.Get(ctxRole)
		if _, ok := allowed[role.(model.Role)]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden for role"})
			return
		}
		c.Next()
	}
}

// CurrentUserID returns the authenticated user id from the context.
func CurrentUserID(c *gin.Context) uint {
	if v, ok := c.Get(ctxUserID); ok {
		return v.(uint)
	}
	return 0
}

// CurrentRole returns the authenticated role from the context.
func CurrentRole(c *gin.Context) model.Role {
	if v, ok := c.Get(ctxRole); ok {
		return v.(model.Role)
	}
	return ""
}
