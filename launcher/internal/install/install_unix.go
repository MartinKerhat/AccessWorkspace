//go:build linux || darwin

package install

import (
	"fmt"
	"net/http"
	"time"
)

type RDPMachineTrustPackage struct {
	Thumbprint     string `json:"thumbprint"`
	LeafCertBase64 string `json:"leafCertBase64"`
	RootCertBase64 string `json:"rootCertBase64"`
}

func waitForBackgroundLauncher() error {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		response, err := client.Get("http://127.0.0.1:47654/status")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("unexpected launcher status code %d", response.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(400 * time.Millisecond)
	}
	return fmt.Errorf("background launcher did not become ready: %w", lastErr)
}

// RDP publisher trust is an mstsc concept; FreeRDP launches need none of it.
func WriteMachineTrustPackage(pkg RDPMachineTrustPackage) (string, error) {
	return "", fmt.Errorf("machine trust packages are only used on Windows")
}

func InstallMachineRDPPublisherTrustFromFile(path string) error {
	return fmt.Errorf("machine trust installation is only used on Windows")
}

func EnsureMachineRDPPublisherTrust(pkg RDPMachineTrustPackage) error {
	// No-op by design: FreeRDP (Linux, macOS) has no profile-signing trust model.
	return nil
}
