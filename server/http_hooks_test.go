package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/user-attribute-sync-starter-template/server/sync"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newTestPlugin returns a Plugin backed by a mocked server API, with its HTTP
// router initialized so requests can be driven through ServeHTTP exactly the
// way the Mattermost server drives them.
func newTestPlugin(t *testing.T) (*Plugin, *plugintest.API) {
	t.Helper()

	api := &plugintest.API{}
	t.Cleanup(func() { api.AssertExpectations(t) })
	mockLogs(api)

	client := pluginapi.NewClient(api, &plugintest.Driver{})
	kvStoreProvider := sync.NewKVStoreProvider(client)

	p := &Plugin{
		MattermostPlugin:  plugin.MattermostPlugin{API: api},
		client:            client,
		attributeProvider: kvStoreProvider,
	}
	p.initializeAPI()

	return p, api
}

// mockLogs registers permissive expectations for the log methods so tests that
// exercise error paths don't have to match every log line exactly.
func mockLogs(api *plugintest.API) {
	const maxLogFields = 8
	for _, method := range []string{"LogDebug", "LogInfo", "LogWarn", "LogError"} {
		for n := 0; n <= maxLogFields; n++ {
			args := make([]interface{}, n+1)
			for i := range args {
				args[i] = mock.Anything
			}
			api.On(method, args...).Maybe()
		}
	}
}

// asSysadmin lets userID through the requireSysadmin middleware.
func asSysadmin(api *plugintest.API, userID string) {
	api.On("HasPermissionTo", userID, model.PermissionManageSystem).Return(true).Once()
}

// doRequest drives a request through the plugin's real router. An empty userID
// simulates an unauthenticated request: the Mattermost server only sets the
// Mattermost-User-Id header once it has verified the session.
func doRequest(t *testing.T, p *Plugin, method, path, userID string, body []byte) *http.Response {
	t.Helper()

	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if userID != "" {
		r.Header.Set("Mattermost-User-Id", userID)
	}

	w := httptest.NewRecorder()
	p.ServeHTTP(&plugin.Context{}, w, r)

	return w.Result()
}

// requireErrorResponse asserts the handler failed with the given status and
// message, which errorWithJSON renders as {"error": "..."}.
func requireErrorResponse(t *testing.T, resp *http.Response, statusCode int, message string) {
	t.Helper()

	require.Equal(t, statusCode, resp.StatusCode)

	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, message, body.Error)
}

// TestUserAttributesAccessControl covers the two rejection paths the requireSysadmin middleware
// applies to every endpoint, so the individual handler tests below can assume an authorized admin.
// Driving each method and path proves the middleware is actually reached by all of them, which is
// the property that replaced a per-handler check.
func TestUserAttributesAccessControl(t *testing.T) {
	endpoints := []struct {
		name   string
		method string
		path   string
	}{
		{"upload", http.MethodPost, "/user_attributes"},
		{"download", http.MethodGet, "/user_attributes"},
		{"status", http.MethodGet, "/user_attributes/status"},
		{"delete", http.MethodDelete, "/user_attributes"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name+"/not logged in", func(t *testing.T) {
			p, _ := newTestPlugin(t)

			resp := doRequest(t, p, endpoint.method, endpoint.path, "", nil)
			requireErrorResponse(t, resp, http.StatusUnauthorized, "not logged in")
		})

		t.Run(endpoint.name+"/not a sysadmin", func(t *testing.T) {
			p, api := newTestPlugin(t)

			userID := model.NewId()
			api.On("HasPermissionTo", userID, model.PermissionManageSystem).Return(false).Once()

			resp := doRequest(t, p, endpoint.method, endpoint.path, userID, nil)
			requireErrorResponse(t, resp, http.StatusForbidden, "not authorized")
		})
	}
}

// TestHandleUploadUserAttributes covers the upload path: what is stored, and the four ways a
// request is refused (malformed JSON, wrong shape, too large, and a KV write that did not take).
func TestHandleUploadUserAttributes(t *testing.T) {
	validFile := []byte(`[{"email":"user1@example.com","job_title":"Engineer"}]`)

	t.Run("stores the uploaded file verbatim", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVSetWithOptions", sync.UserAttrsStoreKey, validFile, model.PluginKVSetOptions{}).
			Return(true, nil).Once()
		api.On("KVSetWithOptions", sync.UserAttrsLastUpdatedKey, mock.Anything, model.PluginKVSetOptions{}).
			Return(true, nil).Once()

		resp := doRequest(t, p, http.MethodPost, "/user_attributes", userID, validFile)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var body userAttributesStatus
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.True(t, body.Exists)
		// Not checking exact time since it is determined by the handler.
		require.NotNil(t, body.LastUpdated)
	})

	// The file is what matters, so a failed timestamp write is still a successful upload. It is
	// reported as a file with no timestamp so the end user can be made aware.
	t.Run("reports no timestamp when the timestamp write fails", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVSetWithOptions", sync.UserAttrsStoreKey, validFile, model.PluginKVSetOptions{}).
			Return(true, nil).Once()
		api.On("KVSetWithOptions", sync.UserAttrsLastUpdatedKey, mock.Anything, model.PluginKVSetOptions{}).
			Return(false, nil).Once()

		resp := doRequest(t, p, http.MethodPost, "/user_attributes", userID, validFile)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var body userAttributesStatus
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.True(t, body.Exists)
		require.Nil(t, body.LastUpdated)
	})

	t.Run("rejects malformed json", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)

		resp := doRequest(t, p, http.MethodPost, "/user_attributes", userID, []byte(`[{"email":`))
		requireErrorResponse(t, resp, http.StatusBadRequest, "invalid json - must be array of objects")
	})

	t.Run("rejects valid json that is not an array of objects", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)

		resp := doRequest(t, p, http.MethodPost, "/user_attributes", userID, []byte(`{"email":"user1@example.com"}`))
		requireErrorResponse(t, resp, http.StatusBadRequest, "invalid json - must be array of objects")
	})

	t.Run("rejects a file over the size limit", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)

		// One byte past the limit is enough to trip MaxBytesReader, and the body
		// never has to be valid JSON because it is rejected before parsing.
		oversized := bytes.Repeat([]byte("a"), maxFileSizeBytes+1)

		resp := doRequest(t, p, http.MethodPost, "/user_attributes", userID, oversized)
		requireErrorResponse(t, resp, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("file exceeds %d byte limit", maxFileSizeBytes))
	})

	t.Run("reports a failed write", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVSetWithOptions", sync.UserAttrsStoreKey, validFile, model.PluginKVSetOptions{}).
			Return(false, model.NewAppError("KVSetWithOptions", "kv.set.app_error", nil, "connection refused", http.StatusInternalServerError)).Once()

		resp := doRequest(t, p, http.MethodPost, "/user_attributes", userID, validFile)
		requireErrorResponse(t, resp, http.StatusInternalServerError, "failed to upload file")
	})

	t.Run("reports a write that did not happen", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVSetWithOptions", sync.UserAttrsStoreKey, validFile, model.PluginKVSetOptions{}).
			Return(false, nil).Once()

		resp := doRequest(t, p, http.MethodPost, "/user_attributes", userID, validFile)
		requireErrorResponse(t, resp, http.StatusInternalServerError, "failed to upload file, please try again")
	})
}

// TestHandleDownloadUserAttributes checks that a download returns the stored bytes unchanged,
// and that "nothing stored" is a 404 rather than an empty 200.
func TestHandleDownloadUserAttributes(t *testing.T) {
	t.Run("returns the stored file", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		stored := []byte(`[{"email":"user1@example.com","job_title":"Engineer"}]`)
		asSysadmin(api, userID)
		api.On("KVGet", sync.UserAttrsStoreKey).Return(stored, nil).Once()

		resp := doRequest(t, p, http.MethodGet, "/user_attributes", userID, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := readAll(resp)
		require.NoError(t, err)
		require.Equal(t, stored, body)
	})

	t.Run("reports no stored file", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVGet", sync.UserAttrsStoreKey).Return(nil, nil).Once()

		resp := doRequest(t, p, http.MethodGet, "/user_attributes", userID, nil)
		requireErrorResponse(t, resp, http.StatusNotFound, "file not found")
	})

	t.Run("reports a failed read", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVGet", sync.UserAttrsStoreKey).
			Return(nil, model.NewAppError("KVGet", "kv.get.app_error", nil, "connection refused", http.StatusInternalServerError)).Once()

		resp := doRequest(t, p, http.MethodGet, "/user_attributes", userID, nil)
		requireErrorResponse(t, resp, http.StatusInternalServerError, "failed to download file")
	})
}

// TestHandleUserAttributesStatus covers the endpoint the settings UI polls on load.
func TestHandleUserAttributesStatus(t *testing.T) {
	uploadedAt, err := time.Parse(time.RFC3339, "2026-08-20T15:04:05Z")
	require.NoError(t, err)
	storedTime, err := uploadedAt.MarshalJSON()
	require.NoError(t, err)

	t.Run("reports a stored file with upload time", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVGet", sync.UserAttrsStoreKey).Return([]byte(`[{"email":"user1@example.com"}]`), nil).Once()
		api.On("KVGet", sync.UserAttrsLastUpdatedKey).Return(storedTime, nil).Once()

		resp := doRequest(t, p, http.MethodGet, "/user_attributes/status", userID, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var body userAttributesStatus
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.True(t, body.Exists)
		require.NotNil(t, body.LastUpdated)
		require.True(t, uploadedAt.Equal(*body.LastUpdated))
	})

	t.Run("reports no stored file", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVGet", sync.UserAttrsStoreKey).Return(nil, nil).Once()
		api.On("KVGet", sync.UserAttrsLastUpdatedKey).Return(nil, nil).Once()

		resp := doRequest(t, p, http.MethodGet, "/user_attributes/status", userID, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var body userAttributesStatus
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.False(t, body.Exists)
		require.Nil(t, body.LastUpdated)
	})

	// A timestamp that will not parse is not worth failing the request over: the file is still
	// there and still downloadable, so the nil lastUpdate is sufficoent red flag.
	t.Run("reports a stored file whose timestamp is malformed", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVGet", sync.UserAttrsStoreKey).Return([]byte(`[{"email":"user1@example.com"}]`), nil).Once()
		api.On("KVGet", sync.UserAttrsLastUpdatedKey).Return([]byte("not a timestamp"), nil).Once()

		resp := doRequest(t, p, http.MethodGet, "/user_attributes/status", userID, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var body userAttributesStatus
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.True(t, body.Exists)
		require.Nil(t, body.LastUpdated)
	})

	t.Run("reports a failed read", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVGet", sync.UserAttrsStoreKey).
			Return(nil, model.NewAppError("KVGet", "kv.get.app_error", nil, "connection refused", http.StatusInternalServerError)).Once()

		resp := doRequest(t, p, http.MethodGet, "/user_attributes/status", userID, nil)
		requireErrorResponse(t, resp, http.StatusInternalServerError, "failed to access storage")
	})

	t.Run("reports a failed timestamp read", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVGet", sync.UserAttrsStoreKey).Return([]byte(`[{"email":"user1@example.com"}]`), nil).Once()
		api.On("KVGet", sync.UserAttrsLastUpdatedKey).
			Return(nil, model.NewAppError("KVGet", "kv.get.app_error", nil, "connection refused", http.StatusInternalServerError)).Once()

		resp := doRequest(t, p, http.MethodGet, "/user_attributes/status", userID, nil)
		requireErrorResponse(t, resp, http.StatusInternalServerError, "failed to access storage")
	})
}

// TestHandleDeleteUserAttributes checks that deleting removes both KV keys, so the provider is
// left with neither data nor a timestamp pointing at data that is gone.
func TestHandleDeleteUserAttributes(t *testing.T) {
	t.Run("deletes the stored file", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		// KV.Delete is a Set of a nil value under the hood.
		api.On("KVSetWithOptions", sync.UserAttrsStoreKey, []byte(nil), model.PluginKVSetOptions{}).
			Return(true, nil).Once()
		api.On("KVSetWithOptions", sync.UserAttrsLastUpdatedKey, []byte(nil), model.PluginKVSetOptions{}).
			Return(true, nil).Once()

		resp := doRequest(t, p, http.MethodDelete, "/user_attributes", userID, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("reports a failed delete", func(t *testing.T) {
		p, api := newTestPlugin(t)

		userID := model.NewId()
		asSysadmin(api, userID)
		api.On("KVSetWithOptions", sync.UserAttrsStoreKey, []byte(nil), model.PluginKVSetOptions{}).
			Return(false, model.NewAppError("KVSetWithOptions", "kv.set.app_error", nil, "connection refused", http.StatusInternalServerError)).Once()

		resp := doRequest(t, p, http.MethodDelete, "/user_attributes", userID, nil)
		requireErrorResponse(t, resp, http.StatusForbidden, "failed to delete file")
	})
}

// readAll drains a response body and closes it.
func readAll(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
