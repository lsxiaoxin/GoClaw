package todo

import (
	"strings"
	"testing"
	"time"
)

func TestItemValidationAndSummary(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	items := []Item{
		{ID: "todo-1", Content: "first", Status: StatusPending, Priority: PriorityHigh, CreatedAt: now, UpdatedAt: now},
		{ID: "todo-2", Content: "second", Status: StatusInProgress, Priority: PriorityMedium, CreatedAt: now, UpdatedAt: now},
		{ID: "todo-3", Content: "third", Status: StatusCompleted, Priority: PriorityLow, CreatedAt: now, UpdatedAt: now},
	}
	if err := ValidateList(items); err != nil {
		t.Fatalf("ValidateList() error = %v", err)
	}

	summary := Summarize(items)
	if summary.Total != 3 || summary.Pending != 1 ||
		summary.InProgress != 1 || summary.Completed != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestItemValidationRejectsInvalidStatusPriorityAndDuplicates(t *testing.T) {
	valid := Item{ID: "todo-1", Content: "first", Status: StatusPending, Priority: PriorityHigh}
	tests := []struct {
		name  string
		items []Item
		want  string
	}{
		{
			name:  "invalid status",
			items: []Item{{ID: "todo-1", Content: "first", Status: "blocked", Priority: PriorityHigh}},
			want:  "invalid status",
		},
		{
			name:  "invalid priority",
			items: []Item{{ID: "todo-1", Content: "first", Status: StatusPending, Priority: "urgent"}},
			want:  "invalid priority",
		},
		{
			name: "duplicate id",
			items: []Item{
				valid,
				{ID: "todo-1", Content: "again", Status: StatusCompleted, Priority: PriorityLow},
			},
			want: "duplicate",
		},
		{
			name: "multiple in progress",
			items: []Item{
				{ID: "todo-1", Content: "first", Status: StatusInProgress, Priority: PriorityHigh},
				{ID: "todo-2", Content: "second", Status: StatusInProgress, Priority: PriorityMedium},
			},
			want: "only one todo",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateList(test.items)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateList() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
