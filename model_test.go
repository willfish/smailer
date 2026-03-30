package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

func TestInitialModel_BucketEmpty_SelectionState(t *testing.T) {
	m := initialModel(&s3.Client{}, "", "inbound/")
	if m.state != bucketSelectionState {
		t.Errorf("state = %v, want bucketSelectionState", m.state)
	}
}

func TestInitialModel_BucketSet_ListState(t *testing.T) {
	m := initialModel(&s3.Client{}, "my-bucket", "inbound/")
	if m.state != listState {
		t.Errorf("state = %v, want listState", m.state)
	}
	if m.bucket != "my-bucket" {
		t.Errorf("bucket = %q, want %q", m.bucket, "my-bucket")
	}
}

func TestInitialModel_HasMoreTrue(t *testing.T) {
	m := initialModel(&s3.Client{}, "b", "p/")
	if !m.hasMore {
		t.Error("hasMore should be true initially")
	}
}

func TestInitialModel_LoadingTrue(t *testing.T) {
	m := initialModel(&s3.Client{}, "b", "p/")
	if !m.loading {
		t.Error("loading should be true initially")
	}
}

func TestPrefixNormalisation(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"inbound", "inbound/"},
		{"inbound/", "inbound/"},
		{"inbound//", "inbound/"},
		{"foo/bar", "foo/bar/"},
		{"foo/bar/", "foo/bar/"},
		{"/", "/"},
	}

	for _, tt := range tests {
		got := strings.TrimRight(tt.input, "/") + "/"
		if got != tt.want {
			t.Errorf("normalise(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRenderListHelp_EmptyEmails(t *testing.T) {
	m := newTestModel()
	m.emails = nil
	m.hasMore = false
	m.loading = false
	m.statusMessage = ""

	help := m.renderListHelp()
	if !strings.Contains(help, "0 emails") {
		t.Errorf("expected '0 emails' in help, got %q", help)
	}
}

func TestRenderListHelp_WithEmails(t *testing.T) {
	m := newTestModel()
	m.emails = []Email{
		{Subject: "a", Date: time.Now()},
		{Subject: "b", Date: time.Now()},
		{Subject: "c", Date: time.Now()},
	}
	m.hasMore = false
	m.loading = false

	help := m.renderListHelp()
	if !strings.Contains(help, "3 emails") {
		t.Errorf("expected '3 emails' in help, got %q", help)
	}
	if strings.Contains(help, "more available") {
		t.Error("should not show 'more available' when hasMore is false")
	}
}

func TestRenderListHelp_MoreAvailable(t *testing.T) {
	m := newTestModel()
	m.emails = []Email{{Subject: "a", Date: time.Now()}}
	m.hasMore = true
	m.loading = false

	help := m.renderListHelp()
	if !strings.Contains(help, "more available") {
		t.Errorf("expected 'more available' in help, got %q", help)
	}
}

func TestRenderListHelp_Loading(t *testing.T) {
	m := newTestModel()
	m.emails = []Email{{Subject: "a", Date: time.Now()}}
	m.loading = true

	help := m.renderListHelp()
	if !strings.Contains(help, "loading more") {
		t.Errorf("expected 'loading more' in help, got %q", help)
	}
}

func TestRenderListHelp_StatusMessage(t *testing.T) {
	m := newTestModel()
	m.emails = nil
	m.loading = false
	m.statusMessage = "Email deleted"

	help := m.renderListHelp()
	if !strings.Contains(help, "Email deleted") {
		t.Errorf("expected 'Email deleted' in help, got %q", help)
	}
}

func TestRenderListHelp_RefreshKeybinding(t *testing.T) {
	m := newTestModel()
	m.emails = nil
	m.loading = false

	help := m.renderListHelp()
	if !strings.Contains(help, "r: refresh") {
		t.Errorf("expected 'r: refresh' in help, got %q", help)
	}
}

func TestRenderListHelp_EscBackKeybinding(t *testing.T) {
	m := newTestModel()
	m.emails = nil
	m.loading = false

	help := m.renderListHelp()
	if !strings.Contains(help, "esc: buckets") {
		t.Errorf("expected 'esc: buckets' in help, got %q", help)
	}
}

func TestFilteredEmails_MatchesQueryAcrossFields(t *testing.T) {
	m := newTestModel()
	m.emails = []Email{
		{From: "alice@example.com", Subject: "Alpha", Key: "inbound/one"},
		{To: "bob@example.com", Subject: "Beta", Key: "inbound/two"},
	}
	m.filterQuery = "bob"

	filtered := m.filteredEmails()

	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered email, got %d", len(filtered))
	}
	if filtered[0].Key != "inbound/two" {
		t.Fatalf("filtered key = %q", filtered[0].Key)
	}
}

func TestItemMethods_ReturnTitleAndFilterValue(t *testing.T) {
	i := item{title: "bucket-name"}

	if i.Title() != "bucket-name" {
		t.Fatalf("title = %q", i.Title())
	}
	if i.Description() != "" {
		t.Fatalf("description = %q", i.Description())
	}
	if i.FilterValue() != "bucket-name" {
		t.Fatalf("filter value = %q", i.FilterValue())
	}
}

func TestUpdateComponents_SetsTableAndFilterWidths(t *testing.T) {
	m := newReadyTestModel()
	m.width = 100
	m.height = 30

	m.updateComponents()

	if m.table.Width() != 96 {
		t.Fatalf("table width = %d, want %d", m.table.Width(), 96)
	}
	if m.filterInput.Width <= 0 {
		t.Fatalf("expected positive filter width, got %d", m.filterInput.Width)
	}
}

func TestUpdateComponents_ScalesColumnsForSmallWidths(t *testing.T) {
	m := newReadyTestModel()
	m.width = 40
	m.height = 20

	m.updateComponents()

	for _, col := range m.table.Columns() {
		if col.Width < 1 {
			t.Fatalf("column width too small: %#v", m.table.Columns())
		}
	}
}

func TestDefaultSaveDir_UsesDownloadsFolder(t *testing.T) {
	got := defaultSaveDir()

	if !strings.Contains(got, "Downloads") || !strings.Contains(got, "smailer") {
		t.Fatalf("got %q", got)
	}
}

func TestDefaultSaveDir_FallsBackWhenHomeUnset(t *testing.T) {
	original, hadHome := os.LookupEnv("HOME")
	if hadHome {
		defer os.Setenv("HOME", original)
	} else {
		defer os.Unsetenv("HOME")
	}
	os.Unsetenv("HOME")

	got := defaultSaveDir()

	if got != "Downloads/smailer" {
		t.Fatalf("got %q", got)
	}
}

func newTestModel() model {
	s := spinner.New()
	s.Spinner = spinner.Globe
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return model{
		spinner: s,
		state:   listState,
	}
}
