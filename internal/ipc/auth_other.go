//go:build !linux

package ipc

import "net"

func authorizePeer(net.Conn) error {
	return nil
}
