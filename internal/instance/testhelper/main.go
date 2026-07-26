//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/airnayden/openbiss/internal/instance"
)

func main() {
	tmpDir, err := os.MkdirTemp("", "openbiss-instance-t4-*")
	if err != nil {
		fail("mkdir temp", err)
	}
	defer os.RemoveAll(tmpDir)
	fmt.Printf("dataDir=%s\n", tmpDir)
	fmt.Printf("GOOS=%s\n", runtime.GOOS)

	var raised atomic.Int32
	instance.OnRaiseWindow(func() {
		raised.Add(1)
	})

	release1, alreadyRunning1, err := instance.TryAcquire(tmpDir)
	if err != nil {
		fail("first TryAcquire", err)
	}
	if alreadyRunning1 {
		fail("expected first TryAcquire alreadyRunning=false", nil)
	}
	if release1 == nil {
		fail("expected first TryAcquire to return non-nil release", nil)
	}
	fmt.Println("[OK] first instance: bound listener, alreadyRunning=false")

	endpointPath, endpointKind := endpointFile(tmpDir)
	if _, err := os.Stat(endpointPath); err != nil {
		fail(fmt.Sprintf("expected %s file to exist after first TryAcquire", endpointKind), err)
	}
	fmt.Printf("[OK] %s file present at %s\n", endpointKind, endpointPath)

	time.Sleep(50 * time.Millisecond)

	release2, alreadyRunning2, err := instance.TryAcquire(tmpDir)
	if err != nil {
		fail("second TryAcquire", err)
	}
	if !alreadyRunning2 {
		fail("expected second TryAcquire alreadyRunning=true", nil)
	}
	if release2 != nil {
		fail("expected nil release from second TryAcquire", nil)
	}
	fmt.Println("[OK] second instance: detected primary, alreadyRunning=true")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if raised.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := raised.Load()
	if got != 1 {
		fail(fmt.Sprintf("expected raise-window callback fired exactly once, got %d", got), nil)
	}
	fmt.Println("[OK] raise-window callback fired exactly once")

	release1()

	if _, err := os.Stat(endpointPath); !os.IsNotExist(err) {
		fail(fmt.Sprintf("expected %s file removed after release; stat err=%v", endpointKind, err), nil)
	}
	fmt.Printf("[OK] release closed listener and removed %s file\n", endpointKind)

	release1()
	fmt.Println("[OK] double-release is a no-op")

	if err := os.WriteFile(endpointPath, []byte("stale-not-a-real-endpoint"), 0o600); err != nil {
		fail("plant stale endpoint file", err)
	}
	fmt.Printf("[OK] stale (bogus) file planted at %s\n", endpointPath)

	release3, alreadyRunning3, err := instance.TryAcquire(tmpDir)
	if err != nil {
		fail("third TryAcquire after stale-file plant", err)
	}
	if alreadyRunning3 {
		fail("expected third TryAcquire alreadyRunning=false (stale recovery)", nil)
	}
	if release3 == nil {
		fail("expected third TryAcquire to return non-nil release", nil)
	}
	fmt.Println("[OK] third instance: cleaned stale file and bound fresh listener")
	release3()

	if _, _, err := instance.TryAcquire(""); err == nil {
		fail("expected empty dataDir to return error", nil)
	}
	fmt.Println("[OK] empty dataDir rejected with error")

	fmt.Println("\nALL CHECKS PASSED")
}

func endpointFile(dataDir string) (path string, kind string) {
	if runtime.GOOS == "windows" {
		return filepath.Join(dataDir, "openbiss.port"), "port"
	}
	return filepath.Join(dataDir, "openbiss.sock"), "socket"
}

func fail(msg string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s: %v\n", msg, err)
	} else {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", msg)
	}
	os.Exit(1)
}
