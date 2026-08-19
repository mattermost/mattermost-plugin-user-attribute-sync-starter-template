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
	maxFileSizeBytes = 10 * 1024 * 1024
)

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) initializeAPI() {
	router := mux.NewRouter()

	// These endpoints are to allow an admin to upload a file directly through the System Console
	// rather than manually installing a file on the filesystem
	router.HandleFunc("/user_attributes", p.handleUploadUserAttributes).Methods("POST")
	router.HandleFunc("/user_attributes", p.handleDownloadUserAttributes).Methods("GET")
	router.HandleFunc("/user_attributes/exists", p.handleUserAttributesExist).Methods("GET")
	router.HandleFunc("/user_attributes", p.handleDeleteUserAttributes).Methods("DELETE")

	p.router = router
}

func (p *Plugin) handleUploadUserAttributes(w http.ResponseWriter, r *http.Request) {

	if !p.checkSysAadmin(w, r) {
		return
	}

	// Put in size protections and read
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

	// Validate - we can also check for valid email and unrecognized fields - for a large file reject all or accept partial and warn?
	var userAttrs []map[string]interface{}
	if err := json.Unmarshal(raw, &userAttrs); err != nil {
		p.errorWithJSON(w, http.StatusBadRequest, "invalid json - must be array of objects")
		return
	}

	// Store in KV
	set, err := p.client.KV.Set(sync.UserAttrsStoreKey, raw)
	if err != nil {
		p.client.Log.Error("failed to upload userAttrs file", "err", err)
		p.errorWithJSON(w, http.StatusInternalServerError, "failed to upload file")
		return
	} else if !set {
		p.errorWithJSON(w, http.StatusInternalServerError, "failed to upload file, please try again")
		return
	}

	// Update last updated time.
	timeData, err := time.Now().MarshalJSON()
	if err != nil {
		p.client.Log.Error("failed to update timestamp to process file", "err", err)
	}

	// No returning early with an error after the file was uploaded, so the timestamp update is best effort.
	if err == nil {
		set, err := p.client.KV.Set(sync.UserAttrsLastUpdatedKey, timeData)
		if !set || err != nil {
			p.client.Log.Error("failed to udpate timestamp to process file", "err", err)
		}
	}

	// Return acknowledgement
	p.responseWithJSON(w, http.StatusCreated, "upload successful")

}

func (p *Plugin) handleDownloadUserAttributes(w http.ResponseWriter, r *http.Request) {
	if !p.checkSysAadmin(w, r) {
		return
	}

	var userAttrs []byte
	if err := p.client.KV.Get(sync.UserAttrsStoreKey, &userAttrs); err != nil {
		p.client.Log.Error("failed to retrieve userAttrs", "err", err)
		p.errorWithJSON(w, http.StatusInternalServerError, "failed to download file")
		return
	}

	if len(userAttrs) == 0 {
		p.errorWithJSON(w, http.StatusNotFound, "file not found")
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(userAttrs); err != nil {
		p.API.LogError("Failed to write file in response", "err", err.Error())
		p.errorWithJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
}

func (p *Plugin) handleUserAttributesExist(w http.ResponseWriter, r *http.Request) {
	if !p.checkSysAadmin(w, r) {
		return
	}

	var userAttrs []byte
	if err := p.client.KV.Get(sync.UserAttrsStoreKey, &userAttrs); err != nil {
		p.client.Log.Error("failed to retrieve userAttrs", "err", err)
		p.errorWithJSON(w, http.StatusInternalServerError, "failed to access storage")
		return
	}

	p.responseWithJSON(w, http.StatusOK, map[string]bool{"exists": len(userAttrs) > 0})
}

func (p *Plugin) handleDeleteUserAttributes(w http.ResponseWriter, r *http.Request) {
	if !p.checkSysAadmin(w, r) {
		return
	}

	if err := p.client.KV.Delete(sync.UserAttrsStoreKey); err != nil {
		p.errorWithJSON(w, http.StatusForbidden, "failed to delete file")
		return
	}

	if err := p.client.KV.Delete(sync.UserAttrsLastUpdatedKey); err != nil {
		// no early error return, timestamp deletion is best effort so we can still report the file was deleted.
		// just log an error instead.
		p.client.Log.Error("failed to delete file timestamp after file deletion: %w", err)
	}

	w.WriteHeader(http.StatusOK)
}

func (p *Plugin) checkSysAadmin(w http.ResponseWriter, r *http.Request) bool {
	// This is the only header that is trusted; it is set by the Mattermost server on the way in.
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		p.errorWithJSON(w, http.StatusUnauthorized, "not logged in")
		return false
	}

	// Access is locked down to a sysadmin
	if !p.client.User.HasPermissionTo(userID, model.PermissionManageSystem) {
		p.errorWithJSON(w, http.StatusForbidden, "not authorized")
		return false
	}

	return true
}

func (p *Plugin) errorWithJSON(w http.ResponseWriter, responseCode int, errMessage string) {
	w.WriteHeader(responseCode)
	responseBody := map[string]any{
		"error": errMessage,
	}
	w.Header().Add("Content-Type", "application/json")
	responseJSON, _ := json.Marshal(responseBody)
	if _, err := w.Write(responseJSON); err != nil {
		p.API.LogError("Failed to write error response", "err", err.Error(), "body", responseJSON)
	}
}

func (p *Plugin) responseWithJSON(w http.ResponseWriter, responseCode int, responseBody any) {
	w.WriteHeader(responseCode)
	w.Header().Add("Content-Type", "application/json")
	responseJSON, err := json.Marshal(responseBody)
	if err != nil {
		p.API.LogError("Failed to write response", "err", err.Error())
		p.errorWithJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := w.Write(responseJSON); err != nil {
		p.API.LogError("Failed to write response", "err", err.Error(), "body", responseJSON)
	}
}
