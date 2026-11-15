package model_test

import (
	"testing"
	"time"

	"github.com/asciimoo/hister/server/model"
	"github.com/asciimoo/hister/server/testutil"
)

func TestRecordVisitDeduplicatesAndOrders(t *testing.T) {
	testutil.InitModel(t)
	const u = "https://example.com/page"
	base := time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		at   time.Time
		want bool
	}{
		{base, true},
		{base.Add(10 * time.Second), false},
		{base.Add(-10 * time.Second), false},
		{base.Add(time.Hour), true},
		{base.Add(-24 * time.Hour), true},
	} {
		got, err := model.RecordVisit(u, 1, tc.at)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("RecordVisit(%s) = %v, want %v", tc.at, got, tc.want)
		}
	}
	if ok, _ := model.RecordVisit(u, 2, base); !ok {
		t.Error("visits of another user must not be deduplicated")
	}
	visits, err := model.GetVisits(u, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(visits) != 3 {
		t.Fatalf("got %d visits, want 3", len(visits))
	}
	if !visits[0].VisitedAt.Equal(base.Add(time.Hour)) || !visits[2].VisitedAt.Equal(base.Add(-24*time.Hour)) {
		t.Errorf("visits are not ordered newest first: %v, %v", visits[0].VisitedAt, visits[2].VisitedAt)
	}
	page, _ := model.GetVisits(u, 1, 1, 1)
	if len(page) != 1 || !page[0].VisitedAt.Equal(base) {
		t.Errorf("paged visits = %+v", page)
	}

	if ok, err := model.DeleteVisit(visits[0].ID, 2); ok || err != nil {
		t.Errorf("deleting another user's visit = %v, %v", ok, err)
	}
	if ok, err := model.DeleteVisit(visits[0].ID, 1); !ok || err != nil {
		t.Errorf("DeleteVisit = %v, %v", ok, err)
	}
	if err := model.MoveVisits(u, 1, 3); err != nil {
		t.Fatal(err)
	}
	if n, _ := model.CountVisits(u, 3); n != 2 {
		t.Errorf("moved visits = %d, want 2", n)
	}
	if err := model.DeleteVisits(u, 3); err != nil {
		t.Fatal(err)
	}
	if n, _ := model.CountVisits(u, 3); n != 0 {
		t.Errorf("visits after delete = %d", n)
	}
}
