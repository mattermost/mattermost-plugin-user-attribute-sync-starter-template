package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/user-attribute-sync-starter-template/server/sync"
)

const (
	// maxFileSizeBytes caps the attributes file an admin can upload. The webapp enforces the
	// same limit client-side (MAX_FILE_BYES in upload_user_attributes.tsx) so the user gets an
	// immediate error instead of a failed request; the two are independent and must be kept in step.
	maxFileSizeBytes = 10 * 1024 * 1024
)

// ServeHTTP is the entry point for every request the Mattermost server proxies to this plugin,
// under /plugins/<plugin id>/. It delegates to the router built in initializeAPI.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

// initializeAPI builds the plugin's HTTP router. It must be called before any request can be
// served, so OnActivate calls it first.
//
// These endpoints let an admin manage the attributes file directly through the System Console
// rather than installing a file on the server's filesystem, which is what KVStoreProvider reads.
// They are only useful with that provider selected; FileProvider ignores the KV store entirely.
//
// Every route is behind requireSysadmin, applied once here as middleware rather than repeated in
// each handler, so a route added later cannot accidentally be left unauthenticated. Note this
// makes the whole router admin-only: a genuinely public route (a webhook, an OAuth callback)
// cannot simply be added below — if desired, move the protected routes onto a subrouter instead
// and apply Use there.
func (p *Plugin) initializeAPI() {
	router := mux.NewRouter()
	router.Use(p.requireSysadmin)

	router.HandleFunc("/user_attributes", p.handleUploadUserAttributes).Methods("POST")
	router.HandleFunc("/user_attributes", p.handleDownloadUserAttributes).Methods("GET")
	router.HandleFunc("/user_attributes/status", p.handleUserAttributesStatus).Methods("GET")
	router.HandleFunc("/user_attributes", p.handleDeleteUserAttributes).Methods("DELETE")

	p.router = router
}

type userAttributesStatus struct {
	Exists      bool       `json:"exists"`
	LastUpdated *time.Time `json:"lastUpdated"`
}

// handleUploadUserAttributes stores an uploaded attributes file in the KV store, where
// KVStoreProvider will find it on the next sync.
func (p *Plugin) handleUploadUserAttributes(w http.ResponseWriter, r *http.Request) {
	// Cap the body before reading it, so an oversized upload cannot exhaust memory
	r.Body = http.MaxBytesReader(w, r.Body, maxFileSizeBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			p.errorWithJSON(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("file exceeds %d byte limit", maxErr.Limit))
			return
		}
		p.errorWithJSON(w, http.StatusBadRequest, "could not read request body")
		return
	}

	// Validate the shape only: the payload has to be an array of objects, matching what
	// AttributeProvider returns.
	//
	// Individual records are deliberately not checked here. Rejecting an entire file because one
	// record is bad is the wrong trade-off — data pulled from an external system routinely has a
	// few unusable records, and refusing all of it means syncing nothing. Value sync already
	// handles them one at a time: unknown fields, unsupported types, unmatched emails and failed
	// writes each log a warning and move on to the next record.
	var userAttrs []map[string]interface{}
	if err = json.Unmarshal(raw, &userAttrs); err != nil || userAttrs == nil || len(userAttrs) == 0 {
		p.errorWithJSON(w, http.StatusBadRequest, "invalid json - must be array of objects")
		return
	}

	// Store the raw bytes rather than the decoded value, so a download returns exactly what was
	// uploaded. The timestamp is stored together with the file to ensure they are in sync.
	// Note KV.Set reports failure two ways: an error, or set == false meaning the write did not
	// happen.
	uploadedAt := time.Now()
	set, err := p.client.KV.Set(sync.UserAttrsStoreKey, sync.StoredUserAttrs{LastUpdated: uploadedAt, Data: raw})
	if err != nil {
		p.client.Log.Error("failed to upload userAttrs file", "err", err)
		p.errorWithJSON(w, http.StatusInternalServerError, "failed to upload file")
		return
	} else if !set {
		p.errorWithJSON(w, http.StatusInternalServerError, "failed to upload file, please try again")
		return
	}

	// Acknowledge with the resulting state, in the same shape the status endpoint uses, so the
	// webapp can show the new timestamp without asking for it again.
	p.responseWithJSON(w, http.StatusCreated, userAttributesStatus{Exists: true, LastUpdated: &uploadedAt})
}

// handleDownloadUserAttributes returns the stored attributes file verbatim, so an admin can see
// exactly what the plugin is syncing. This is the one handler that does not respond with the JSON
// envelope, because the body is the file itself.
func (p *Plugin) handleDownloadUserAttributes(w http.ResponseWriter, r *http.Request) {
	stored, err := sync.ReadStoredUserAttrs(p.client)
	if err != nil {
		p.client.Log.Error("failed to retrieve userAttrs", "err", err)
		p.errorWithJSON(w, http.StatusInternalServerError, "failed to download file")
		return
	}

	if len(stored.Data) == 0 {
		p.errorWithJSON(w, http.StatusNotFound, "file not found")
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(stored.Data); err != nil {
		p.API.LogError("Failed to write file in response", "err", err.Error())
		p.errorWithJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
}

// handleUserAttributesStatus reports whether a file is currently stored and when it was uploaded.
// The settings UI calls this on load to decide whether to offer Download and Delete, and to show
// how current the stored data is — neither of which it could get from the download endpoint
// without pulling the whole file down.
func (p *Plugin) handleUserAttributesStatus(w http.ResponseWriter, r *http.Request) {
	status, err := p.readUserAttributesStatus()
	if err != nil {
		p.client.Log.Error("failed to read user attributes status", "err", err)
		p.errorWithJSON(w, http.StatusInternalServerError, "failed to access storage")
		return
	}

	p.responseWithJSON(w, http.StatusOK, status)
}

// handleDeleteUserAttributes removes the stored file.
//
// Attribute values already written to user profiles are not affected — deleting the source does not
// retract what has already been synced. Note this leaves KVStoreProvider with nothing to read,
// which it reports as an error on every subsequent sync until a replacement is uploaded.
func (p *Plugin) handleDeleteUserAttributes(w http.ResponseWriter, r *http.Request) {
	if err := p.client.KV.Delete(sync.UserAttrsStoreKey); err != nil {
		p.errorWithJSON(w, http.StatusForbidden, "failed to delete file")
		return
	}

	w.WriteHeader(http.StatusOK)
}

// readUserAttributesStatus reports what is stored.
func (p *Plugin) readUserAttributesStatus() (userAttributesStatus, error) {
	stored, err := sync.ReadStoredUserAttrs(p.client)
	if err != nil {
		return userAttributesStatus{}, err
	}

	status := userAttributesStatus{Exists: len(stored.Data) > 0}
	if !stored.LastUpdated.IsZero() {
		status.LastUpdated = &stored.LastUpdated
	}

	return status, nil
}

// requireSysadmin is middleware that restricts every route on the router to system admins,
// responding itself and not calling through when the request should be refused.
//
// These endpoints read and overwrite the data that feeds every user's attributes, and attributes
// can gate channel access through ABAC policies, so system-admin is the right bar rather than
// merely "logged in".
//
// Note gorilla/mux only runs middleware once a route matches, so requests to unknown paths are
// answered with 404 without reaching this check.
func (p *Plugin) requireSysadmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This is the only header that is trusted; it is set by the Mattermost server on the way in.
		userID := r.Header.Get("Mattermost-User-Id")
		if userID == "" {
			p.errorWithJSON(w, http.StatusUnauthorized, "not logged in")
			return
		}

		// Access is locked down to a sysadmin
		if !p.client.User.HasPermissionTo(userID, model.PermissionManageSystem) {
			p.errorWithJSON(w, http.StatusForbidden, "not authorized")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (p *Plugin) errorWithJSON(w http.ResponseWriter, responseCode int, errMessage string) {
	responseBody := map[string]any{
		"error": errMessage,
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(responseCode)
	responseJSON, _ := json.Marshal(responseBody)
	if _, err := w.Write(responseJSON); err != nil {
		p.API.LogError("Failed to write error response", "err", err.Error(), "body", responseJSON)
	}
}

func (p *Plugin) responseWithJSON(w http.ResponseWriter, responseCode int, responseBody any) {
	responseJSON, err := json.Marshal(responseBody)
	if err != nil {
		p.client.Log.Error("Failed to write response", "err", err.Error())
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(responseCode)
	if _, err := w.Write(responseJSON); err != nil {
		p.API.LogError("Failed to write response", "err", err.Error(), "body", responseJSON)
	}
}
