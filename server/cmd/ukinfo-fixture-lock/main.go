//go:build unix

// ukinfo-fixture-lock serializes private fixture PostgreSQL processes across
// container PID namespaces. The parent retains the lock until its child exits.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/gofrs/flock"
)

func main() { os.Exit(run()) }

func run() int {
	if os.Geteuid() == 0 || len(os.Args) < 2 || os.Args[1] != "postgres" {
		fmt.Fprintln(os.Stderr, "Requires non-root postgres command")
		return 2
	}
	lock := flock.New("/var/lib/postgresql/.ukinfo-fixture.lock")
	ok, err := lock.TryLock()
	if err != nil || !ok {
		fmt.Fprintln(os.Stderr, "Private fixture is locked")
		return 1
	}
	defer lock.Unlock()
	// Install signal handling before starting PostgreSQL so termination cannot
	// release the lock while the child is still shutting down.
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(signals)
	child := exec.Command("/usr/local/bin/postgres", os.Args[2:]...)
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "Cannot start fixture PostgreSQL")
		return 1
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	for {
		select {
		case sig := <-signals:
			_ = child.Process.Signal(sig)
		case err := <-done:
			if err == nil {
				return 0
			}
			if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() >= 0 {
				return exit.ExitCode()
			}
			return 1
		}
	}
}
