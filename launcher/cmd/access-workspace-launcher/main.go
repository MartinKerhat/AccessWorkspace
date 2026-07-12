package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"access-workspace/launcher/internal/install"
	"access-workspace/launcher/internal/launcher"
	"access-workspace/launcher/internal/launcherinfo"
	"access-workspace/launcher/internal/payload"
)

func main() {
	var rawURI string
	var encodedPayload string
	var sshSessionFile string
	var machineTrustPackage string
	var syncPrereqs bool
	var printOnly bool
	var installOnly bool
	var backgroundOnly bool

	flag.StringVar(&rawURI, "uri", "", "full access-workspace:// launch uri")
	flag.StringVar(&encodedPayload, "payload", "", "base64url encoded launch payload")
	flag.StringVar(&sshSessionFile, "ssh-session-file", "", "json payload file for a managed ssh session")
	flag.StringVar(&machineTrustPackage, "install-machine-rdp-trust", "", "install machine-level RDP publisher trust from a package file")
	flag.BoolVar(&syncPrereqs, "sync-agent-prereqs", false, "synchronize launcher prerequisites like RDP publisher trust")
	flag.BoolVar(&printOnly, "print", false, "decode and print the payload without launching")
	flag.BoolVar(&installOnly, "install", false, "install or upgrade the launcher and protocol registration")
	flag.BoolVar(&backgroundOnly, "background", false, "run the local launcher bridge")
	flag.Parse()

	if strings.TrimSpace(machineTrustPackage) != "" {
		if err := install.InstallMachineRDPPublisherTrustFromFile(machineTrustPackage); err != nil {
			exitWithError(err)
		}
		return
	}

	if syncPrereqs {
		if err := launcher.SyncAgentPrerequisites(); err != nil {
			exitWithError(err)
		}
		return
	}

	if backgroundOnly {
		if err := runBackgroundServer(); err != nil {
			exitWithError(err)
		}
		return
	}

	if strings.TrimSpace(sshSessionFile) != "" {
		item, err := payload.DecodePayloadFile(sshSessionFile)
		if err != nil {
			launcher.ShowLaunchFailure(err)
			exitWithError(err)
		}
		_ = os.Remove(sshSessionFile)
		if err := launcher.RunSSHSession(item); err != nil {
			launcher.ShowLaunchFailure(err)
			exitWithError(err)
		}
		return
	}

	if installOnly || shouldInstallByDefault(rawURI, encodedPayload, flag.Args()) {
		installedPath, err := install.InstallOrUpgrade()
		if err != nil {
			install.ShowInstallFailure(err)
			exitWithError(err)
		}
		install.ShowInstallSuccess(installedPath)
		return
	}

	item, err := resolvePayload(rawURI, encodedPayload, flag.Args())
	if err != nil {
		// The launcher is a windowsgui binary, so stderr is invisible in
		// protocol launches — a dialog is the only way the user sees failures.
		launcher.Logf("decode launch payload failed: %v", err)
		launcher.ShowLaunchFailure(err)
		exitWithError(err)
	}

	if printOnly {
		fmt.Printf("resource=%s type=%s target=%s method=%s\n", item.ResourceID, item.ResourceType, item.Target, item.Method)
		return
	}

	if err := launcher.Run(item); err != nil {
		launcher.ShowLaunchFailure(err)
		exitWithError(err)
	}
}

func shouldInstallByDefault(rawURI string, encodedPayload string, args []string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	if strings.TrimSpace(rawURI) != "" || strings.TrimSpace(encodedPayload) != "" {
		return false
	}
	if len(args) > 0 {
		first := strings.TrimSpace(args[0])
		if strings.HasPrefix(strings.ToLower(first), "access-workspace://") || first != "" {
			return false
		}
	}
	return true
}

func resolvePayload(rawURI string, encodedPayload string, args []string) (payload.LaunchPayload, error) {
	if strings.TrimSpace(rawURI) != "" {
		return payload.DecodeProtocolURI(rawURI)
	}
	if strings.TrimSpace(encodedPayload) != "" {
		return payload.DecodePayload(encodedPayload)
	}
	if len(args) > 0 {
		first := strings.TrimSpace(args[0])
		if strings.HasPrefix(strings.ToLower(first), "access-workspace://") {
			return payload.DecodeProtocolURI(first)
		}
		return payload.DecodePayload(first)
	}
	return payload.LaunchPayload{}, fmt.Errorf("no launch payload provided")
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, "access-workspace-launcher:", err)
	os.Exit(1)
}

func runBackgroundServer() error {
	go func() {
		_ = launcher.SyncAgentPrerequisites()
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			_ = launcher.SyncAgentPrerequisites()
		}
	}()

	listener, err := net.Listen("tcp", launcherinfo.ListenURL)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "only one usage") || strings.Contains(lower, "address already in use") {
			return nil
		}
		return err
	}
	defer listener.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		writeLauncherCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeLauncherJSON(w, http.StatusOK, map[string]any{
			"version": launcherinfo.Version,
			"ready":   true,
		})
	})
	mux.HandleFunc("/launch", func(w http.ResponseWriter, r *http.Request) {
		writeLauncherCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			writeLauncherJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var item payload.LaunchPayload
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeLauncherJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if err := launcher.Run(item); err != nil {
			writeLauncherJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeLauncherJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return (&http.Server{Handler: mux}).Serve(listener)
}

func writeLauncherCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func writeLauncherJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
