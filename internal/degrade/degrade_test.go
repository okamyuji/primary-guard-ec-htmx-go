package degrade

import (
	"testing"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
)

// TestActiveSwitches Trip と Reset で Active の戻り値が変わる
func TestActiveSwitches(t *testing.T) {
	t.Parallel()

	s := dbx.NewReplicaState()
	if Active(s) {
		t.Fatal("initial want false")
	}
	s.Trip()
	if !Active(s) {
		t.Fatal("trip want true")
	}
	if ReportEnabled(s) {
		t.Fatal("trip ReportEnabled want false")
	}
	if SuggestEnabled(s) {
		t.Fatal("trip SuggestEnabled want false")
	}
	s.Reset()
	if Active(s) {
		t.Fatal("reset want false")
	}
	if !ReportEnabled(s) {
		t.Fatal("reset ReportEnabled want true")
	}
}

// TestNilStateSafe nil state でパニックしない
func TestNilStateSafe(t *testing.T) {
	t.Parallel()
	if Active(nil) {
		t.Fatal("nil want false")
	}
	if !ReportEnabled(nil) {
		t.Fatal("nil ReportEnabled want true")
	}
}
