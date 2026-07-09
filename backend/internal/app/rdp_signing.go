package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

type RDPSigningConfigView struct {
	Enabled               bool       `json:"enabled"`
	CertificateConfigured bool       `json:"certificateConfigured"`
	Subject               string     `json:"subject"`
	ThumbprintSHA256      string     `json:"thumbprintSha256"`
	GeneratedAt           *time.Time `json:"generatedAt,omitempty"`
}

type RDPSigningRuntimeConfig struct {
	Enabled               bool
	CertificateConfigured bool
	Subject               string
	ThumbprintSHA256      string
	PFXBase64             string
	PFXPassword           string
	LeafCertBase64        string
	RootCertBase64        string
}

type RDPSigningPublicConfig struct {
	Enabled               bool   `json:"enabled"`
	CertificateConfigured bool   `json:"certificateConfigured"`
	Subject               string `json:"subject"`
	ThumbprintSHA256      string `json:"thumbprintSha256"`
	LeafCertBase64        string `json:"leafCertBase64"`
	RootCertBase64        string `json:"rootCertBase64"`
}

func (s *AdminConfigStore) GetRDPSigningRuntime(ctx context.Context) (RDPSigningRuntimeConfig, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return RDPSigningRuntimeConfig{}, err
	}
	return rdpSigningRuntimeFromSettings(settings), nil
}

func (s *AdminConfigStore) GetRDPSigningPublic(ctx context.Context) (RDPSigningPublicConfig, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return RDPSigningPublicConfig{}, err
	}
	config := rdpSigningRuntimeFromSettings(settings)
	return RDPSigningPublicConfig{
		Enabled:               config.Enabled,
		CertificateConfigured: config.CertificateConfigured,
		Subject:               config.Subject,
		ThumbprintSHA256:      config.ThumbprintSHA256,
		LeafCertBase64:        config.LeafCertBase64,
		RootCertBase64:        config.RootCertBase64,
	}, nil
}

func (s *AdminConfigStore) GenerateTestRDPSigningCertificate(ctx context.Context) (any, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	if existing := rdpSigningRuntimeFromSettings(settings); existing.CertificateConfigured && isManagedTestRDPSigningSubject(existing.Subject) {
		return rdpSigningViewFromSettings(settings), nil
	}

	packageData, err := generateTestRDPSigningPackage()
	if err != nil {
		return nil, err
	}

	items := map[string]string{
		"rdp_signing_enabled":           "true",
		"rdp_signing_subject":           packageData.Subject,
		"rdp_signing_thumbprint_sha256": packageData.ThumbprintSHA256,
		"rdp_signing_pfx_base64":        packageData.PFXBase64,
		"rdp_signing_pfx_password":      packageData.PFXPassword,
		"rdp_signing_leaf_cert_base64":  packageData.LeafCertBase64,
		"rdp_signing_root_cert_base64":  packageData.RootCertBase64,
		"rdp_signing_generated_at":      packageData.GeneratedAt.Format(time.RFC3339),
	}
	for key, value := range items {
		stored, err := s.encryptSetting(ctx, key, value)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, `
			insert into admin_settings (key, value, updated_at)
			values ($1, $2, now())
			on conflict (key) do update
			set value = excluded.value, updated_at = now()
		`, key, stored); err != nil {
			return nil, err
		}
	}

	settings, err = s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	return rdpSigningViewFromSettings(settings), nil
}

func (c RDPSigningRuntimeConfig) View() RDPSigningConfigView {
	return RDPSigningConfigView{
		Enabled:               c.Enabled,
		CertificateConfigured: c.CertificateConfigured,
		Subject:               c.Subject,
		ThumbprintSHA256:      c.ThumbprintSHA256,
		GeneratedAt:           nil,
	}
}

func rdpSigningRuntimeFromSettings(settings map[string]string) RDPSigningRuntimeConfig {
	config := RDPSigningRuntimeConfig{
		Enabled:          settings["rdp_signing_enabled"] == "true",
		Subject:          strings.TrimSpace(settings["rdp_signing_subject"]),
		ThumbprintSHA256: strings.ToUpper(strings.TrimSpace(settings["rdp_signing_thumbprint_sha256"])),
		PFXBase64:        strings.TrimSpace(settings["rdp_signing_pfx_base64"]),
		PFXPassword:      strings.TrimSpace(settings["rdp_signing_pfx_password"]),
		LeafCertBase64:   strings.TrimSpace(settings["rdp_signing_leaf_cert_base64"]),
		RootCertBase64:   strings.TrimSpace(settings["rdp_signing_root_cert_base64"]),
	}
	config.CertificateConfigured = config.Subject != "" &&
		config.ThumbprintSHA256 != "" &&
		config.PFXBase64 != "" &&
		config.PFXPassword != "" &&
		config.LeafCertBase64 != "" &&
		config.RootCertBase64 != ""
	return config
}

func rdpSigningViewFromSettings(settings map[string]string) RDPSigningConfigView {
	config := rdpSigningRuntimeFromSettings(settings)
	view := config.View()
	if raw := strings.TrimSpace(settings["rdp_signing_generated_at"]); raw != "" {
		if generatedAt, err := time.Parse(time.RFC3339, raw); err == nil {
			view.GeneratedAt = &generatedAt
		}
	}
	return view
}

type generatedRDPSigningPackage struct {
	Subject          string
	ThumbprintSHA256 string
	PFXBase64        string
	PFXPassword      string
	LeafCertBase64   string
	RootCertBase64   string
	GeneratedAt      time.Time
}

func generateTestRDPSigningPackage() (generatedRDPSigningPackage, error) {
	now := time.Now().UTC()
	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return generatedRDPSigningPackage{}, fmt.Errorf("generate root key: %w", err)
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return generatedRDPSigningPackage{}, fmt.Errorf("generate signing key: %w", err)
	}

	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject: pkix.Name{
			CommonName:   "Access Workspace Test RDP Root",
			Organization: []string{"Access Workspace"},
		},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.AddDate(3, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return generatedRDPSigningPackage{}, fmt.Errorf("create root certificate: %w", err)
	}
	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return generatedRDPSigningPackage{}, fmt.Errorf("parse root certificate: %w", err)
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(now.Add(time.Second).UnixNano()),
		Subject: pkix.Name{
			CommonName:   "Access Workspace Test RDP Publisher",
			Organization: []string{"Access Workspace"},
		},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, rootCert, &leafKey.PublicKey, rootKey)
	if err != nil {
		return generatedRDPSigningPackage{}, fmt.Errorf("create signing certificate: %w", err)
	}
	leafCert, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return generatedRDPSigningPackage{}, fmt.Errorf("parse signing certificate: %w", err)
	}

	password := randomPassword(24)
	pfxBytes, err := pkcs12.Modern2023.Encode(leafKey, leafCert, []*x509.Certificate{rootCert}, password)
	if err != nil {
		return generatedRDPSigningPackage{}, fmt.Errorf("encode signing package: %w", err)
	}
	sum := sha1.Sum(leafDER)
	thumbprint := strings.ToUpper(hex.EncodeToString(sum[:]))

	return generatedRDPSigningPackage{
		Subject:          leafCert.Subject.String(),
		ThumbprintSHA256: thumbprint,
		PFXBase64:        base64.StdEncoding.EncodeToString(pfxBytes),
		PFXPassword:      password,
		LeafCertBase64:   base64.StdEncoding.EncodeToString(leafDER),
		RootCertBase64:   base64.StdEncoding.EncodeToString(rootDER),
		GeneratedAt:      now,
	}, nil
}

func isManagedTestRDPSigningSubject(subject string) bool {
	return strings.TrimSpace(subject) == "CN=Access Workspace Test RDP Publisher,O=Access Workspace"
}

func randomPassword(length int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	if length <= 0 {
		length = 24
	}
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("aw-%d", time.Now().UnixNano())
	}
	for i := range buffer {
		buffer[i] = alphabet[int(buffer[i])%len(alphabet)]
	}
	return string(buffer)
}
