package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL + "/", Username: "tf", Token: "secret", MaxRetries: 3})
	if err != nil {
		t.Fatal(err)
	}
	c.retryWait = time.Millisecond
	return c
}

func TestNewValidation(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no url":     {Username: "u", Token: "t"},
		"bad scheme": {BaseURL: "ftp://x", Username: "u", Token: "t"},
		"no user":    {BaseURL: "https://x", Token: "t"},
		"no token":   {BaseURL: "https://x", Username: "u"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestAuthHeaderAndPath(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token tf:secret" {
			t.Errorf("Authorization = %q", got)
		}
		if r.URL.Path != "/api/lists/7" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"id":7,"name":"news","type":"public","optin":"double","tags":["a"]}}`))
	})
	l, err := c.GetList(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if l.ID != 7 || l.Name != "news" || l.Optin != "double" || len(l.Tags) != 1 {
		t.Errorf("unexpected list %+v", l)
	}
}

func TestNotFound(t *testing.T) {
	for _, code := range []int{http.StatusBadRequest, http.StatusNotFound} {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"list not found"}`))
		})
		_, err := c.GetList(context.Background(), 1)
		if !IsNotFound(err) {
			t.Errorf("HTTP %d: IsNotFound = false for %v", code, err)
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Message != "list not found" {
			t.Errorf("HTTP %d: message not decoded: %v", code, err)
		}
	}

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"permission denied"}`))
	})
	if _, err := c.GetList(context.Background(), 1); IsNotFound(err) {
		t.Error("403 must not count as not found")
	}
}

func TestRetriesTransientErrors(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"data":true}`))
	})
	if err := c.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	})
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	if calls.Load() != 4 { // 1 + 3 retries
		t.Errorf("calls = %d, want 4", calls.Load())
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnprocessableEntity)
	})
	_ = c.Health(context.Background())
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestGetListsEmptyArray(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("minimal") != "true" {
			t.Errorf("minimal not set: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	lists, err := c.GetLists(context.Background())
	if err != nil || len(lists) != 0 {
		t.Fatalf("lists=%v err=%v", lists, err)
	}
}

func TestGetUserRoleNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"Super Admin","permissions":[]}]}`))
	})
	if _, err := c.GetUserRole(context.Background(), 2); !IsNotFound(err) {
		t.Errorf("expected not found, got %v", err)
	}
	r, err := c.GetUserRole(context.Background(), 1)
	if err != nil || r.Name != "Super Admin" {
		t.Errorf("r=%v err=%v", r, err)
	}
}

func TestUpdateSettingSendsRawJSON(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/settings/app.site_name" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var v string
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil || v != "My list" {
			t.Errorf("body %q err=%v", v, err)
		}
		_, _ = w.Write([]byte(`{"data":true}`))
	})
	if err := c.UpdateSetting(context.Background(), "app.site_name", json.RawMessage(`"My list"`)); err != nil {
		t.Fatal(err)
	}
}

// listmonk < v6 has no PUT /api/settings/{key}: the client must fall back
// to PUT /api/settings with the full object, blanking masked secrets so the
// server keeps the stored ones.
func TestUpdateSettingFallback(t *testing.T) {
	var byKeyCalls, fullCalls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/settings/app.site_name":
			byKeyCalls.Add(1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/settings":
			_, _ = w.Write([]byte(`{"data":{"app.site_name":"old","app.batch_size":1000,` +
				`"smtp":[{"uuid":"u1","host":"h","password":"••••"}]}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/settings":
			fullCalls.Add(1)
			var got map[string]any
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got["app.site_name"] != "new" {
				t.Errorf("site_name = %v", got["app.site_name"])
			}
			if got["app.batch_size"] != float64(1000) {
				t.Errorf("batch_size = %v", got["app.batch_size"])
			}
			smtp := got["smtp"].([]any)[0].(map[string]any)
			if smtp["password"] != "" || smtp["uuid"] != "u1" {
				t.Errorf("smtp = %v", smtp)
			}
			_, _ = w.Write([]byte(`{"data":true}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	for i := 0; i < 2; i++ {
		if err := c.UpdateSetting(context.Background(), "app.site_name", json.RawMessage(`"new"`)); err != nil {
			t.Fatal(err)
		}
	}
	// The missing endpoint is probed once, then remembered.
	if byKeyCalls.Load() != 1 || fullCalls.Load() != 2 {
		t.Errorf("byKey=%d full=%d", byKeyCalls.Load(), fullCalls.Load())
	}
}

func TestUserRoleIDs(t *testing.T) {
	var u User
	if err := json.Unmarshal([]byte(`{"id":3,"user_role":{"id":5},"list_role":{"id":9}}`), &u); err != nil {
		t.Fatal(err)
	}
	ur, lr := u.RoleIDs()
	if ur != 5 || lr == nil || *lr != 9 {
		t.Errorf("RoleIDs = %d, %v", ur, lr)
	}
	u = User{}
	_ = json.Unmarshal([]byte(`{"id":3,"user_role_id":2,"list_role":null}`), &u)
	ur, lr = u.RoleIDs()
	if ur != 2 || lr != nil {
		t.Errorf("RoleIDs = %d, %v", ur, lr)
	}
}
