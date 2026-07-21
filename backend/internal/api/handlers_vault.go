package api

import (
	"encoding/json"
	"net/http"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
)

func (s *Server) handleVaultStatus(w http.ResponseWriter, r *http.Request, user auth.User) {
	status, err := s.authenticator.GetVaultStatus(r.Context(), user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleVaultSetup(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.authenticator.SetupVault(r.Context(), user, requestSessionToken(r), input.Passphrase); err != nil {
		writeError(w, err)
		return
	}
	s.auditVault(r, user, audit.EventVaultSetup, "passphrase")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVaultUnlock(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.authenticator.UnlockVault(r.Context(), user, requestSessionToken(r), input.Passphrase); err != nil {
		writeError(w, err)
		return
	}
	s.auditVault(r, user, audit.EventVaultUnlocked, "passphrase")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// auditVault records a personal-vault operation with the unlock method used.
func (s *Server) auditVault(r *http.Request, user auth.User, event audit.EventType, method string) {
	_ = s.audit.Log(r.Context(), audit.LogParams{
		EventType: event,
		UserID:    user.ID,
		UserName:  user.Name,
		Metadata:  map[string]any{"method": method},
	})
}

func (s *Server) handleVaultAddPassphrase(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.authenticator.AddVaultPassphrase(r.Context(), user, input.Passphrase); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVaultPasskeySetup(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input vaultPasskeyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.authenticator.SetupVaultWithPasskey(r.Context(), user, requestSessionToken(r), input.CredentialID, input.PRFSalt, input.PRFSecret); err != nil {
		writeError(w, err)
		return
	}
	s.auditVault(r, user, audit.EventVaultSetup, "passkey")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVaultPasskeyUnlock(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input vaultPasskeyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.authenticator.UnlockVaultWithPasskey(r.Context(), user, requestSessionToken(r), input.CredentialID, input.PRFSecret); err != nil {
		writeError(w, err)
		return
	}
	s.auditVault(r, user, audit.EventVaultUnlocked, "passkey")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVaultPasskeyAdd(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input vaultPasskeyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.authenticator.AddVaultPasskey(r.Context(), user, input.CredentialID, input.PRFSalt, input.PRFSecret); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
