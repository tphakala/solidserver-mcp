//go:build windows

package main

import (
	"context"
	"fmt"
	"net"
	"os"
)

// bindUnixSocket binds an AF_UNIX socket on Windows, which has supported them
// since Windows 10. Windows has no umask and its file-mode bits do not map onto
// the 0600 owner-only model, so socket confidentiality relies on the ACLs of the
// directory the socket lives in. os.Chmod is applied best-effort for parity but
// only toggles the read-only bit on Windows.
func bindUnixSocket(ctx context.Context, path string) (net.Listener, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("listening on Unix socket %q: %w", path, err)
	}
	_ = os.Chmod(path, socketFileMode)
	return ln, nil
}
