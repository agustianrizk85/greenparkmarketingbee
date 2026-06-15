package middleware

import (
	"net/http"
	"strings"

	"marketingflow/internal/model"

	"github.com/gin-gonic/gin"
)

const (
	ctxUserID = "auth_user_id"
	ctxRole   = "auth_role"
	ctxEmail  = "auth_email"
)

// Auth validates the Bearer token and stores the identity in the gin context.
func Auth(tm *TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		raw := strings.TrimPrefix(header, "Bearer ")
		claims, err := tm.Parse(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxRole, claims.Role)
		c.Set(ctxEmail, claims.Email)
		c.Next()
	}
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
