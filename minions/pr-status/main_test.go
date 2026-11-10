package main

import "testing"

func TestAssess(t *testing.T) {
	green := []job{{expectedJobs[0], "completed", "success"}, {expectedJobs[1], "completed", "success"}}
	for _, tc := range []struct {
		name string
		jobs []job
		want int
	}{
		{"green", green, 0},

		{"missing job", green[:1], 1},
		{"duplicate names", []job{green[0], green[0]}, 1},
		{"skipped is not success", []job{green[0], {expectedJobs[1], "completed", "skipped"}}, 1},
		{"failure while other pending", []job{{expectedJobs[0], "in_progress", ""}, {expectedJobs[1], "completed", "failure"}}, 1},
		{"pending", []job{green[0], {expectedJobs[1], "queued", ""}}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := assess(details{Jobs: tc.jobs, Status: "completed", Conclusion: "success"}); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestLatestExactHeadAndEvent(t *testing.T) {
	runs := []run{{DatabaseID: 10, HeadSha: "old", Event: "push"}, {DatabaseID: 8, HeadSha: "current", Event: "push"}, {DatabaseID: 9, HeadSha: "current", Event: "pull_request"}, {DatabaseID: 7, HeadSha: "current", Event: "push"}}
	r, ok := latest(runs, "current", "push")
	if !ok || r.DatabaseID != 8 {
		t.Fatalf("wrong run: %+v", r)
	}
	if _, ok := latest(runs, "missing", "push"); ok {
		t.Fatal("accepted a different head")
	}
}

func TestWorkflowConclusionAlsoRequired(t *testing.T) {
	green := []job{{expectedJobs[0], "completed", "success"}, {expectedJobs[1], "completed", "success"}}
	for _, tc := range []struct {
		status, conclusion string
		want               int
	}{
		{"in_progress", "", 2}, {"completed", "cancelled", 1}, {"completed", "failure", 1},
	} {
		if got := assess(details{Jobs: green, Status: tc.status, Conclusion: tc.conclusion}); got != tc.want {
			t.Fatalf("workflow %s/%s: got %d want %d", tc.status, tc.conclusion, got, tc.want)
		}
	}
}

func TestNoJobsQueuedVersusCancelled(t *testing.T) {
	if got := assess(details{Status: "queued"}); got != 2 {
		t.Fatalf("queued = %d", got)
	}
	if got := assess(details{Status: "completed", Conclusion: "cancelled"}); got != 1 {
		t.Fatalf("cancelled = %d", got)
	}
}
