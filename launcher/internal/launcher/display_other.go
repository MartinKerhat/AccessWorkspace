//go:build !windows

package launcher

func currentDisplaySize() (int, int) {
	return 1600, 900
}
