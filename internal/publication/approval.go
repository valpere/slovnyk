package publication

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/valpere/slovnyk/internal/importdata"
)

const MaxApprovalBytes = 16 << 10

// LossApproval records an owner decision conveyed by the trusted CLI operator.
// It is audit evidence, not a signature or an authentication mechanism.
type LossApproval struct {
	Version              int    `json:"version"`
	Dataset              string `json:"dataset"`
	Fingerprint          string `json:"fingerprint"`
	ReconciliationSHA256 string `json:"reconciliation_sha256"`
	OwnerRef             string `json:"owner_ref"`
	DecisionRef          string `json:"decision_ref"`
	ApprovedAt           string `json:"approved_at"`
}

// ParseLossApproval accepts a bounded, flat object with no duplicate/unknown
// keys or trailing values. No approval is inferred from a reconciliation report.
func ParseLossApproval(b []byte) (*LossApproval, error) {
	if len(b) > MaxApprovalBytes || !utf8.Valid(b) {
		return nil, ErrArguments
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if token, e := d.Token(); e != nil || token != json.Delim('{') {
		return nil, ErrArguments
	}
	seen := map[string]bool{}
	for d.More() {
		token, e := d.Token()
		key, ok := token.(string)
		if e != nil || !ok || seen[key] {
			return nil, ErrArguments
		}
		switch key {
		case "version", "dataset", "fingerprint", "reconciliation_sha256", "owner_ref", "decision_ref", "approved_at":
		default:
			return nil, ErrArguments
		}
		seen[key] = true
		value, e := d.Token()
		if _, nested := value.(json.Delim); e != nil || nested || value == nil {
			return nil, ErrArguments
		}
	}
	if token, e := d.Token(); e != nil || token != json.Delim('}') {
		return nil, ErrArguments
	}
	if _, e := d.Token(); e != io.EOF {
		return nil, ErrArguments
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var a LossApproval
	if d.Decode(&a) != nil || !a.valid() {
		return nil, ErrArguments
	}
	return &a, nil
}

func validSHA256(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == sha256.Size && s == strings.ToLower(s)
}

func (a LossApproval) valid() bool {
	if a.Version != 1 || !ValidID(a.Dataset) || !validSHA256(a.Fingerprint) || !validSHA256(a.ReconciliationSHA256) {
		return false
	}
	for _, ref := range []string{a.OwnerRef, a.DecisionRef} {
		if strings.TrimSpace(ref) == "" || len(ref) > 512 || !utf8.ValidString(ref) || strings.ContainsAny(ref, "\x00\r\n") {
			return false
		}
	}
	_, e := time.Parse(time.RFC3339, a.ApprovedAt)
	return e == nil
}

func reconciliationReport(r Report, m importdata.Manifest) ReconciliationReport {
	return ReconciliationReport{
		Version: 1, Kind: "reconciliation", Dataset: r.Dataset, Fingerprint: r.Fingerprint, State: "validating",
		Parsed: r.Parsed, Unique: r.Unique, Duplicates: r.Duplicates, Rejected: r.Rejected,
		Files: r.Files, KnownGaps: m.KnownGaps, Exceptions: m.Exceptions,
	}
}

// checkLossApproval binds a decision to the exact deterministic JSON emitted by
// dataset reconcile (including its final LF). All source/inventory guards still
// run; exceptions and rejected records remain unsupported even with approval.
func checkLossApproval(r Report, m importdata.Manifest) error {
	if len(m.Exceptions) != 0 || !slices.Equal(r.KnownGaps, m.KnownGaps) {
		return ErrVerification
	}
	a := r.LossApproval
	if len(m.KnownGaps) == 0 {
		if a != nil {
			return ErrVerification
		}
		return nil
	}
	if a == nil || !a.valid() || a.Dataset != r.Dataset || a.Fingerprint != r.Fingerprint || r.Rejected != 0 {
		return ErrVerification
	}
	b, e := json.Marshal(reconciliationReport(r, m))
	if e != nil {
		return ErrVerification
	}
	h := sha256.Sum256(append(b, '\n'))
	if a.ReconciliationSHA256 != hex.EncodeToString(h[:]) {
		return ErrVerification
	}
	return nil
}
