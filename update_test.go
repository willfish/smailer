package main

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

func TestBucketsLoadedMsg_AutoSelectSingleSES(t *testing.T) {
	m := newReadyTestModel()
	m.state = bucketSelectionState

	msg := bucketsLoadedMsg{buckets: []string{"my-ses-bucket", "other-bucket"}}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.state != listState {
		t.Errorf("state = %v, want listState (auto-select single SES bucket)", rm.state)
	}
	if rm.bucket != "my-ses-bucket" {
		t.Errorf("bucket = %q, want %q", rm.bucket, "my-ses-bucket")
	}
}

func TestBucketsLoadedMsg_NoAutoSelectMultipleSES(t *testing.T) {
	m := newReadyTestModel()
	m.state = bucketSelectionState

	msg := bucketsLoadedMsg{buckets: []string{"ses-one", "ses-two", "other"}}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.state != bucketSelectionState {
		t.Errorf("state = %v, want bucketSelectionState (multiple SES buckets)", rm.state)
	}
}

func TestBucketsLoadedMsg_NoAutoSelectNoSES(t *testing.T) {
	m := newReadyTestModel()
	m.state = bucketSelectionState

	msg := bucketsLoadedMsg{buckets: []string{"alpha", "beta"}}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.state != bucketSelectionState {
		t.Errorf("state = %v, want bucketSelectionState (no SES buckets)", rm.state)
	}
}

func TestEmailDeletedMsg_SetsPreviousState(t *testing.T) {
	m := newReadyTestModel()
	m.state = confirmDeleteState
	m.previousState = viewState
	m.emails = []Email{
		{Subject: "a", Date: time.Now(), Key: "k1"},
		{Subject: "b", Date: time.Now(), Key: "k2"},
	}
	m.selectedIndex = 0
	m.selectedEmail = &m.emails[0]

	msg := emailDeletedMsg{err: nil}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.state != viewState {
		t.Errorf("state = %v, want viewState (restored from previousState)", rm.state)
	}
}

func TestEmailDeletedMsg_SetsStatusMessage(t *testing.T) {
	m := newReadyTestModel()
	m.state = confirmDeleteState
	m.previousState = listState
	m.emails = []Email{
		{Subject: "a", Date: time.Now(), Key: "k1"},
	}
	m.selectedIndex = 0
	m.selectedEmail = &m.emails[0]

	msg := emailDeletedMsg{err: nil}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.statusMessage != "Email deleted" {
		t.Errorf("statusMessage = %q, want %q", rm.statusMessage, "Email deleted")
	}
}

func TestClearStatusMsg_ClearsMessage(t *testing.T) {
	m := newReadyTestModel()
	m.statusMessage = "Email deleted"

	msg := clearStatusMsg{}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.statusMessage != "" {
		t.Errorf("statusMessage = %q, want empty", rm.statusMessage)
	}
}

func TestEmailsLoadedMsg_AppendAndSort(t *testing.T) {
	m := newReadyTestModel()
	m.state = listState
	m.emails = []Email{
		{Subject: "old", Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	msg := emailsLoadedMsg{
		emails: []Email{
			{Subject: "newer", Date: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)},
			{Subject: "oldest", Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		hasMore:      true,
		continuation: strPtr("token"),
	}

	result, _ := m.Update(msg)
	rm := result.(model)

	if len(rm.emails) != 3 {
		t.Fatalf("expected 3 emails, got %d", len(rm.emails))
	}
	if rm.emails[0].Subject != "newer" {
		t.Errorf("emails[0] = %q, want 'newer' (most recent first)", rm.emails[0].Subject)
	}
	if rm.emails[2].Subject != "oldest" {
		t.Errorf("emails[2] = %q, want 'oldest' (least recent last)", rm.emails[2].Subject)
	}
	if !rm.hasMore {
		t.Error("hasMore should be true")
	}
	if rm.loading {
		t.Error("loading should be false after emails loaded")
	}
}

func TestEmailDeletedMsg_RemovesEmail(t *testing.T) {
	m := newReadyTestModel()
	m.state = confirmDeleteState
	m.previousState = listState
	m.emails = []Email{
		{Subject: "a", Date: time.Now(), Key: "k1"},
		{Subject: "b", Date: time.Now(), Key: "k2"},
		{Subject: "c", Date: time.Now(), Key: "k3"},
	}
	m.selectedIndex = 1
	m.selectedEmail = &m.emails[1]

	msg := emailDeletedMsg{err: nil}
	result, _ := m.Update(msg)
	rm := result.(model)

	if len(rm.emails) != 2 {
		t.Fatalf("expected 2 emails after delete, got %d", len(rm.emails))
	}
	if rm.emails[0].Subject != "a" || rm.emails[1].Subject != "c" {
		t.Errorf("wrong emails remaining: %v", rm.emails)
	}
}

func strPtr(s string) *string { return &s }

func newReadyTestModel() model {
	s := spinner.New()
	s.Spinner = spinner.Globe
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return model{
		spinner: s,
		state:   listState,
		ready:   true,
		width:   120,
		height:  40,
	}
}
