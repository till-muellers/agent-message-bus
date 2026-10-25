package invites

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type acceptBody struct {
	Name, Space, Token string
	Task               *string
	Error              struct{ Code, Message string }
}

func postAccept(t *testing.T, e env, authz string) (int, acceptBody) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/invites/accept", nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	e.svc.AcceptHandler().ServeHTTP(rec, req)
	var body acceptBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body.String(), err)
	}
	return rec.Code, body
}

func TestAcceptHandler(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	inv := mustCreate(t, e, "worker", nil)
	status, body := postAccept(t, e, "Bearer "+inv.Code)
	if status != http.StatusOK || body.Name != "worker" || body.Space != "demo" || body.Token == "" || body.Task != nil {
		t.Fatalf("accept = %d %+v", status, body)
	}
	tests := []struct{ name, authz, msg string }{
		{name: "missing bearer", authz: "", msg: "bearer token required"},
		{name: "empty bearer", authz: "Bearer ", msg: "bearer token required"},
		{name: "used code", authz: "Bearer " + inv.Code, msg: "invalid or expired invite code"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, body := postAccept(t, e, tc.authz)
			if status != http.StatusUnauthorized || body.Error.Code != "unauthenticated" || body.Error.Message != tc.msg {
				t.Fatalf("got %d %+v", status, body)
			}
		})
	}
}

func TestAcceptHandlerReturnsTask(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	task := "fix the bug"
	status, body := postAccept(t, e, "Bearer "+mustCreate(t, e, "worker", &task).Code)
	if status != http.StatusOK || body.Task == nil || *body.Task != task {
		t.Fatalf("accept = %d %+v", status, body)
	}
}
