package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	maxFileSizeBytes  = 10 * 1024 * 1024
	userAttrsStoreKey = "user-attrs-file"
)

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) initializeAPI() {
	router := mux.NewRouter()

	router.HandleFunc("/user_attributes", p.handleUploadUserAttributes).Methods("POST")
	router.HandleFunc("/user_attributes", p.handleDownloadUserAttributes).Methods("GET")

	p.router = router
}

func (p *Plugin) handleUploadUserAttributes(w http.ResponseWriter, r *http.Request) {
	// This is the only header that is trusted; it is set by the Mattermost server on the way in.
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		p.errorWithJSON(w, http.StatusUnauthorized, "log in")
		return
	}
	// Access is locked down to a sysadmin
	if !p.client.User.HasPermissionTo(userID, model.PermissionManageSystem) {
		p.errorWithJSON(w, http.StatusForbidden, "not authorized")
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
	set, err := p.client.KV.Set(userAttrsStoreKey, raw)
	if err != nil {
		p.client.Log.Error("failed to upload userAttrs file", "err", err)
		p.errorWithJSON(w, http.StatusInternalServerError, "failed to upload file")
		return
	} else if !set {
		p.errorWithJSON(w, http.StatusInternalServerError, "failed to upload file, please try again")
		return
	}

	// Return acknowledgement
	p.responseWithJSON(w, http.StatusCreated, "upload successful")

}

func (p *Plugin) handleDownloadUserAttributes(w http.ResponseWriter, r *http.Request) {
	// This is the only header that is trusted; it is set by the Mattermost server on the way in.
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		p.errorWithJSON(w, http.StatusUnauthorized, "log in")
		return
	}

	// Access is locked down to a sysadmin
	if !p.client.User.HasPermissionTo(userID, model.PermissionManageSystem) {
		p.errorWithJSON(w, http.StatusForbidden, "not authorized")
		return
	}

	var userAttrs []byte
	if err := p.client.KV.Get(userAttrsStoreKey, &userAttrs); err != nil {
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
