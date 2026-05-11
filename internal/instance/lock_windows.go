//go:build windows

package instance

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const portFilename = "openbiss.port"

// TryAcquire binds a TCP listener on 127.0.0.1 with an OS-assigned port and
// records the port in $dataDir/openbiss.port.
//
// On success it returns release, alreadyRunning=false, nil; release closes the
// listener and removes the port file.
//
// If a port file exists and the recorded port responds, it dials the primary,
// sends the raise-window byte, and returns alreadyRunning=true. A stale port
// file (no listener responds) is removed and a new listener is opened.
func TryAcquire(dataDir string) (release func(), alreadyRunning bool, err error) {
	if dataDir == "" {
		return nil, false, errors.New("instance: dataDir is empty")
	}
	portPath := filepath.Join(dataDir, portFilename)

	if existingPort, ok := readPortFile(portPath); ok {
		addr := fmt.Sprintf("127.0.0.1:%d", existingPort)
		conn, dialErr := net.DialTimeout("tcp", addr, raiseTimeoutMillis*time.Millisecond)
		if dialErr == nil {
			_, writeErr := conn.Write([]byte{raiseByte})
			_ = conn.Close()
			if writeErr != nil {
				return nil, false, fmt.Errorf("instance: send raise byte: %w", writeErr)
			}
			return nil, true, nil
		}
		if removeErr := os.Remove(portPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return nil, false, fmt.Errorf("instance: remove stale port file %s: %w", portPath, removeErr)
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, false, fmt.Errorf("instance: listen on loopback: %w", err)
	}

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return nil, false, fmt.Errorf("instance: unexpected listener address %T", ln.Addr())
	}
	port := tcpAddr.Port

	if writeErr := os.WriteFile(portPath, []byte(strconv.Itoa(port)), 0o600); writeErr != nil {
		_ = ln.Close()
		return nil, false, fmt.Errorf("instance: write port file %s: %w", portPath, writeErr)
	}

	return startAcceptor(ln, portPath), false, nil
}

func readPortFile(portPath string) (int, bool) {
	data, err := os.ReadFile(portPath)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

func startAcceptor(ln net.Listener, portPath string) func() {
	go acceptLoop(ln)

	var released bool
	return func() {
		if released {
			return
		}
		released = true
		_ = ln.Close()
		_ = os.Remove(portPath)
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
