// Package tools holds server-side generators for the workspace "Generator"
// page: things a browser cannot produce well on its own. Nothing here is
// persisted — the caller receives the material once and the server forgets
// it.
package tools

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

var ErrInvalidInput = errors.New("invalid input")

// Certificate profiles decide key usage / extended key usage.
const (
	ProfileDocumentSigning = "document_signing"
	ProfileCodeSigning     = "code_signing"
	ProfileTLSServer       = "tls_server"
	ProfileClientAuth      = "client_auth"
)

// Key algorithms.
const (
	KeyRSA2048 = "rsa2048"
	KeyRSA4096 = "rsa4096"
	KeyECP256  = "ec_p256"
)

// PFX encodings. Modern (AES-256 / SHA-256, PBKDF2) is what current Windows
// and OpenSSL 3 expect; legacy (RC2 / 3DES / SHA-1) is for older software
// that cannot open modern PKCS#12 files.
const (
	PFXModern = "modern"
	PFXLegacy = "legacy"
)

// adobePDFSigningEKU is the Adobe "PDF signing" extended key usage
// (1.2.840.113583.1.1.5) that document-signing certificates commonly carry
// alongside e-mail protection.
var adobePDFSigningEKU = asn1.ObjectIdentifier{1, 2, 840, 113583, 1, 1, 5}

type CertificateInput struct {
	CommonName   string `json:"commonName"`
	Organization string `json:"organization"`
	// OrganizationalUnit (department / team) and Country (ISO 3166-1 alpha-2)
	// are optional subject attributes; signing software shows them next to
	// the signer's name.
	OrganizationalUnit string   `json:"organizationalUnit"`
	Country            string   `json:"country"`
	Email              string   `json:"email"`
	ValidityDays       int      `json:"validityDays"`
	KeyAlgorithm       string   `json:"keyAlgorithm"`
	Profile            string   `json:"profile"`
	DNSNames           []string `json:"dnsNames"`
	PFXPassword        string   `json:"pfxPassword"`
	PFXEncoding        string   `json:"pfxEncoding"`
}

type CertificateResult struct {
	Subject          string    `json:"subject"`
	Profile          string    `json:"profile"`
	KeyAlgorithm     string    `json:"keyAlgorithm"`
	NotBefore        time.Time `json:"notBefore"`
	NotAfter         time.Time `json:"notAfter"`
	SerialNumber     string    `json:"serialNumber"`
	ThumbprintSHA1   string    `json:"thumbprintSha1"`
	ThumbprintSHA256 string    `json:"thumbprintSha256"`
	// Human-readable intended purposes, as Windows shows them in the
	// certificate dialog — what the chosen profile actually wrote into the
	// certificate.
	KeyUsages        []string `json:"keyUsages"`
	ExtendedKeyUsage []string `json:"extendedKeyUsages"`
	CertificatePEM   string   `json:"certificatePem"`
	PrivateKeyPEM    string   `json:"privateKeyPem"`
	PFXBase64        string   `json:"pfxBase64"`
	PFXFileName      string   `json:"pfxFileName"`
}

// GenerateCertificate creates a self-signed X.509 certificate for the given
// profile and returns it as PEM (certificate + PKCS#8 private key) and as a
// password-protected PKCS#12 bundle.
func GenerateCertificate(input CertificateInput) (CertificateResult, error) {
	commonName := strings.TrimSpace(input.CommonName)
	if commonName == "" {
		return CertificateResult{}, fmt.Errorf("%w: common name is required", ErrInvalidInput)
	}
	if len(commonName) > 64 {
		return CertificateResult{}, fmt.Errorf("%w: common name must be at most 64 characters", ErrInvalidInput)
	}
	if input.ValidityDays < 1 || input.ValidityDays > 3650 {
		return CertificateResult{}, fmt.Errorf("%w: validity must be between 1 and 3650 days", ErrInvalidInput)
	}
	if country := strings.TrimSpace(input.Country); country != "" {
		if len(country) != 2 || strings.ContainsFunc(country, func(r rune) bool { return r < 'A' || r > 'Z' }) {
			return CertificateResult{}, fmt.Errorf("%w: country must be a two-letter upper-case code such as CZ", ErrInvalidInput)
		}
	}
	// A document-signing certificate identifies a person; signing software
	// shows the e-mail next to the name and some refuse a certificate without.
	if strings.TrimSpace(input.Profile) == ProfileDocumentSigning && strings.TrimSpace(input.Email) == "" {
		return CertificateResult{}, fmt.Errorf("%w: document signing needs the signer's e-mail address", ErrInvalidInput)
	}
	if len(input.PFXPassword) < 8 {
		return CertificateResult{}, fmt.Errorf("%w: PFX password must be at least 8 characters", ErrInvalidInput)
	}
	encoding := strings.TrimSpace(input.PFXEncoding)
	if encoding == "" {
		encoding = PFXModern
	}
	if encoding != PFXModern && encoding != PFXLegacy {
		return CertificateResult{}, fmt.Errorf("%w: unknown PFX encoding %q", ErrInvalidInput, input.PFXEncoding)
	}

	privateKey, err := generateKey(input.KeyAlgorithm)
	if err != nil {
		return CertificateResult{}, err
	}

	template, err := certificateTemplate(input, commonName)
	if err != nil {
		return CertificateResult{}, err
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, publicKeyOf(privateKey), privateKey)
	if err != nil {
		return CertificateResult{}, fmt.Errorf("create certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return CertificateResult{}, fmt.Errorf("parse certificate: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return CertificateResult{}, fmt.Errorf("encode private key: %w", err)
	}

	encoder := pkcs12.Modern2023
	if encoding == PFXLegacy {
		encoder = pkcs12.Legacy
	}
	pfxBytes, err := encoder.Encode(privateKey, cert, nil, input.PFXPassword)
	if err != nil {
		return CertificateResult{}, fmt.Errorf("encode PFX: %w", err)
	}

	sha1Sum := sha1.Sum(certDER)
	sha256Sum := sha256.Sum256(certDER)

	return CertificateResult{
		Subject:          cert.Subject.String(),
		Profile:          input.Profile,
		KeyAlgorithm:     input.KeyAlgorithm,
		NotBefore:        cert.NotBefore,
		NotAfter:         cert.NotAfter,
		SerialNumber:     strings.ToUpper(cert.SerialNumber.Text(16)),
		ThumbprintSHA1:   strings.ToUpper(hex.EncodeToString(sha1Sum[:])),
		ThumbprintSHA256: strings.ToUpper(hex.EncodeToString(sha256Sum[:])),
		KeyUsages:        keyUsageNames(cert),
		ExtendedKeyUsage: extendedKeyUsageNames(cert),
		CertificatePEM:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})),
		PrivateKeyPEM:    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})),
		PFXBase64:        base64.StdEncoding.EncodeToString(pfxBytes),
		PFXFileName:      pfxFileName(commonName),
	}, nil
}

func generateKey(algorithm string) (crypto.Signer, error) {
	switch strings.TrimSpace(algorithm) {
	case KeyRSA2048:
		return rsa.GenerateKey(rand.Reader, 2048)
	case KeyRSA4096:
		return rsa.GenerateKey(rand.Reader, 4096)
	case KeyECP256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	default:
		return nil, fmt.Errorf("%w: unknown key algorithm %q", ErrInvalidInput, algorithm)
	}
}

func publicKeyOf(key crypto.Signer) crypto.PublicKey {
	return key.Public()
}

func certificateTemplate(input CertificateInput, commonName string) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, fmt.Errorf("serial number: %w", err)
	}
	now := time.Now().UTC()
	subject := pkix.Name{CommonName: commonName}
	if organization := strings.TrimSpace(input.Organization); organization != "" {
		subject.Organization = []string{organization}
	}
	if unit := strings.TrimSpace(input.OrganizationalUnit); unit != "" {
		subject.OrganizationalUnit = []string{unit}
	}
	if country := strings.TrimSpace(input.Country); country != "" {
		subject.Country = []string{country}
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               subject,
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(0, 0, input.ValidityDays),
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	if email := strings.TrimSpace(input.Email); email != "" {
		template.EmailAddresses = []string{email}
	}

	switch strings.TrimSpace(input.Profile) {
	case ProfileDocumentSigning:
		template.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection}
		template.UnknownExtKeyUsage = []asn1.ObjectIdentifier{adobePDFSigningEKU}
	case ProfileCodeSigning:
		template.KeyUsage = x509.KeyUsageDigitalSignature
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}
	case ProfileTLSServer:
		template.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		names := uniqueNames(append([]string{commonName}, input.DNSNames...))
		for _, name := range names {
			if ip := net.ParseIP(name); ip != nil {
				template.IPAddresses = append(template.IPAddresses, ip)
			} else {
				template.DNSNames = append(template.DNSNames, name)
			}
		}
	case ProfileClientAuth:
		template.KeyUsage = x509.KeyUsageDigitalSignature
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	default:
		return nil, fmt.Errorf("%w: unknown certificate profile %q", ErrInvalidInput, input.Profile)
	}
	return template, nil
}

func keyUsageNames(cert *x509.Certificate) []string {
	var names []string
	if cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		names = append(names, "Digital signature")
	}
	if cert.KeyUsage&x509.KeyUsageContentCommitment != 0 {
		names = append(names, "Non-repudiation")
	}
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
		names = append(names, "Key encipherment")
	}
	return names
}

func extendedKeyUsageNames(cert *x509.Certificate) []string {
	var names []string
	for _, eku := range cert.ExtKeyUsage {
		switch eku {
		case x509.ExtKeyUsageEmailProtection:
			names = append(names, "Secure e-mail")
		case x509.ExtKeyUsageCodeSigning:
			names = append(names, "Code signing")
		case x509.ExtKeyUsageServerAuth:
			names = append(names, "Server authentication")
		case x509.ExtKeyUsageClientAuth:
			names = append(names, "Client authentication")
		}
	}
	for _, oid := range cert.UnknownExtKeyUsage {
		if oid.Equal(adobePDFSigningEKU) {
			names = append(names, "Document signing (Adobe PDF)")
		}
	}
	return names
}

func uniqueNames(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func pfxFileName(commonName string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(commonName) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '.', r == '-', r == '_':
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "certificate"
	}
	return name + ".pfx"
}
