package queue

import (
	"strings"
	"testing"
	"time"
)

// TestRecords: AppendRecord appends and refuses an unknown kind, a record
// with no note, and a security-ack that is not the human's; Decided for you
// lists a departure, a return past the second and a re-estimate first; the
// landing body renders every kind it holds (FLOW-20, Decision 13).
func TestRecords(t *testing.T) {
	dir := t.TempDir()
	add := func(r Record) error {
		r.At = now
		return AppendRecord(dir, r)
	}
	for _, bad := range []Record{
		{By: "cloud/run", Kind: "whim", Note: "?"},
		{By: "cloud/run", Kind: "merge"},
		{By: "cloud/run", Kind: "security-ack", Note: "read"},
	} {
		if err := add(bad); err == nil {
			t.Errorf("AppendRecord(%+v) succeeded", bad)
		}
	}
	for _, kind := range RecordKinds {
		by := "cloud/run"
		if kind == "security-ack" {
			by = "human/jeff"
		}
		if err := add(Record{By: by, Kind: kind, Feature: "p", Repo: "o/here",
			Note: kind + " decided"}); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := LoadRecords(dir)
	if err != nil || len(rs) != len(RecordKinds) {
		t.Fatalf("LoadRecords = %d records, %v", len(rs), err)
	}
	got := DecidedForYou(rs)
	for i, want := range []string{"departure", "return", "re-estimate", "merge"} {
		if got[i].Kind != want {
			t.Fatalf("Decided for you[%d] = %s, want %s", i, got[i].Kind, want)
		}
	}
	body := ProgramBody(rs, "o/here")
	for _, kind := range RecordKinds {
		if !strings.Contains(body, "### "+kind+" (1)") {
			t.Errorf("the landing body has no %s section:\n%s", kind, body)
		}
	}
	if strings.Index(body, "### deferred") > strings.Index(body, "### merge") {
		t.Errorf("deferred findings are not listed first:\n%s", body)
	}
	if b := ProgramBody(rs, "o/elsewhere"); !strings.Contains(b, "No record.") {
		t.Errorf("another repository's body:\n%s", b)
	}
}

// TestDecidedForYouInQueue: QUEUE.md lists the week's records under Decided
// for you, a departure first (FLOW-20).
func TestDecidedForYouInQueue(t *testing.T) {
	rs := []Record{
		{At: now, By: "cloud/run", Kind: "merge", Feature: "p", Note: "merged"},
		{At: now, By: "cloud/run", Kind: "departure", Feature: "p", Repo: "o/here",
			Note: "departure decided"},
	}
	s := state(feature("p", 1, "in_progress"))
	s.Features[0].Log(now, "cloud/run", "x")
	s.Records = append(rs, Record{At: now.Add(-30 * 24 * time.Hour), By: "cloud/run",
		Kind: "tag", Note: "an old tag"})
	md := Summary(s)
	if !strings.Contains(md, "## Decided for you") ||
		!strings.Contains(md, "departure **p** o/here: departure decided") ||
		strings.Contains(md, "an old tag") {
		t.Fatalf("Decided for you:\n%s", md)
	}
}
