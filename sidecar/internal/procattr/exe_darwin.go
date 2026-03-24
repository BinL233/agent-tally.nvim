//go:build darwin

package procattr

// resolveExeName is a no-op on Darwin because ps comm reliably returns the
// binary basename — no thread-name renaming occurs like on Linux.
func resolveExeName(_ int) string {
	return ""
}
