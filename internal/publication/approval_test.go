package publication

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/importdata"
)

func sampleApproval() LossApproval {
	return LossApproval{Version: 1, Dataset: "00000000-0000-0000-0000-000000000001",
		Fingerprint: strings.Repeat("a", 64), ReconciliationSHA256: strings.Repeat("b", 64),
		OwnerRef: "synthetic owner", DecisionRef: "synthetic decision", ApprovedAt: "2026-11-04T00:00:00Z"}
}

func TestApprovalParsing(t *testing.T) {
	a := sampleApproval()
	b, _ := json.Marshal(a)
	parsed, e := ParseLossApproval(b)
	if e != nil || *parsed != a {
		t.Fatalf("valid approval: %+v %v", parsed, e)
	}
	for name, b := range map[string][]byte{
		"duplicate":  append([]byte(`{"version":1,`), b[1:]...),
		"unknown":    append([]byte(`{"extra":true,`), b[1:]...),
		"case alias": append([]byte(`{"Version":1,`), b[1:]...),
		"nested":     []byte(`{"version":{}}`),
		"array":      []byte(`[]`),
		"missing":    []byte(`{"version":1}`),
		"null":       []byte(`{"version":null}`),
		"trailing":   append(append([]byte{}, b...), []byte(` {}`)...),
		"oversized":  []byte(strings.Repeat(" ", MaxApprovalBytes+1)),
		"encoding":   {0xff},
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := ParseLossApproval(b); e == nil {
				t.Fatal("invalid approval admitted")
			}
		})
	}
	for name, change := range map[string]func(*LossApproval){
		"version":   func(a *LossApproval) { a.Version = 0 },
		"hash":      func(a *LossApproval) { a.ReconciliationSHA256 = strings.Repeat("B", 64) },
		"identity":  func(a *LossApproval) { a.Dataset = "invalid" },
		"owner":     func(a *LossApproval) { a.OwnerRef = " " },
		"decision":  func(a *LossApproval) { a.DecisionRef = "" },
		"timestamp": func(a *LossApproval) { a.ApprovedAt = "tomorrow" },
	} {
		t.Run(name, func(t *testing.T) {
			a := sampleApproval()
			change(&a)
			b, _ := json.Marshal(a)
			if _, e := ParseLossApproval(b); e == nil {
				t.Fatal("invalid decision admitted")
			}
		})
	}
}

func TestApprovalBindsExactReconciliation(t *testing.T) {
	a := sampleApproval()
	m := importdata.Manifest{KnownGaps: []string{"missing historical metadata"}, Exceptions: []importdata.Exception{}}
	r := Report{Version: 1, Dataset: a.Dataset, Fingerprint: a.Fingerprint, Parsed: 10, Unique: 10, KnownGaps: m.KnownGaps, LossApproval: &a}
	b, _ := json.Marshal(reconciliationReport(r, m))
	h := sha256.Sum256(append(b, '\n'))
	a.ReconciliationSHA256 = hex.EncodeToString(h[:])
	if e := checkLossApproval(r, m); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*Report, *importdata.Manifest){
		"no decision":       func(r *Report, _ *importdata.Manifest) { r.LossApproval = nil },
		"other dataset":     func(r *Report, _ *importdata.Manifest) { r.Dataset = "00000000-0000-0000-0000-000000000002" },
		"other fingerprint": func(r *Report, _ *importdata.Manifest) { r.Fingerprint = strings.Repeat("c", 64) },
		"changed counts":    func(r *Report, _ *importdata.Manifest) { r.Parsed++ },
		"rejected":          func(r *Report, _ *importdata.Manifest) { r.Rejected++ },
		"changed gap": func(r *Report, m *importdata.Manifest) {
			m.KnownGaps = []string{"different gap"}
			r.KnownGaps = m.KnownGaps
		},
		"hidden gap": func(r *Report, _ *importdata.Manifest) { r.KnownGaps = nil },
		"exception": func(_ *Report, m *importdata.Manifest) {
			m.Exceptions = []importdata.Exception{{Reason: "not supported"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, m := r, m
			change(&r, &m)
			if checkLossApproval(r, m) == nil {
				t.Fatal("mismatched decision admitted")
			}
		})
	}
}
