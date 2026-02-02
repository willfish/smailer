package main

import (
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
	if !strings.Contains(help, "esc: back") {
		t.Errorf("expected 'esc: back' in help, got %q", help)
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
