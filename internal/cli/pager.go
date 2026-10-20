package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/cli/go-gh/v2/pkg/config"
	"github.com/cli/safeexec"
	"github.com/google/shlex"
)

func pagerCommand() (string, error) {
	if command, ok := os.LookupEnv("GH_PAGER"); ok {
		return command, nil
	}
	cfg, err := config.Read(nil)
	if err != nil {
		return "", fmt.Errorf("read pager configuration: %w", err)
	}
	command, err := cfg.Get([]string{"pager"})
	var missing *config.KeyNotFoundError
	if err != nil && !errors.As(err, &missing) {
		return "", fmt.Errorf("read pager setting: %w", err)
	}
	if command != "" {
		return command, nil
	}
	return os.Getenv("PAGER"), nil
}

func writePaged(ctx context.Context, output, stderr io.Writer, command string, content []byte) error {
	writeDirect := func() error {
		_, err := io.Copy(output, bytes.NewReader(content))
		return err
	}
	if command == "" || command == "cat" {
		return writeDirect()
	}
	startFailure := func(err error) error {
		if _, writeErr := fmt.Fprintf(stderr, "failed to start pager: %v\n", err); writeErr != nil {
			return writeErr
		}
		return writeDirect()
	}
	args, err := shlex.Split(command)
	if err != nil {
		return startFailure(err)
	}
	if len(args) == 0 {
		return startFailure(fmt.Errorf("pager command is empty"))
	}
	path, err := safeexec.LookPath(args[0])
	if err != nil {
		return startFailure(err)
	}
	cmd := exec.CommandContext(ctx, path, args[1:]...)
	// exec handles a closed stdin pipe when the user quits the pager early.
	cmd.Stdin = bytes.NewReader(content)
	cmd.Stdout, cmd.Stderr = output, stderr
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PAGER=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	if _, ok := os.LookupEnv("LESS"); !ok {
		cmd.Env = append(cmd.Env, "LESS=FRX")
	}
	if _, ok := os.LookupEnv("LV"); !ok {
		cmd.Env = append(cmd.Env, "LV=-c")
	}
	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return startFailure(err)
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("pager: %w", err)
	}
	return nil
}
