//go:build !darwin || !cgo

package main

// waitForLaunchURL: Windows and Linux pass the launch URL in argv. A darwin
// build without cgo cannot receive the Apple Event, so URL-scheme launches
// there fall back to the install flow; release builds always use cgo.
func waitForLaunchURL() string {
	return ""
}
