package screens

import (
	"testing"
	"time"

	"github.com/airnayden/openbiss/internal/server"
)

func mkRec(t time.Time, path string) server.RequestRecord {
	return server.RequestRecord{Time: t, Method: "GET", Path: path, StatusCode: 200}
}

func TestResolveSelection_EmptyRecords(t *testing.T) {
	chronoIdx, listID := resolveSelection(nil, 0)
	if chronoIdx != -1 || listID != -1 {
		t.Errorf("empty records: got (%d, %d), want (-1, -1)", chronoIdx, listID)
	}
}

func TestResolveSelection_NoSelectionDefaultsToNewest(t *testing.T) {
	base := time.Unix(0, 1_000_000_000)
	records := []server.RequestRecord{
		mkRec(base, "/a"),
		mkRec(base.Add(time.Second), "/b"),
		mkRec(base.Add(2*time.Second), "/c"),
	}
	chronoIdx, listID := resolveSelection(records, 0)
	if chronoIdx != 2 {
		t.Errorf("chronoIdx = %d, want 2 (newest)", chronoIdx)
	}
	if listID != 0 {
		t.Errorf("listID = %d, want 0 (top of newest-first list)", listID)
	}
}

func TestResolveSelection_MatchesByTimestamp(t *testing.T) {
	base := time.Unix(0, 1_000_000_000)
	records := []server.RequestRecord{
		mkRec(base, "/a"),
		mkRec(base.Add(time.Second), "/b"),
		mkRec(base.Add(2*time.Second), "/c"),
	}
	chronoIdx, listID := resolveSelection(records, records[0].Time.UnixNano())
	if chronoIdx != 0 {
		t.Errorf("chronoIdx = %d, want 0", chronoIdx)
	}
	if listID != 2 {
		t.Errorf("listID = %d, want 2 (bottom of newest-first list)", listID)
	}
}

func TestResolveSelection_AnchorsAcrossRingShift(t *testing.T) {
	base := time.Unix(0, 1_000_000_000)
	records := []server.RequestRecord{
		mkRec(base, "/a"),
		mkRec(base.Add(time.Second), "/b"),
		mkRec(base.Add(2*time.Second), "/c"),
	}
	target := records[1].Time.UnixNano()

	chronoIdxBefore, listIDBefore := resolveSelection(records, target)
	if chronoIdxBefore != 1 || listIDBefore != 1 {
		t.Fatalf("before shift: (%d,%d), want (1,1)", chronoIdxBefore, listIDBefore)
	}

	shifted := append(records, mkRec(base.Add(3*time.Second), "/d"))
	chronoIdxAfter, listIDAfter := resolveSelection(shifted, target)
	if chronoIdxAfter != 1 {
		t.Errorf("after append: chronoIdx = %d, want still 1 (record didn't move chronologically)", chronoIdxAfter)
	}
	if listIDAfter != 2 {
		t.Errorf("after append: listID = %d, want 2 (record pushed down one row in newest-first list)", listIDAfter)
	}
}

func TestResolveSelection_EvictedSelectionFallsBackToNewest(t *testing.T) {
	base := time.Unix(0, 1_000_000_000)
	originalSelection := base.Add(2 * time.Second).UnixNano()
	postEviction := []server.RequestRecord{
		mkRec(base.Add(10*time.Second), "/x"),
		mkRec(base.Add(11*time.Second), "/y"),
	}
	chronoIdx, listID := resolveSelection(postEviction, originalSelection)
	if chronoIdx != 1 {
		t.Errorf("chronoIdx = %d, want 1 (fallback to newest)", chronoIdx)
	}
	if listID != 0 {
		t.Errorf("listID = %d, want 0 (top of list)", listID)
	}
}
