//go:build integration

package e2ereal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type childProcess struct {
	name string
	done chan struct{}
	err  error
}

func buildBinary(ctx context.Context, moduleRoot, outputPath, packagePath string) error {
	command := exec.CommandContext(ctx, "go", "build", "-o", outputPath, packagePath)
	command.Dir = moduleRoot
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("build %s: %w", packagePath, err)
	}
	return nil
}

func executablePath(directory, name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(directory, name)
}

func startProcess(ctx context.Context, name, executable, workdir string, args []string, environment []string) (*childProcess, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = workdir
	command.Env = append(os.Environ(), environment...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	process := &childProcess{name: name, done: make(chan struct{})}
	go func() {
		process.err = command.Wait()
		close(process.done)
	}()
	return process, nil
}

func waitTCP(ctx context.Context, address string, process *childProcess) error {
	return waitFor(ctx, process, func(probeCtx context.Context) error {
		dialer := net.Dialer{Timeout: time.Second}
		connection, err := dialer.DialContext(probeCtx, "tcp", address)
		if err != nil {
			return err
		}
		return connection.Close()
	})
}

func waitHTTP(ctx context.Context, endpoint string, process *childProcess) error {
	client := &http.Client{Timeout: 2 * time.Second}
	return waitFor(ctx, process, func(probeCtx context.Context) error {
		request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("unexpected status %d", response.StatusCode)
		}
		return nil
	})
}

func waitFor(ctx context.Context, process *childProcess, probe func(context.Context) error) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := probe(probeCtx)
		cancel()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-process.done:
			processErr := process.err
			if processErr == nil {
				processErr = errors.New("process exited unexpectedly")
			}
			return fmt.Errorf("%s stopped before readiness: %w", process.name, processErr)
		case <-ticker.C:
		}
	}
}

func waitForProcessExit(ctx context.Context, process *childProcess) error {
	if process == nil {
		return nil
	}
	select {
	case <-process.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for %s to stop: %w", process.name, ctx.Err())
	}
}
