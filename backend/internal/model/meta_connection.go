package model

import "time"

// MetaAppConfig is the dynamic OAuth credentials for the Meta (Facebook) app
// used to run the Login flow. It is a singleton row (ID = 1) so the App ID /
// Secret can be set from the UI instead of being baked into env — the team
// registers one Meta App and pastes its credentials once.
type MetaAppConfig struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	AppID       string    `gorm:"size:64" json:"app_id"`
	AppSecret   string    `gorm:"size:255" json:"-"` // never serialised to the client
	RedirectURI string    `gorm:"size:255" json:"redirect_uri"`
	APIVersion  string    `gorm:"size:16" json:"api_version"`
	Scopes      string    `gorm:"size:512" json:"scopes"` // comma-separated Graph permissions
	UpdatedAt   time.Time `json:"updated_at"`
}

// HasSecret reports whether an app secret has been stored (the value itself is
// never returned to the client).
func (c *MetaAppConfig) HasSecret() bool { return c.AppSecret != "" }

// MetaConnection is one connected Meta account (one OAuth login → one access
// token). A login may grant access to several ad accounts / pages / WABAs; the
// connection holds the long-lived user token. Exactly one connection is active
// at a time and supplies the token the Graph proxy uses. Multi-account: connect
// as many as needed and switch the active one.
type MetaConnection struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Label          string     `gorm:"size:160" json:"label"`         // editable display name
	MetaUserID     string     `gorm:"size:64;index" json:"meta_user_id"`
	MetaUserName   string     `gorm:"size:160" json:"meta_user_name"`
	AccessToken    string     `gorm:"size:1024" json:"-"` // long-lived token, server-side only
	TokenExpiresAt *time.Time `json:"token_expires_at"`
	BusinessID     string     `gorm:"size:64" json:"business_id"`
	AdAccountID    string     `gorm:"size:64" json:"ad_account_id"` // pinned ad account (without act_), optional
	Scopes         string     `gorm:"size:512" json:"scopes"`
	IsActive       bool       `gorm:"index" json:"is_active"`
	CreatedBy      uint       `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
