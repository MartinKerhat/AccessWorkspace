//go:build darwin

package launcher

import "github.com/MartinKerhat/AccessWorkspace/launcher/internal/macdialog"

// showLauncherMessageBox surfaces a launch failure to the user with a
// standard alert; the log remains the source of truth.
func showLauncherMessageBox(message string, title string) {
	_ = macdialog.Start(message, title, "caution")
}
