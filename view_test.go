package main

import (
	"strings"
	"testing"
	"time"
)

func TestCutToWidth_PlainASCII(t *testing.T) {
	prefix, remainder := cutToWidth("hello world", 5)
	if prefix != "hello" {
		t.Errorf("prefix = %q, want %q", prefix, "hello")
	}
	if remainder != " world" {
		t.Errorf("remainder = %q, want %q", remainder, " world")
	}
}

func TestCutToWidth_ExactFit(t *testing.T) {
	prefix, remainder := cutToWidth("hello", 5)
	if prefix != "hello" {
		t.Errorf("prefix = %q, want %q", prefix, "hello")
	}
	if remainder != "" {
		t.Errorf("remainder = %q, want %q", remainder, "")
	}
}

func TestCutToWidth_ZeroWidth(t *testing.T) {
	prefix, remainder := cutToWidth("hello", 0)
	if prefix != "" {
		t.Errorf("prefix = %q, want %q", prefix, "")
	}
	if remainder != "hello" {
		t.Errorf("remainder = %q, want %q", remainder, "hello")
	}
}

func TestCutToWidth_EmptyString(t *testing.T) {
	prefix, remainder := cutToWidth("", 5)
	if prefix != "" {
		t.Errorf("prefix = %q, want %q", prefix, "")
	}
	if remainder != "" {
		t.Errorf("remainder = %q, want %q", remainder, "")
	}
}

func TestCutToWidth_ANSIEscapesPreserved(t *testing.T) {
	// ANSI escape for red text: \033[31m
	input := "\033[31mhello\033[0m world"
	prefix, _ := cutToWidth(input, 5)
	// Should include the opening escape and all 5 visible characters
	if !strings.Contains(prefix, "\033[31m") {
		t.Errorf("prefix should contain ANSI escape, got %q", prefix)
	}
	if !strings.Contains(prefix, "hello") {
		t.Errorf("prefix should contain 'hello', got %q", prefix)
	}
}

func TestCutToWidth_ANSIDoesNotCountAsWidth(t *testing.T) {
	// The ANSI escape has zero visible width, so cutting at width 3
	// should give us the escape + "hel"
	input := "\033[31mhello"
	prefix, remainder := cutToWidth(input, 3)
	if prefix != "\033[31mhel" {
		t.Errorf("prefix = %q, want %q", prefix, "\033[31mhel")
	}
	if remainder != "lo" {
		t.Errorf("remainder = %q, want %q", remainder, "lo")
	}
}

func TestPlaceOverlay_CentredOnBase(t *testing.T) {
	base := strings.Join([]string{
		"aaaaaaaaaa",
		"aaaaaaaaaa",
		"aaaaaaaaaa",
		"aaaaaaaaaa",
	}, "\n")

	overlay := "XX"

	result := placeOverlay(4, 1, overlay, base)
	lines := strings.Split(result, "\n")

	if lines[0] != "aaaaaaaaaa" {
		t.Errorf("line 0 should be unchanged, got %q", lines[0])
	}
	// Line 1 should have overlay at position 4
	if !strings.Contains(lines[1], "XX") {
		t.Errorf("line 1 should contain overlay, got %q", lines[1])
	}
	if lines[2] != "aaaaaaaaaa" {
		t.Errorf("line 2 should be unchanged, got %q", lines[2])
	}
}

func TestPlaceOverlay_AtOrigin(t *testing.T) {
	base := "abcdef\nghijkl"
	overlay := "XY"

	result := placeOverlay(0, 0, overlay, base)
	lines := strings.Split(result, "\n")

	if !strings.HasPrefix(lines[0], "XY") {
		t.Errorf("line 0 should start with overlay, got %q", lines[0])
	}
}

func TestPlaceOverlay_BeyondBase(t *testing.T) {
	base := "abc"
	overlay := "XY\nZW"

	// Overlay starts at y=0 but extends beyond base (only 1 line)
	result := placeOverlay(0, 0, overlay, base)
	lines := strings.Split(result, "\n")

	// Should only have 1 line (base has 1 line, second overlay line is out of bounds)
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}
}

func TestRenderStatusLine_IncludesFilterSaveDirAndStatus(t *testing.T) {
	m := newReadyTestModel()
	m.filterQuery = "hello"
	m.saveDir = "/tmp/downloads/smailer"
	m.statusMessage = "Saved"

	line := m.renderStatusLine()

	if !strings.Contains(line, "Filter: hello") {
		t.Fatalf("missing filter in %q", line)
	}
	if !strings.Contains(line, "Downloads: smailer") {
		t.Fatalf("missing save dir in %q", line)
	}
	if !strings.Contains(line, "Saved") {
		t.Fatalf("missing status in %q", line)
	}
}

func TestRenderEmailView_ShowsAttachmentSummaryAndKey(t *testing.T) {
	m := newReadyTestModel()
	m.selectedEmail = &Email{
		From:        "alice@example.com",
		To:          "bob@example.com",
		Subject:     "Subject",
		Date:        time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC),
		Key:         "inbound/very/long/key/123456789",
		Attachments: []Attachment{{Name: "invoice.pdf"}},
	}

	rendered := m.renderEmailView()

	if !strings.Contains(rendered, "invoice.pdf") {
		t.Fatalf("expected attachment summary in %q", rendered)
	}
	if !strings.Contains(rendered, "...long/key/123456789") {
		t.Fatalf("expected shortened key in %q", rendered)
	}
}

func TestView_InitialisingState(t *testing.T) {
	m := model{}

	if got := m.View(); got != "Initializing...\n" {
		t.Fatalf("got %q", got)
	}
}

func TestView_BucketLoadingState(t *testing.T) {
	m := newReadyTestModel()
	m.state = bucketSelectionState
	m.loading = true

	got := m.View()

	if !strings.Contains(got, "Loading buckets") {
		t.Fatalf("got %q", got)
	}
}

func TestView_ListLoadingState(t *testing.T) {
	m := newReadyTestModel()
	m.state = listState
	m.loading = true
	m.visibleEmails = nil

	got := m.View()

	if !strings.Contains(got, "Loading emails") {
		t.Fatalf("got %q", got)
	}
}

func TestView_FilterOverlayAndDeleteModal(t *testing.T) {
	m := newReadyTestModel()
	m.state = confirmDeleteState
	m.previousState = listState
	m.filterActive = true
	m.filterInput.SetValue("alice")
	m.visibleEmails = []Email{{Key: "one"}}

	got := m.View()

	if !strings.Contains(got, "Filter:") {
		t.Fatalf("expected filter overlay in %q", got)
	}
	if !strings.Contains(got, "Delete this email?") {
		t.Fatalf("expected delete modal in %q", got)
	}
}

func TestView_EmptyListState(t *testing.T) {
	m := newReadyTestModel()
	m.state = listState
	m.loading = false

	got := m.View()

	if !strings.Contains(got, "No emails found") {
		t.Fatalf("got %q", got)
	}
}
