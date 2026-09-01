package version

import "testing"

// TestStringSemCommit retorna apenas a versão quando não há commit associado.
func TestStringSemCommit(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	defer func() { Version, Commit, Date = oldVersion, oldCommit, oldDate }()

	Version, Commit, Date = "1.2.3", "", ""
	if got := String(); got != "1.2.3" {
		t.Errorf("String() = %q, esperado %q", got, "1.2.3")
	}
}

// TestStringComCommit inclui o hash do commit na saída.
func TestStringComCommit(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	defer func() { Version, Commit, Date = oldVersion, oldCommit, oldDate }()

	Version, Commit, Date = "1.2.3", "abc1234", ""
	if got, want := String(), "1.2.3 (abc1234)"; got != want {
		t.Errorf("String() = %q, esperado %q", got, want)
	}
}

// TestStringComCommitEData inclui hash e data quando ambos presentes.
func TestStringComCommitEData(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	defer func() { Version, Commit, Date = oldVersion, oldCommit, oldDate }()

	Version, Commit, Date = "1.2.3", "abc1234", "2026-01-01T00:00:00Z"
	if got, want := String(), "1.2.3 (abc1234, 2026-01-01T00:00:00Z)"; got != want {
		t.Errorf("String() = %q, esperado %q", got, want)
	}
}
