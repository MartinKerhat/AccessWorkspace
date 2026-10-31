//go:build !windows && (!darwin || !cgo)

package launcher

func currentDisplaySize() (int, int) {
	return 1600, 900
}
