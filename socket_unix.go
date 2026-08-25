//go:build !windows

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
)

// bindUnixSocket binds a Unix domain socket at path with 0600 permissions. It
// sets a restrictive umask around Listen so the socket is never briefly
// world-accessible between bind and chmod (which a plain chmod-after-listen
// would allow), then asserts the mode explicitly as defense in depth.
func bindUnixSocket(ctx context.Context, path string) (net.Listener, error) {
	// Close the create-then-chmod race: create the socket with no group/other
	// bits by masking them at bind time, then restore the previous umask
	// immediately after. Umask is process-global, but runUnix runs once at
	// single-threaded startup before any request goroutine exists, so the brief
	// tightened-mask window affects nothing else.
	var lc net.ListenConfig
	oldMask := syscall.Umask(socketUmask)
	ln, err := lc.Listen(ctx, "unix", path)
	syscall.Umask(oldMask)
	if err != nil {
		return nil, fmt.Errorf("listening on Unix socket %q: %w", path, err)
	}

	if err := os.Chmod(path, socketFileMode); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("setting permissions on Unix socket %q: %w", path, err)
	}
	return ln, nil
}
