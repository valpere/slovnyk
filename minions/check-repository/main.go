// Command check-repository is an assistant-only shortcut to project checks.
package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/valpere/slovnyk/minions/internal/support"
)

func main() {
	root, err := support.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cmd := exec.Command("go", "test", "./tests/repository", "-count=1", "-v")
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			os.Exit(e.ExitCode())
		}
		os.Exit(1)
	}
}
