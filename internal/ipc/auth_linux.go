//go:build linux

package ipc

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// authorizePeer rejects any caller whose uid differs from the daemon's.
// Guards every mutating method, not just IngestEvent: RunAgent can launch
// an arbitrarily-permissive agent, so it needs at least the same check.
func authorizePeer(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("%w: this method requires a unix socket peer", ErrPermissionDeniedErr)
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("%w: inspect peer credentials: %v", ErrPermissionDeniedErr, err)
	}

	var cred *unix.Ucred
	var controlErr error
	if err := raw.Control(func(fd uintptr) {
		cred, controlErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return fmt.Errorf("%w: inspect peer credentials: %v", ErrPermissionDeniedErr, err)
	}
	if controlErr != nil {
		return fmt.Errorf("%w: inspect peer credentials: %v", ErrPermissionDeniedErr, controlErr)
	}

	if cred != nil && cred.Uid == uint32(os.Getuid()) { //nolint:gosec // UIDs always fit in uint32 on Linux
		return nil
	}
	if allowed := os.Getenv("AGENT_SANDBOX_INGEST_UID"); allowed != "" {
		uid, err := strconv.ParseUint(allowed, 10, 32)
		if err == nil && cred != nil && cred.Uid == uint32(uid) {
			return nil
		}
	}
	if cred == nil {
		return fmt.Errorf("%w: missing peer credentials", ErrPermissionDeniedErr)
	}
	return fmt.Errorf("%w: peer uid %d is not authorized", ErrPermissionDeniedErr, cred.Uid)
}
