//go:build windows

package launcher

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"access-workspace/launcher/internal/payload"
)

// runRDPPlatform is the Windows RDP flow: temporary Credential Manager
// handoff (cmdkey), a stable signed .rdp profile, and mstsc.
func runRDPPlatform(item payload.LaunchPayload, host string, port string, gatewayHost string) error {
	args := buildRDPArgs(host, port, item.Metadata)

	login := windowsConnectionIdentity(
		payload.MetadataString(item.Metadata, "connectionDomain"),
		payload.MetadataString(item.Metadata, "username"),
	)
	secret := payload.MetadataString(item.Metadata, "secretValue")
	cmdkeyTargets := []string{}
	if login != "" && secret != "" {
		cmdkeyTargets = buildRDPStoredCredentialTargets(host, port)
		for _, target := range cmdkeyTargets {
			cmdkey := exec.Command("cmdkey.exe", "/generic:"+target, "/user:"+login, "/pass:"+secret)
			hideWindow(cmdkey)
			if output, err := cmdkey.CombinedOutput(); err != nil {
				return fmt.Errorf("store rdp credentials: %w (%s)", err, strings.TrimSpace(string(output)))
			}
		}
		if gatewayHost != "" {
			// The RD Gateway is a second authenticated hop and its credential
			// lookup does NOT use the generic TERMSRV/* entries that satisfy the
			// target hop — it reads a DOMAIN-type credential for the bare gateway
			// hostname (exactly what Windows stores when "remember me" is ticked
			// on the gateway prompt). cmdkey /add: creates that domain-type entry;
			// /generic: does not work here (verified: mstsc still prompted).
			gatewayCred := exec.Command("cmdkey.exe", "/add:"+gatewayHost, "/user:"+login, "/pass:"+secret)
			hideWindow(gatewayCred)
			if output, err := gatewayCred.CombinedOutput(); err != nil {
				return fmt.Errorf("store rdp gateway credentials: %w (%s)", err, strings.TrimSpace(string(output)))
			}
			cmdkeyTargets = append(cmdkeyTargets, gatewayHost)
		}
		Logf("rdp: stored credentials for %v", cmdkeyTargets)
	} else {
		Logf("rdp: no stored credentials (login present=%t, secret present=%t)", login != "", secret != "")
	}

	tempFile := ""
	if login != "" {
		var err error
		var signRequired bool
		tempFile, signRequired, err = writeRDPProfile(item, host, port, item.Metadata)
		if err != nil {
			return err
		}
		Logf("rdp: profile %s (signRequired=%t, signingEnabled=%t)", tempFile, signRequired, payload.MetadataBool(item.Metadata, "rdpSigningEnabled"))
		if err := ensureRDPSigningTrust(item.Metadata); err != nil {
			return err
		}
		if payload.MetadataBool(item.Metadata, "rdpSigningEnabled") {
			// Best-effort: ensure this machine trusts the deployment's RDP
			// publisher before mstsc opens the signed profile. Idempotent — it
			// skips (no prompt) when the thumbprint is already trusted, so it
			// only elevates the first time or when the cert rotates. If it fails
			// (e.g. elevation declined) the launch still proceeds; mstsc just
			// shows its untrusted-publisher warning.
			if err := SyncAgentPrerequisites(); err != nil {
				Logf("rdp: publisher trust sync failed (continuing): %v", err)
			}
		}
		if signRequired {
			if err := signRDPProfile(item.Metadata, tempFile); err != nil {
				return err
			}
			Logf("rdp: profile signed")
		}
		args = []string{tempFile}
	}

	command := exec.Command("mstsc.exe", args...)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start mstsc: %w", err)
	}
	Logf("rdp: mstsc started (pid=%d, args=%v)", command.Process.Pid, args)
	go focusLaunchedWindow(command.Process.Pid)
	if len(cmdkeyTargets) > 0 {
		go clearRDPCredentialsLater(cmdkeyTargets)
	}
	// mstsc exiting within the watch window means no connection window ever
	// appeared (rejected signature, corrupt profile, ...). Without this check
	// the web app reports a successful hand-off while nothing happens.
	if err := waitForEarlyRDPExit(command, 2500*time.Millisecond); err != nil {
		return err
	}
	return nil
}
