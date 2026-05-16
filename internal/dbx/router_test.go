package dbx

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// TestReplicaStateTripReset Trip と Reset の往復を確認する
func TestReplicaStateTripReset(t *testing.T) {
	t.Parallel()

	s := NewReplicaState()
	if s.Down() {
		t.Fatal("initial want up")
	}
	s.Trip()
	if !s.Down() {
		t.Fatal("trip want down")
	}
	s.Reset()
	if s.Down() {
		t.Fatal("reset want up")
	}
}

// TestForcePrimaryWindow 5 秒以内は true、それ以降は false
func TestForcePrimaryWindow(t *testing.T) {
	t.Parallel()

	ctx := WithReadAfterWrite(context.Background(), 50*time.Millisecond)
	if !ForcePrimary(ctx) {
		t.Fatal("within window want true")
	}
	time.Sleep(80 * time.Millisecond)
	if ForcePrimary(ctx) {
		t.Fatal("after window want false")
	}
}

// TestForcePrimaryNoWindow window 無しなら false
func TestForcePrimaryNoWindow(t *testing.T) {
	t.Parallel()

	if ForcePrimary(context.Background()) {
		t.Fatal("no window want false")
	}
}

// TestRouterPicksReplicaWhenNeither force-primary でなく Replica も up なら Replica
func TestRouterPicksReplicaWhenNeither(t *testing.T) {
	t.Parallel()

	primary := &sql.DB{}
	replica := &sql.DB{}
	r := New(primary, replica, NewReplicaState())

	if r.Reader(context.Background()) != replica {
		t.Fatal("want replica")
	}
	if r.Writer(context.Background()) != primary {
		t.Fatal("writer want primary")
	}
}

// TestRouterPicksPrimaryOnForce read-after-write 中は Primary
func TestRouterPicksPrimaryOnForce(t *testing.T) {
	t.Parallel()

	primary := &sql.DB{}
	replica := &sql.DB{}
	r := New(primary, replica, NewReplicaState())

	ctx := WithReadAfterWrite(context.Background(), 100*time.Millisecond)
	if r.Reader(ctx) != primary {
		t.Fatal("RAW window want primary")
	}
}

// TestRouterPicksPrimaryWhenReplicaDown Replica down のときは Primary
func TestRouterPicksPrimaryWhenReplicaDown(t *testing.T) {
	t.Parallel()

	primary := &sql.DB{}
	replica := &sql.DB{}
	state := NewReplicaState()
	state.Trip()
	r := New(primary, replica, state)

	if r.Reader(context.Background()) != primary {
		t.Fatal("replica down want primary")
	}
}

// TestPlaceholders 各 n に対する出力を確認する
func TestPlaceholders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		n    int
		want string
	}{
		{0, ""},
		{1, "?"},
		{2, "?,?"},
		{3, "?,?,?"},
	}
	for _, c := range cases {
		if got := Placeholders(c.n); got != c.want {
			t.Errorf("Placeholders(%d) got %q want %q", c.n, got, c.want)
		}
	}
}
