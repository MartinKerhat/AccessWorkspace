//go:build !windows && !linux

package install

import "fmt"

func InstallOrUpgrade() (string, error) {
	return "", fmt.Errorf("self-install preview is currently supported only on Windows")
}

func ShowInstallSuccess(installedPath string) {}

func ShowInstallFailure(err error) {}

type RDPMachineTrustPackage struct {
	Thumbprint     string `json:"thumbprint"`
	LeafCertBase64 string `json:"leafCertBase64"`
	RootCertBase64 string `json:"rootCertBase64"`
}

func WriteMachineTrustPackage(pkg RDPMachineTrustPackage) (string, error) {
	return "", fmt.Errorf("machine trust package is only supported on Windows")
}

func InstallMachineRDPPublisherTrustFromFile(path string) error {
	return fmt.Errorf("machine trust installation is only supported on Windows")
}

func EnsureMachineRDPPublisherTrust(pkg RDPMachineTrustPackage) error {
	return fmt.Errorf("machine trust installation is only supported on Windows")
}
