// Command prepare-artifacts provisions a durable local artifact root.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func prepare(path string) (string, error) {
	root, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(root, 0750); err != nil {
		return "", err
	}
	// Persist this directory and every ancestor entry before verification starts.
	for dir := root; ; dir = filepath.Dir(dir) {
		f, e := os.Open(dir)
		if e != nil {
			return "", e
		}
		info, e := f.Stat()
		if e == nil && !info.IsDir() {
			e = fmt.Errorf("not a directory")
		}
		if e == nil {
			e = f.Sync()
		}
		closeErr := f.Close()
		if e != nil {
			return "", e
		}
		if closeErr != nil {
			return "", closeErr
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return root, nil
}
func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: prepare-artifacts directory")
		os.Exit(2)
	}
	root, err := prepare(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot provision artifact directory")
		os.Exit(1)
	}
	fmt.Println(root)
}
