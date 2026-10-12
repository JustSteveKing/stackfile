package stack

import (
	"strings"
	"testing"
)

// basis.yml uses a pending basis, a version with its own records, and
// proposals with one candidate and with none. It must be clean.
func TestBasisExampleIsValid(t *testing.T) {
	f, ds := parseFile(t, "../testdata/basis.yml")
	if f == nil || len(ds) > 0 {
		for _, d := range ds {
			t.Log(d)
		}
		t.Fatal("testdata/basis.yml should be valid")
	}
	river, _ := f.Entry("river")
	if river.Basis != BasisPending || len(river.PendingOn) != 1 {
		t.Errorf("river = basis %q pending_on %v", river.Basis, river.PendingOn)
	}
	jcs, _ := f.Entry("jcs")
	if len(jcs.VersionDecidedBy) != 1 || jcs.VersionDecidedBy[0] != "RFC-0003" {
		t.Errorf("jcs version_decided_by = %v", jcs.VersionDecidedBy)
	}
	if f.Avoid[0].Basis != BasisPending {
		t.Errorf("asynq basis = %q", f.Avoid[0].Basis)
	}
}

const head = "stackfile: 1\nproject: x\nlayers:\n  a:\n    - name: a\n"

func TestBasisRules(t *testing.T) {
	cases := []struct {
		name, yaml string
		want       []want
	}{
		{"pending without records", head + "      status: adopt\n      basis: pending\n",
			[]want{{5, SeverityError, "basis: pending needs pending_on"}}},
		// anyOf fails on both branches; they must collapse into one message.
		{"proposed with neither", head + "      status: proposed\n",
			[]want{{5, SeverityError, "a proposed entry needs candidates, an open question, or both"}}},
		{"version records without a version", head + "      status: adopt\n      version_decided_by: [ADR-1]\n",
			[]want{{5, SeverityError, "version_decided_by needs 'version' too"}}},
		{"pending_on with declared", head + "      status: adopt\n      pending_on: [RFC-5]\n",
			[]want{{7, SeverityWarning, "set basis: pending"}}},
		{"pending_on with ratified", head + "      status: adopt\n      basis: ratified\n      decided_by: [RFC-1]\n      pending_on: [RFC-5]\n",
			[]want{{9, SeverityWarning, "nothing is pending"}}},
		{"decided_by with pending", head + "      status: adopt\n      basis: pending\n      pending_on: [RFC-5]\n      decided_by: [RFC-1]\n",
			[]want{{9, SeverityWarning, "basis is pending"}}},
		{"the same record in both", head + "      status: adopt\n      basis: pending\n      pending_on: [RFC-5]\n      decided_by: [RFC-5]\n",
			[]want{{8, SeverityError, "RFC-5 is in both"}, {9, SeverityWarning, "basis is pending"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ds := Parse([]byte(c.yaml))
			assertDiagnostics(t, ds, c.want)
		})
	}
}

func TestProposedWithOnlyAQuestionIsValid(t *testing.T) {
	_, ds := Parse([]byte(head + "      status: proposed\n      open: Which one?\n"))
	if len(ds) > 0 {
		t.Errorf("got %v", ds)
	}
}

func TestPendingMatchesInLookup(t *testing.T) {
	f, _ := parseFile(t, "../testdata/basis.yml")
	if got := f.Lookup("github.com/hibiken/asynq", "go"); got.Verdict != VerdictAvoid || !strings.Contains(strings.Join(got.Avoid.PendingOn, ""), "RFC-0005") {
		t.Errorf("asynq = %+v", got)
	}
}
