//go:build darwin || linux

package instance

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

const sockFilename = "openbiss.sock"

// TryAcquire attempts to bind a Unix domain socket at $dataDir/openbiss.sock.
//
// On success it returns release, alreadyRunning=false, nil; release closes the
// listener and removes the socket file.
//
// If the socket is already in use by a live primary, it dials the primary,
// sends the raise-window byte, and returns alreadyRunning=true; the caller
// should exit. A stale socket left by a crashed primary is removed and the
// listen is retried once.
func TryAcquire(dataDir string) (release func(), alreadyRunning bool, err error) {
	if dataDir == "" {
		return nil, false, errors.New("instance: dataDir is empty")
	}
	sockPath := filepath.Join(dataDir, sockFilename)

	ln, err := net.Listen("unix", sockPath)
	if err == nil {
		return startAcceptor(ln, sockPath), false, nil
	}

	dialAddr := &net.UnixAddr{Name: sockPath, Net: "unix"}
	conn, dialErr := net.DialTimeout("unix", dialAddr.Name, raiseTimeoutMillis*time.Millisecond)
	if dialErr == nil {
		_, writeErr := conn.Write([]byte{raiseByte})
		_ = conn.Close()
		if writeErr != nil {
			return nil, false, fmt.Errorf("instance: send raise byte: %w", writeErr)
		}
		return nil, true, nil
	}

	if removeErr := os.Remove(sockPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return nil, false, fmt.Errorf("instance: remove stale socket %s: %w", sockPath, removeErr)
	}

	ln, err = net.Listen("unix", sockPath)
	if err != nil {
		return nil, false, fmt.Errorf("instance: listen %s after stale cleanup: %w", sockPath, err)
	}
	return startAcceptor(ln, sockPath), false, nil
}

func startAcceptor(ln net.Listener, sockPath string) func() {
	go acceptLoop(ln)

	var released bool
	return func() {
		if released {
			return
		}
		released = true
		_ = ln.Close()
		_ = os.Remove(sockPath)
	}
}

func acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(raiseTimeoutMillis * time.Millisecond))
	buf := [1]byte{}
	n, err := conn.Read(buf[:])
	if err != nil || n != 1 {
		return
	}
	if buf[0] == raiseByte {
		invokeRaiseWindow()
	}
}
