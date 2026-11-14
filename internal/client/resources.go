package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ---------------------------------------------------------------------------
// Lists
// ---------------------------------------------------------------------------

// List is a mailing list.
type List struct {
	ID              int64    `json:"id,omitempty"`
	UUID            string   `json:"uuid,omitempty"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Optin           string   `json:"optin"`
	Status          string   `json:"status,omitempty"`
	Tags            []string `json:"tags"`
	Description     string   `json:"description"`
	SubscriberCount int64    `json:"subscriber_count,omitempty"`
}

func (c *Client) GetList(ctx context.Context, id int64) (*List, error) {
	var out List
	err := c.do(ctx, http.MethodGet, "/lists/"+itoa(id), nil, nil, &out)
	return &out, err
}

// GetLists returns every list the API user can see, unpaginated
// (minimal=true; subscriber counts are not included).
func (c *Client) GetLists(ctx context.Context) ([]List, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/lists", url.Values{"minimal": {"true"}}, nil, &raw); err != nil {
		return nil, err
	}
	// With no lists at all listmonk returns a bare [] instead of a page.
	if len(raw) > 0 && raw[0] == '[' {
		var out []List
		return out, json.Unmarshal(raw, &out)
	}
	var page struct {
		Results []List `json:"results"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, fmt.Errorf("decoding lists: %w", err)
	}
	return page.Results, nil
}

func (c *Client) CreateList(ctx context.Context, l List) (*List, error) {
	var out List
	err := c.do(ctx, http.MethodPost, "/lists", nil, l, &out)
	return &out, err
}

func (c *Client) UpdateList(ctx context.Context, id int64, l List) (*List, error) {
	var out List
	err := c.do(ctx, http.MethodPut, "/lists/"+itoa(id), nil, l, &out)
	return &out, err
}

func (c *Client) DeleteList(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, "/lists/"+itoa(id), nil, nil, nil)
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

// Template is an e-mail template.
type Template struct {
	ID         int64   `json:"id,omitempty"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Subject    string  `json:"subject"`
	Body       string  `json:"body"`
	BodySource *string `json:"body_source,omitempty"`
	IsDefault  bool    `json:"is_default,omitempty"`
}

func (c *Client) GetTemplate(ctx context.Context, id int64) (*Template, error) {
	var out Template
	err := c.do(ctx, http.MethodGet, "/templates/"+itoa(id), nil, nil, &out)
	return &out, err
}

// GetTemplates returns all templates without their bodies.
func (c *Client) GetTemplates(ctx context.Context) ([]Template, error) {
	var out []Template
	err := c.do(ctx, http.MethodGet, "/templates", url.Values{"no_body": {"true"}}, nil, &out)
	return out, err
}

func (c *Client) CreateTemplate(ctx context.Context, t Template) (*Template, error) {
	var out Template
	err := c.do(ctx, http.MethodPost, "/templates", nil, t, &out)
	return &out, err
}

func (c *Client) UpdateTemplate(ctx context.Context, id int64, t Template) error {
	// The update response body is not consistent across listmonk versions;
	// callers re-read the template instead.
	return c.do(ctx, http.MethodPut, "/templates/"+itoa(id), nil, t, nil)
}

func (c *Client) SetDefaultTemplate(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodPut, "/templates/"+itoa(id)+"/default", nil, nil, nil)
}

func (c *Client) DeleteTemplate(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, "/templates/"+itoa(id), nil, nil, nil)
}

// ---------------------------------------------------------------------------
// Subscribers
// ---------------------------------------------------------------------------

// Subscriber as returned by the API.
type Subscriber struct {
	ID      int64                  `json:"id"`
	UUID    string                 `json:"uuid"`
	Email   string                 `json:"email"`
	Name    string                 `json:"name"`
	Status  string                 `json:"status"`
	Attribs map[string]any         `json:"attribs"`
	Lists   []SubscriberListStatus `json:"lists"`
}

// SubscriberListStatus is one list membership of a subscriber.
type SubscriberListStatus struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	SubscriptionStatus string `json:"subscription_status"`
}

// SubscriberRequest is the create/update payload.
type SubscriberRequest struct {
	Email                   string         `json:"email"`
	Name                    string         `json:"name"`
	Status                  string         `json:"status"`
	Attribs                 map[string]any `json:"attribs"`
	Lists                   []int64        `json:"lists"`
	PreconfirmSubscriptions bool           `json:"preconfirm_subscriptions"`
}

func (c *Client) GetSubscriber(ctx context.Context, id int64) (*Subscriber, error) {
	var out Subscriber
	err := c.do(ctx, http.MethodGet, "/subscribers/"+itoa(id), nil, nil, &out)
	return &out, err
}

func (c *Client) CreateSubscriber(ctx context.Context, s SubscriberRequest) (*Subscriber, error) {
	var out Subscriber
	err := c.do(ctx, http.MethodPost, "/subscribers", nil, s, &out)
	return &out, err
}

func (c *Client) UpdateSubscriber(ctx context.Context, id int64, s SubscriberRequest) (*Subscriber, error) {
	var out Subscriber
	err := c.do(ctx, http.MethodPut, "/subscribers/"+itoa(id), nil, s, &out)
	return &out, err
}

func (c *Client) DeleteSubscriber(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, "/subscribers/"+itoa(id), nil, nil, nil)
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

// User is an admin or API user.
type User struct {
	ID            int64   `json:"id,omitempty"`
	Username      string  `json:"username"`
	Name          string  `json:"name"`
	Email         string  `json:"email,omitempty"`
	Type          string  `json:"type"`
	Status        string  `json:"status"`
	PasswordLogin bool    `json:"password_login"`
	Password      *string `json:"password,omitempty"`
	UserRoleID    int64   `json:"user_role_id,omitempty"`
	ListRoleID    *int64  `json:"list_role_id"`

	// Read-only role views returned by GET.
	UserRole *struct {
		ID int64 `json:"id"`
	} `json:"user_role,omitempty"`
	ListRole *struct {
		ID int64 `json:"id"`
	} `json:"list_role,omitempty"`
}

// RoleIDs returns the user and list role IDs from whichever representation
// the server used.
func (u *User) RoleIDs() (userRole int64, listRole *int64) {
	userRole = u.UserRoleID
	if userRole == 0 && u.UserRole != nil {
		userRole = u.UserRole.ID
	}
	listRole = u.ListRoleID
	if listRole == nil && u.ListRole != nil && u.ListRole.ID != 0 {
		id := u.ListRole.ID
		listRole = &id
	}
	return userRole, listRole
}

func (c *Client) GetUser(ctx context.Context, id int64) (*User, error) {
	var out User
	err := c.do(ctx, http.MethodGet, "/users/"+itoa(id), nil, nil, &out)
	return &out, err
}

// CreateUser creates a user. For type "api" the returned Password holds the
// generated API token; listmonk returns it only once.
func (c *Client) CreateUser(ctx context.Context, u User) (*User, error) {
	var out User
	err := c.do(ctx, http.MethodPost, "/users", nil, u, &out)
	return &out, err
}

func (c *Client) UpdateUser(ctx context.Context, id int64, u User) (*User, error) {
	var out User
	err := c.do(ctx, http.MethodPut, "/users/"+itoa(id), nil, u, &out)
	return &out, err
}

func (c *Client) DeleteUser(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, "/users/"+itoa(id), nil, nil, nil)
}

// ---------------------------------------------------------------------------
// Roles
// ---------------------------------------------------------------------------

// UserRole is a global permission role.
type UserRole struct {
	ID          int64    `json:"id,omitempty"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

// ListRole grants permissions on individual lists.
type ListRole struct {
	ID    int64            `json:"id,omitempty"`
	Name  string           `json:"name"`
	Lists []ListPermission `json:"lists"`
}

// ListPermission is the permission set of a list role on one list.
type ListPermission struct {
	ID          int64    `json:"id"`
	Permissions []string `json:"permissions"`
}

// GetUserRoles lists all user roles; listmonk has no GET-by-ID for roles.
func (c *Client) GetUserRoles(ctx context.Context) ([]UserRole, error) {
	var out []UserRole
	err := c.do(ctx, http.MethodGet, "/roles/users", nil, nil, &out)
	return out, err
}

// GetUserRole finds a user role by ID. It returns an *APIError with status
// 404 when there is none.
func (c *Client) GetUserRole(ctx context.Context, id int64) (*UserRole, error) {
	roles, err := c.GetUserRoles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		if roles[i].ID == id {
			return &roles[i], nil
		}
	}
	return nil, notFound("/roles/users/" + itoa(id))
}

func (c *Client) CreateUserRole(ctx context.Context, r UserRole) (*UserRole, error) {
	var out UserRole
	err := c.do(ctx, http.MethodPost, "/roles/users", nil, r, &out)
	return &out, err
}

func (c *Client) UpdateUserRole(ctx context.Context, id int64, r UserRole) error {
	return c.do(ctx, http.MethodPut, "/roles/users/"+itoa(id), nil, r, nil)
}

func (c *Client) GetListRoles(ctx context.Context) ([]ListRole, error) {
	var out []ListRole
	err := c.do(ctx, http.MethodGet, "/roles/lists", nil, nil, &out)
	return out, err
}

// GetListRole finds a list role by ID. It returns an *APIError with status
// 404 when there is none.
func (c *Client) GetListRole(ctx context.Context, id int64) (*ListRole, error) {
	roles, err := c.GetListRoles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		if roles[i].ID == id {
			return &roles[i], nil
		}
	}
	return nil, notFound("/roles/lists/" + itoa(id))
}

func (c *Client) CreateListRole(ctx context.Context, r ListRole) (*ListRole, error) {
	var out ListRole
	err := c.do(ctx, http.MethodPost, "/roles/lists", nil, r, &out)
	return &out, err
}

func (c *Client) UpdateListRole(ctx context.Context, id int64, r ListRole) error {
	return c.do(ctx, http.MethodPut, "/roles/lists/"+itoa(id), nil, r, nil)
}

// DeleteRole deletes a user or list role.
func (c *Client) DeleteRole(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, "/roles/"+itoa(id), nil, nil, nil)
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

// GetSettings returns all settings keyed by their dotted name
// ("app.site_name", "smtp", ...). Secrets are masked by the server.
func (c *Client) GetSettings(ctx context.Context) (map[string]json.RawMessage, error) {
	var out map[string]json.RawMessage
	err := c.do(ctx, http.MethodGet, "/settings", nil, nil, &out)
	return out, err
}

// UpdateSetting sets one settings key to a raw JSON value. listmonk reloads
// itself afterwards; the client retries the next requests while it does.
//
// The server silently ignores unknown keys, so callers must check that the
// key exists first.
//
// PUT /api/settings/{key} only exists since listmonk v6. On older servers
// the client falls back to rewriting the whole settings object.
func (c *Client) UpdateSetting(ctx context.Context, key string, value json.RawMessage) error {
	if !c.noSettingsByKey.Load() {
		err := c.do(ctx, http.MethodPut, "/settings/"+url.PathEscape(key), nil, value, nil)
		var apiErr *APIError
		if err == nil || !errors.As(err, &apiErr) ||
			(apiErr.StatusCode != http.StatusNotFound && apiErr.StatusCode != http.StatusMethodNotAllowed) {
			return err
		}
		c.noSettingsByKey.Store(true)
	}
	return c.updateSettingFull(ctx, key, value)
}

// updateSettingFull replaces one key through PUT /api/settings, which takes
// the complete settings object. Secrets come back masked from GET; they are
// sent as "" so listmonk keeps the stored values (it restores every masked
// field when it is blank).
func (c *Client) updateSettingFull(ctx context.Context, key string, value json.RawMessage) error {
	settings, err := c.GetSettings(ctx)
	if err != nil {
		return err
	}
	all := make(map[string]any, len(settings))
	for k, raw := range settings {
		var v any
		if err := decodeJSON(raw, &v); err != nil {
			return fmt.Errorf("decoding setting %q: %w", k, err)
		}
		all[k] = blankMasked(v)
	}
	var v any
	if err := decodeJSON(value, &v); err != nil {
		return fmt.Errorf("decoding value for %q: %w", key, err)
	}
	all[key] = v
	return c.do(ctx, http.MethodPut, "/settings", nil, all, nil)
}

// ---------------------------------------------------------------------------

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

func notFound(path string) error {
	return &APIError{Method: http.MethodGet, Path: path, StatusCode: http.StatusNotFound, Message: fmt.Sprintf("%s not found", path)}
}
