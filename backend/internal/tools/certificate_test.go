package tools

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

func baseInput() CertificateInput {
	return CertificateInput{
		CommonName:   "Example Document Signer",
		Organization: "Example Org",
		Email:        "signer@example.internal",
		ValidityDays: 365,
		KeyAlgorithm: KeyRSA2048,
		Profile:      ProfileDocumentSigning,
		PFXPassword:  "pfx-password-1",
	}
}

func parseCert(t *testing.T, pemText string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(pemText))
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("certificate PEM not decodable")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}

func TestGenerateCertificateDocumentSigning(t *testing.T) {
	result, err := GenerateCertificate(baseInput())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	cert := parseCert(t, result.CertificatePEM)
	if cert.Subject.CommonName != "Example Document Signer" || cert.Subject.Organization[0] != "Example Org" {
		t.Fatalf("unexpected subject %s", cert.Subject)
	}
	if cert.IsCA {
		t.Fatal("must not be a CA")
	}
	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 || cert.KeyUsage&x509.KeyUsageContentCommitment == 0 {
		t.Fatalf("document signing key usage wrong: %v", cert.KeyUsage)
	}
	if len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageEmailProtection {
		t.Fatalf("expected email protection EKU, got %v", cert.ExtKeyUsage)
	}
	if len(cert.UnknownExtKeyUsage) != 1 || !cert.UnknownExtKeyUsage[0].Equal(adobePDFSigningEKU) {
		t.Fatalf("expected Adobe PDF signing EKU, got %v", cert.UnknownExtKeyUsage)
	}
	if len(cert.EmailAddresses) != 1 || cert.EmailAddresses[0] != "signer@example.internal" {
		t.Fatalf("email SAN missing: %v", cert.EmailAddresses)
	}
	if until := time.Until(cert.NotAfter); until < 364*24*time.Hour || until > 366*24*time.Hour {
		t.Fatalf("unexpected validity: %v", cert.NotAfter)
	}
	// Self-signed: the signature verifies with the certificate's own key.
	// (CheckSignatureFrom is not usable here — it requires a CA parent, and an
	// end-entity certificate is deliberately not a CA.)
	if err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
		t.Fatalf("self-signature check: %v", err)
	}

	// Private key PEM is PKCS#8 and matches the certificate.
	block, _ := pem.Decode([]byte(result.PrivateKeyPEM))
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatal("private key PEM not PKCS#8")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok || !rsaKey.PublicKey.Equal(cert.PublicKey) {
		t.Fatal("private key does not match certificate")
	}

	// PFX round-trips with the password.
	pfxBytes, err := base64.StdEncoding.DecodeString(result.PFXBase64)
	if err != nil {
		t.Fatalf("pfx base64: %v", err)
	}
	pfxKey, pfxCert, err := pkcs12.Decode(pfxBytes, "pfx-password-1")
	if err != nil {
		t.Fatalf("decode pfx: %v", err)
	}
	if !pfxCert.Equal(cert) {
		t.Fatal("pfx certificate differs")
	}
	if _, ok := pfxKey.(*rsa.PrivateKey); !ok {
		t.Fatal("pfx key not RSA")
	}
	if _, _, err := pkcs12.Decode(pfxBytes, "wrong"); err == nil {
		t.Fatal("pfx must not open with the wrong password")
	}
	if result.PFXFileName != "example-document-signer.pfx" {
		t.Fatalf("unexpected file name %q", result.PFXFileName)
	}
	if len(result.KeyUsages) != 2 || len(result.ExtendedKeyUsage) != 2 || result.ExtendedKeyUsage[1] != "Document signing (Adobe PDF)" {
		t.Fatalf("readable usages wrong: %v %v", result.KeyUsages, result.ExtendedKeyUsage)
	}
	if len(result.ThumbprintSHA1) != 40 || len(result.ThumbprintSHA256) != 64 {
		t.Fatalf("thumbprints wrong length: %q %q", result.ThumbprintSHA1, result.ThumbprintSHA256)
	}
}

func TestGenerateCertificateProfiles(t *testing.T) {
	cases := []struct {
		profile string
		eku     x509.ExtKeyUsage
	}{
		{ProfileCodeSigning, x509.ExtKeyUsageCodeSigning},
		{ProfileClientAuth, x509.ExtKeyUsageClientAuth},
	}
	for _, tc := range cases {
		input := baseInput()
		input.Profile = tc.profile
		input.Email = ""
		input.KeyAlgorithm = KeyECP256
		input.PFXEncoding = PFXLegacy
		result, err := GenerateCertificate(input)
		if err != nil {
			t.Fatalf("%s: %v", tc.profile, err)
		}
		cert := parseCert(t, result.CertificatePEM)
		if len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != tc.eku {
			t.Fatalf("%s: EKU %v", tc.profile, cert.ExtKeyUsage)
		}
		if _, ok := cert.PublicKey.(*ecdsa.PublicKey); !ok {
			t.Fatalf("%s: expected EC key", tc.profile)
		}
		pfxBytes, _ := base64.StdEncoding.DecodeString(result.PFXBase64)
		if _, _, err := pkcs12.Decode(pfxBytes, "pfx-password-1"); err != nil {
			t.Fatalf("%s: legacy pfx decode: %v", tc.profile, err)
		}
	}
}

func TestGenerateCertificateTLSServerSANs(t *testing.T) {
	input := baseInput()
	input.Profile = ProfileTLSServer
	input.CommonName = "app.example.internal"
	input.DNSNames = []string{"App.Example.Internal", "api.example.internal", "10.0.0.5", ""}
	result, err := GenerateCertificate(input)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	cert := parseCert(t, result.CertificatePEM)
	if len(cert.DNSNames) != 2 || cert.DNSNames[0] != "app.example.internal" || cert.DNSNames[1] != "api.example.internal" {
		t.Fatalf("DNS SANs wrong: %v", cert.DNSNames)
	}
	if len(cert.IPAddresses) != 1 || cert.IPAddresses[0].String() != "10.0.0.5" {
		t.Fatalf("IP SANs wrong: %v", cert.IPAddresses)
	}
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		t.Fatal("TLS server needs key encipherment for RSA")
	}
}

func TestGenerateCertificateSubjectAttributes(t *testing.T) {
	input := baseInput()
	input.OrganizationalUnit = "Finance"
	input.Country = "CZ"
	result, err := GenerateCertificate(input)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	cert := parseCert(t, result.CertificatePEM)
	if len(cert.Subject.OrganizationalUnit) != 1 || cert.Subject.OrganizationalUnit[0] != "Finance" {
		t.Fatalf("OU missing: %v", cert.Subject)
	}
	if len(cert.Subject.Country) != 1 || cert.Subject.Country[0] != "CZ" {
		t.Fatalf("country missing: %v", cert.Subject)
	}
}

func TestGenerateCertificateValidation(t *testing.T) {
	bad := []func(*CertificateInput){
		func(i *CertificateInput) { i.CommonName = "  " },
		func(i *CertificateInput) { i.Email = "" }, // document signing needs an e-mail
		func(i *CertificateInput) { i.Country = "Czechia" },
		func(i *CertificateInput) { i.Country = "cz" },
		func(i *CertificateInput) { i.ValidityDays = 0 },
		func(i *CertificateInput) { i.ValidityDays = 5000 },
		func(i *CertificateInput) { i.PFXPassword = "short" },
		func(i *CertificateInput) { i.KeyAlgorithm = "dsa" },
		func(i *CertificateInput) { i.Profile = "ca" },
		func(i *CertificateInput) { i.PFXEncoding = "pkcs7" },
	}
	for i, mutate := range bad {
		input := baseInput()
		mutate(&input)
		if _, err := GenerateCertificate(input); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("case %d: expected ErrInvalidInput, got %v", i, err)
		}
	}
}
