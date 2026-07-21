package pkgmanager_test

import (
	"testing"

	"abstrax/internal/services/pkgmanager"
)

func TestNewForFamily(t *testing.T) {
	apt, err := pkgmanager.NewForFamily("debian", false, false)
	if err != nil {
		t.Fatalf("debian: %v", err)
	}
	if _, ok := apt.(*pkgmanager.AptManager); !ok {
		t.Fatalf("expected *AptManager, got %T", apt)
	}

	dnf, err := pkgmanager.NewForFamily("rhel", false, false)
	if err != nil {
		t.Fatalf("rhel: %v", err)
	}
	if _, ok := dnf.(*pkgmanager.DnfManager); !ok {
		t.Fatalf("expected *DnfManager, got %T", dnf)
	}

	if _, err := pkgmanager.NewForFamily("unknown", false, false); err == nil {
		t.Fatal("expected error for unknown family")
	}
}
