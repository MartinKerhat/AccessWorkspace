package launcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"access-workspace/launcher/internal/install"
	"access-workspace/launcher/internal/launcherinfo"
)

type rdpSigningPublicConfig struct {
	Enabled               bool   `json:"enabled"`
	CertificateConfigured bool   `json:"certificateConfigured"`
	Subject               string `json:"subject"`
	ThumbprintSHA256      string `json:"thumbprintSha256"`
	LeafCertBase64        string `json:"leafCertBase64"`
	RootCertBase64        string `json:"rootCertBase64"`
}

// SyncAgentPrerequisites ensures deployment-wide RDP publisher trust is present.
// It targets the workspace the launcher last handled a launch from (learned at
// runtime, persisted by rememberWorkspaceBaseURL). If no workspace is known yet
// — e.g. right after install, before any launch — it is a no-op: trust is
// ensured lazily on the first RDP launch, once the deployment URL is known.
func SyncAgentPrerequisites() error {
	base := loadWorkspaceBaseURL()
	if base == "" {
		return nil
	}
	config, err := fetchRDPSigningPublicConfig(base)
	if err != nil {
		return err
	}
	if !config.Enabled || !config.CertificateConfigured {
		return nil
	}
	return installRDPPublisherTrustPackage(config.ThumbprintSHA256, config.LeafCertBase64, config.RootCertBase64)
}

func installRDPPublisherTrustPackage(thumbprint string, leafCertBase64 string, rootCertBase64 string) error {
	if configured, _, err := trustedRDPPublisherThumbprintPresentAtPath(`HKLM:\SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services`, thumbprint); err == nil && configured {
		return nil
	}

	return install.EnsureMachineRDPPublisherTrust(install.RDPMachineTrustPackage{
		Thumbprint:     strings.ToUpper(strings.TrimSpace(thumbprint)),
		LeafCertBase64: leafCertBase64,
		RootCertBase64: rootCertBase64,
	})
}

func fetchRDPSigningPublicConfig(workspaceBaseURL string) (rdpSigningPublicConfig, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Get(workspaceBaseURL + launcherinfo.RDPTrustPath)
	if err != nil {
		return rdpSigningPublicConfig{}, fmt.Errorf("fetch RDP publisher trust configuration: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return rdpSigningPublicConfig{}, fmt.Errorf("fetch RDP publisher trust configuration: unexpected status %d", response.StatusCode)
	}
	var config rdpSigningPublicConfig
	if err := json.NewDecoder(response.Body).Decode(&config); err != nil {
		return rdpSigningPublicConfig{}, fmt.Errorf("decode RDP publisher trust configuration: %w", err)
	}
	config.ThumbprintSHA256 = strings.ToUpper(strings.TrimSpace(config.ThumbprintSHA256))
	config.LeafCertBase64 = strings.TrimSpace(config.LeafCertBase64)
	config.RootCertBase64 = strings.TrimSpace(config.RootCertBase64)
	return config, nil
}
