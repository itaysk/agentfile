package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

type runningAgent struct {
	PID     int       `json:"pid"`
	Agent   string    `json:"agent"`
	Runtime string    `json:"runtime"`
	Mode    string    `json:"mode"`
	Started time.Time `json:"started"`
}

func runPS(args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af ps")
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("ps does not accept arguments")
	}
	runs, err := activeRuns()
	if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "PID\tAGENT\tRUNTIME\tMODE\tSTARTED")
	for _, run := range runs {
		fmt.Fprintf(writer, "%d\t%s\t%s\t%s\t%s\n", run.PID, run.Agent, run.Runtime, run.Mode, run.Started.UTC().Format(time.RFC3339))
	}
	return writer.Flush()
}

func trackRun(run runningAgent) (func(), error) {
	dir, err := runsDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(dir, ".run-*")
	if err != nil {
		return nil, err
	}
	tempPath := file.Name()
	cleanup := func() {
		_ = os.Remove(tempPath)
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		cleanup()
		return nil, err
	}
	// The kernel releases this lock even if af exits without running cleanup.
	if err := json.NewEncoder(file).Encode(run); err != nil {
		cleanup()
		return nil, err
	}
	path := filepath.Join(dir, strings.TrimPrefix(filepath.Base(tempPath), ".")+".json")
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return nil, err
	}
	tempPath = path
	return cleanup, nil
}

func activeRuns() ([]runningAgent, error) {
	dir, err := runsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	runs := make([]runningAgent, 0, len(entries))
	for _, entry := range entries {
		isRecord := filepath.Ext(entry.Name()) == ".json"
		isTemp := strings.HasPrefix(entry.Name(), ".run-")
		if entry.IsDir() || (!isRecord && !isTemp) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			_ = file.Close()
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			continue
		} else if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = file.Close()
			return nil, err
		}
		if isTemp {
			_ = file.Close()
			continue
		}
		var run runningAgent
		decodeErr := json.NewDecoder(file).Decode(&run)
		closeErr := file.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("read running agent %q: %w", path, decodeErr)
		}
		if closeErr != nil {
			return nil, closeErr
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].Started.Equal(runs[j].Started) {
			return runs[i].PID < runs[j].PID
		}
		return runs[i].Started.Before(runs[j].Started)
	})
	return runs, nil
}

func runsDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "agentfile", "runs"), nil
}
