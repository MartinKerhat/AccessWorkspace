package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/audit"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/tools"
)

// handleGenerateCertificate backs the Generator page's certificate entry:
// a self-signed certificate the browser cannot build on its own (PKCS#12
// export). Nothing is stored; the response is the only copy. The audit
// event records the subject and profile, never key material.
func (s *Server) handleGenerateCertificate(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !auth.CapabilitiesForUser(user).Generator.Certificates {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var input tools.CertificateInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	result, err := tools.GenerateCertificate(input)
	if err != nil {
		if errors.Is(err, tools.ErrInvalidInput) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeError(w, err)
		return
	}
	_ = s.audit.Log(r.Context(), audit.LogParams{
		EventType: audit.EventToolCertificateGenerated,
		UserID:    user.ID,
		UserName:  user.Name,
		Metadata: map[string]any{
			"subject":      result.Subject,
			"profile":      result.Profile,
			"keyAlgorithm": result.KeyAlgorithm,
			"notAfter":     result.NotAfter,
		},
	})
	writeJSON(w, http.StatusOK, result)
}
