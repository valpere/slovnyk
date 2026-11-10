// pr-status reads the exact-head push and PR checks; it never mutates GitHub.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type run struct {
	DatabaseID                         int64
	Event, HeadSha, Status, Conclusion string
}

type job struct{ Name, Status, Conclusion string }
type details struct {
	HeadSha, Event, Status, Conclusion string
	Jobs                               []job
}

var expectedJobs = []string{"Go checks and HTTP smoke", "PostgreSQL integration"}

// 0 = all four passed; 1 = failed or unexpected job set; 2 = pending.
func assess(d details) int {
	if d.Status == "completed" && (d.Conclusion != "success" || len(d.Jobs) != len(expectedJobs)) {
		return 1
	}
	if len(d.Jobs) == 0 {
		return 2
	}
	if len(d.Jobs) != len(expectedJobs) {
		return 1
	}
	seen := map[string]bool{}
	code := 0
	for _, j := range d.Jobs {
		known := false
		for _, name := range expectedJobs {
			if j.Name == name {
				known = true
			}
		}
		if !known || seen[j.Name] {
			return 1
		}
		seen[j.Name] = true
		if j.Status != "completed" {
			code = 2
			continue
		}
		if j.Conclusion != "success" {
			return 1
		}
	}
	if d.Status != "completed" {
		return 2
	}
	if d.Conclusion != "success" {
		return 1
	}
	return code
}

func latest(runs []run, head, event string) (run, bool) {
	var selected run
	for _, r := range runs {
		if r.HeadSha == head && r.Event == event && r.DatabaseID > selected.DatabaseID {
			selected = r
		}
	}
	return selected, selected.DatabaseID != 0
}

func gh(ctx context.Context, dst any, args ...string) error {
	// Do not print command output on failure: authentication errors can be private.
	b, err := exec.CommandContext(ctx, "gh", args...).Output()
	if err != nil {
		return fmt.Errorf("gh %s failed: %w", args[0], err)
	}
	return json.Unmarshal(b, dst)
}

func check(number string) (int, error) {
	n, err := strconv.Atoi(number)
	if err != nil || n <= 0 {
		return 1, fmt.Errorf("PR number must be positive")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var pr struct{ HeadRefOid, HeadRefName, State string }
	if err := gh(ctx, &pr, "pr", "view", number, "--json", "headRefOid,headRefName,state"); err != nil {
		return 1, err
	}
	fmt.Printf("PR %d %s head %s\n", n, pr.State, pr.HeadRefOid)
	var runs []run
	if err := gh(ctx, &runs, "run", "list", "--workflow", "ci.yml", "--branch", pr.HeadRefName, "--commit", pr.HeadRefOid, "--limit", "100", "--json", "databaseId,event,headSha,status,conclusion"); err != nil {
		return 1, err
	}
	code := 0
	for _, event := range []string{"push", "pull_request"} {
		r, ok := latest(runs, pr.HeadRefOid, event)
		if !ok {
			fmt.Printf("%s: missing run (pending)\n", event)
			if code == 0 {
				code = 2
			}
			continue
		}
		var d details
		if err := gh(ctx, &d, "run", "view", strconv.FormatInt(r.DatabaseID, 10), "--json", "headSha,event,status,conclusion,jobs"); err != nil {
			return 1, err
		}
		if d.HeadSha != pr.HeadRefOid || d.Event != event {
			return 1, fmt.Errorf("run identity changed")
		}
		fmt.Printf("%s run %d\n", event, r.DatabaseID)
		for _, j := range d.Jobs {
			fmt.Printf("  %s: %s %s\n", j.Name, j.Status, j.Conclusion)
		}
		result := assess(d)
		if result == 1 || (result == 2 && code == 0) {
			code = result
		}
	}
	// A force-push during the queries invalidates the entire observation.
	var after struct{ HeadRefOid string }
	if err := gh(ctx, &after, "pr", "view", number, "--json", "headRefOid"); err != nil {
		return 1, err
	}
	if after.HeadRefOid != pr.HeadRefOid {
		return 1, fmt.Errorf("PR head changed; run again")
	}
	return code, nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: pr-status PR_NUMBER")
		os.Exit(1)
	}
	code, err := check(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}
