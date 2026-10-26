package replace

import (
	"reflect"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	// 0         1         2         3         4
	// 0123456789012345678901234567890123456789012345
	// DB=mysql://app:hunter2@db.example.com/shop ok
	const s = "DB=mysql://app:hunter2@db.example.com/shop ok"

	dsn := Match{Start: 3, End: 42, Label: "DSN", Priority: 5}
	password := Match{Start: 15, End: 22, Label: "PASSWORD", Priority: 1}
	host := Match{Start: 23, End: 37, Label: "HOST", Priority: 9}
	ok := Match{Start: 43, End: 45, Label: "WORD", Priority: 9}

	tests := []struct {
		name       string
		candidates []Match
		want       []Match
	}{
		{"none", nil, nil},
		{"one", []Match{password}, []Match{password}},
		{"longest span wins", []Match{password, dsn, host}, []Match{dsn}},
		{"non-overlapping all kept, in position order", []Match{host, password}, []Match{password, host}},
		{"kept alongside a longer match elsewhere", []Match{ok, dsn}, []Match{dsn, ok}},
		{
			"equal spans: lower priority wins",
			[]Match{{Start: 15, End: 22, Label: "B", Priority: 2}, {Start: 15, End: 22, Label: "A", Priority: 1}},
			[]Match{{Start: 15, End: 22, Label: "A", Priority: 1}},
		},
		{
			"equal length and priority: earlier start wins",
			[]Match{{Start: 16, End: 23, Label: "X"}, {Start: 15, End: 22, Label: "X"}},
			[]Match{{Start: 15, End: 22, Label: "X"}},
		},
		{
			"touching spans do not overlap",
			[]Match{{Start: 4, End: 8, Label: "B"}, {Start: 0, End: 4, Label: "A"}},
			[]Match{{Start: 0, End: 4, Label: "A"}, {Start: 4, End: 8, Label: "B"}},
		},
		{
			"empty and out-of-range spans dropped",
			[]Match{{Start: 5, End: 5}, {Start: -1, End: 3}, {Start: 40, End: 99}, {Start: 9, End: 2}, password},
			[]Match{password},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Resolve(s, tt.candidates); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplaceUsesResolvedMatches(t *testing.T) {
	t.Parallel()

	const s = "DB=mysql://app:hunter2@db.example.com/shop ok"
	matches := []Match{
		{Start: 15, End: 22, Label: "PASSWORD", Priority: 1},
		{Start: 3, End: 42, Label: "DSN", Priority: 5},
	}

	want := "DB=<DSN_1> ok"
	if got := newPlaceholders(t, DefaultFormat).Replace(s, matches); got != want {
		t.Errorf("Replace() = %q, want %q", got, want)
	}
}
