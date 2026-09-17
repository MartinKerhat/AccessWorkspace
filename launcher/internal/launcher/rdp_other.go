//go:build !windows && !linux

package launcher

import (
	"fmt"

	"github.com/MartinKerhat/AccessWorkspace/launcher/internal/payload"
)

func runRDPPlatform(item payload.LaunchPayload, host string, port string, gatewayHost string) error {
	return fmt.Errorf("rdp launch is not supported on this platform yet")
}
