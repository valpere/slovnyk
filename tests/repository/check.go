// Package repository checks active Markdown and local-data ignore rules.
package repository

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var links = regexp.MustCompile(`\]\((<[^>]+>|[^\s)]+)(?:\s+"[^"]*")?\)`)
var scheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

type report struct {
	Passed           bool     `json:"passed"`
	MarkdownFiles    int      `json:"markdown_files"`
	IgnoredDataFiles int      `json:"ignored_data_files"`
	Errors           []string `json:"errors"`
}

func markdown(path, label string) []string {
	var problems []string
	b, err := os.ReadFile(path)
	if err != nil {
		return []string{"Cannot read document: " + label}
	}
	if !utf8.Valid(b) {
		return []string{"Invalid UTF-8: " + label}
	}
	body := string(b)
	for n, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.TrimRightFunc(line, unicode.IsSpace) != line {
			problems = append(problems, fmt.Sprintf("%s:%d: trailing whitespace", label, n+1))
		}
	}
	for _, match := range links.FindAllStringSubmatch(body, -1) {
		target := strings.Trim(match[1], "<>")
		target = strings.SplitN(strings.SplitN(target, "#", 2)[0], "?", 2)[0]
		if target == "" || scheme.MatchString(target) || strings.HasPrefix(target, "/") {
			continue
		}
		decoded, e := url.PathUnescape(target)
		if e != nil {
			problems = append(problems, label+": invalid link encoding "+target)
			continue
		}
		if _, e = os.Stat(filepath.Join(filepath.Dir(path), decoded)); e != nil {
			problems = append(problems, label+": missing link target "+target)
		}
	}
	return problems
}

func files(root, dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, e := filepath.Rel(root, p)
			if e != nil {
				return e
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if os.IsNotExist(err) {
		return out, nil
	}
	sort.Strings(out)
	return out, err
}

// NUL-delimited paths keep spaces and newlines unambiguous to Git.
func ignored(root string, paths []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(paths) == 0 {
		return out, nil
	}
	cmd := exec.Command("git", "check-ignore", "--no-index", "-z", "--stdin")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	b, err := cmd.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 {
			return nil, fmt.Errorf("git check-ignore failed")
		}
	}
	for _, p := range bytes.Split(b, []byte{0}) {
		if len(p) > 0 {
			out[string(p)] = true
		}
	}
	return out, nil
}

func check(root string, args []string) report {
	r := report{Errors: []string{}}
	paths := args
	if len(paths) == 0 {
		paths = []string{"README.md", "data/README.md", "minions/README.md", "utilities/README.md", "tests/README.md", "testdata/README.md"}
		docs, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
		if err != nil {
			r.Errors = append(r.Errors, "Cannot list documents")
		}
		for _, p := range docs {
			rel, _ := filepath.Rel(root, p)
			paths = append(paths, rel)
		}
	}
	r.MarkdownFiles = len(paths)
	for _, p := range paths {
		absolute := p
		if !filepath.IsAbs(p) {
			absolute = filepath.Join(root, p)
		}
		r.Errors = append(r.Errors, markdown(absolute, p)...)
	}
	data, err := files(root, "data/experiments")
	if err != nil {
		r.Errors = append(r.Errors, "Cannot list local data")
	}
	r.IgnoredDataFiles = len(data)
	ignoredData, err := ignored(root, data)
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
	} else {
		for _, p := range data {
			if !ignoredData[p] {
				r.Errors = append(r.Errors, "Local data is not ignored: "+p)
			}
		}
	}
	sources := []string{"data/README.md"}
	for _, dir := range []string{"minions", "utilities", "tests", "testdata"} {
		list, e := files(root, dir)
		if e != nil {
			r.Errors = append(r.Errors, "Cannot list sources: "+dir)
		}
		sources = append(sources, list...)
	}
	ignoredSources, err := ignored(root, sources)
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
	} else {
		for _, p := range sources {
			if ignoredSources[p] {
				r.Errors = append(r.Errors, "Source ignored: "+p)
			}
		}
	}
	r.Passed = len(r.Errors) == 0
	return r
}
