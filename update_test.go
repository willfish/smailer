package main

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
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
		{Subject: "old", Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Key: "k-old"},
	}

	msg := emailsLoadedMsg{
		emails: []Email{
			{Subject: "newer", Date: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), Key: "k-new"},
			{Subject: "oldest", Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Key: "k-oldest"},
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

func TestEmailsLoadedMsg_DeduplicatesByKey(t *testing.T) {
	m := newReadyTestModel()
	m.emails = []Email{
		{Subject: "existing", Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Key: "inbound/one"},
	}

	msg := emailsLoadedMsg{
		emails: []Email{
			{Subject: "replacement", Date: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Key: "inbound/one"},
			{Subject: "new", Date: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), Key: "inbound/two"},
		},
	}

	result, _ := m.Update(msg)
	rm := result.(model)

	if len(rm.emails) != 2 {
		t.Fatalf("expected 2 unique emails, got %d", len(rm.emails))
	}
	if rm.emails[1].Key != "inbound/one" {
		t.Fatalf("expected merged key to remain present, got %#v", rm.emails)
	}
	if rm.emails[1].Subject != "replacement" {
		t.Fatalf("expected duplicate key to be updated, got %q", rm.emails[1].Subject)
	}
}

func TestEmailLoadedMsg_ReplacesSummaryWithLoadedBody(t *testing.T) {
	m := newReadyTestModel()
	m.emails = []Email{{Subject: "hello", Key: "inbound/one"}}
	m.updateTableRows()

	msg := emailLoadedMsg{email: Email{
		Subject:    "hello",
		Key:        "inbound/one",
		Body:       "loaded body",
		BodyLoaded: true,
	}}

	result, _ := m.Update(msg)
	rm := result.(model)

	if !rm.emails[0].BodyLoaded {
		t.Fatal("expected loaded email to be stored")
	}
	if rm.emails[0].Body != "loaded body" {
		t.Fatalf("body = %q, want %q", rm.emails[0].Body, "loaded body")
	}
}

func TestInit_LoadsBucketsWhenSelectingBucket(t *testing.T) {
	mock := &mockS3{
		listBucketsFunc: func(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
			return &s3.ListBucketsOutput{}, nil
		},
	}
	m := initialModel(mock, "", "inbound/")

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected init command")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	if len(batch) == 0 {
		t.Fatal("expected batched commands")
	}
}

func TestFilterStatus_ReturnsClearMessage(t *testing.T) {
	if got := filterStatus("", 0); got != "Filter cleared" {
		t.Fatalf("got %q", got)
	}
	if got := filterStatus("alice", 2); got != "Filter 'alice' matched 2 email(s)" {
		t.Fatalf("got %q", got)
	}
}

func TestUpdate_BucketSelectionEnterStartsLoadingEmails(t *testing.T) {
	m := newReadyTestModel()
	m.state = bucketSelectionState
	m.bucketsList = list.New([]list.Item{item{title: "my-bucket"}}, list.NewDefaultDelegate(), 40, 10)

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm := result.(model)

	if rm.bucket != "my-bucket" {
		t.Fatalf("bucket = %q", rm.bucket)
	}
	if rm.state != listState {
		t.Fatalf("state = %v", rm.state)
	}
	if !rm.loading {
		t.Fatal("expected loading")
	}
	if cmd == nil {
		t.Fatal("expected command")
	}
}

func TestUpdate_FilterEscapeBlursWithoutChangingQuery(t *testing.T) {
	m := newReadyTestModel()
	m.filterActive = true
	m.filterQuery = "alice"
	m.filterInput.SetValue("alice")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	rm := result.(model)

	if rm.filterActive {
		t.Fatal("expected filter to close")
	}
	if rm.filterQuery != "alice" {
		t.Fatalf("query = %q", rm.filterQuery)
	}
}

func TestUpdate_FilterEnterAppliesQueryAndStatus(t *testing.T) {
	m := newReadyTestModel()
	m.filterActive = true
	m.emails = []Email{{From: "alice@example.com", Key: "one"}, {From: "bob@example.com", Key: "two"}}
	m.filterInput.SetValue("alice")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm := result.(model)

	if rm.filterActive {
		t.Fatal("expected filter to close")
	}
	if rm.filterQuery != "alice" {
		t.Fatalf("query = %q", rm.filterQuery)
	}
	if len(rm.visibleEmails) != 1 {
		t.Fatalf("visible emails = %d", len(rm.visibleEmails))
	}
	if !strings.Contains(rm.statusMessage, "matched 1") {
		t.Fatalf("status = %q", rm.statusMessage)
	}
}

func TestUpdate_ListEnterWithLoadedBodyMovesToView(t *testing.T) {
	m := newReadyTestModel()
	m.emails = []Email{{Key: "one", Subject: "hello", Body: "body", BodyLoaded: true}}
	m.updateTableRows()

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm := result.(model)

	if rm.state != viewState {
		t.Fatalf("state = %v", rm.state)
	}
	if cmd != nil {
		t.Fatal("expected no load command")
	}
	if !strings.Contains(rm.viewport.View(), "body") {
		t.Fatalf("viewport = %q", rm.viewport.View())
	}
}

func TestUpdate_ListEnterWithUnloadedBodyStartsLoad(t *testing.T) {
	raw := buildMIMEEmail("alice@example.com", "bob@example.com", "Hello", "Loaded body", time.Now())
	mock := &mockS3{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(raw))}, nil
		},
	}
	m := newMockTestModel(mock)
	m.emails = []Email{{Key: "one", Subject: "hello"}}
	m.updateTableRows()

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm := result.(model)

	if rm.state != viewState {
		t.Fatalf("state = %v", rm.state)
	}
	if cmd == nil {
		t.Fatal("expected load command")
	}
}

func TestUpdate_ListDeleteMovesToConfirmation(t *testing.T) {
	m := newReadyTestModel()
	m.emails = []Email{{Key: "one", Subject: "hello"}}
	m.updateTableRows()

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	rm := result.(model)

	if rm.state != confirmDeleteState {
		t.Fatalf("state = %v", rm.state)
	}
	if rm.previousState != listState {
		t.Fatalf("previous state = %v", rm.previousState)
	}
}

func TestUpdate_ListRefreshResetsPagination(t *testing.T) {
	m := newReadyTestModel()
	token := "next"
	m.emails = []Email{{Key: "one"}}
	m.visibleEmails = m.emails
	m.continuation = &token
	m.hasMore = false

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	rm := result.(model)

	if len(rm.emails) != 0 {
		t.Fatalf("emails = %d", len(rm.emails))
	}
	if rm.continuation != nil {
		t.Fatal("expected continuation reset")
	}
	if !rm.hasMore || !rm.loading {
		t.Fatal("expected reset loading state")
	}
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func TestUpdate_ListSaveStartsEmailSave(t *testing.T) {
	m := newReadyTestModel()
	m.saveDir = t.TempDir()
	m.emails = []Email{{Key: "one", Subject: "hello", Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), RawLoaded: true, Raw: []byte("raw")}}
	m.updateTableRows()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("expected save command")
	}
}

func TestUpdate_ViewKeysCoverBackDeleteSaveAttachment(t *testing.T) {
	m := newReadyTestModel()
	m.state = viewState
	m.selectedEmail = &Email{Key: "one", Subject: "hello", Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), RawLoaded: true, Raw: []byte("raw"), BodyLoaded: true}
	m.saveDir = t.TempDir()

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if result.(model).state != listState {
		t.Fatal("esc should return to list")
	}

	m.state = viewState
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	rm := result.(model)
	if rm.state != confirmDeleteState || rm.previousState != viewState {
		t.Fatal("d should open confirm from view")
	}

	m.state = viewState
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}); cmd == nil {
		t.Fatal("s should trigger save")
	}

	m.state = viewState
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}); cmd == nil {
		t.Fatal("a should trigger attachment save")
	}
}

func TestUpdate_MessageHandlersSetStatuses(t *testing.T) {
	m := newReadyTestModel()
	m.selectedEmail = &Email{Key: "one"}
	m.previousState = listState
	m.state = confirmDeleteState

	result, _ := m.Update(emailSavedMsg{path: "/tmp/test.eml"})
	if !strings.Contains(result.(model).statusMessage, "Saved .eml") {
		t.Fatal("expected save status")
	}

	result, _ = m.Update(attachmentsSavedMsg{})
	if result.(model).statusMessage != "No attachments to save" {
		t.Fatal("expected no attachments status")
	}

	result, _ = m.Update(attachmentsSavedMsg{paths: []string{"a", "b"}})
	if !strings.Contains(result.(model).statusMessage, "Saved 2 attachment") {
		t.Fatal("expected attachment count status")
	}

	result, _ = m.Update(errorMsg{err: context.DeadlineExceeded})
	if !strings.Contains(result.(model).statusMessage, "Error:") {
		t.Fatal("expected error status")
	}
}

func TestInit_LoadsEmailsWhenBucketAlreadySelected(t *testing.T) {
	mock := &mockS3{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{}, nil
		},
	}
	m := initialModel(mock, "bucket", "inbound/")

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected init command")
	}
}

func TestUpdate_BucketSelectionQuitReturnsQuitCmd(t *testing.T) {
	m := newReadyTestModel()
	m.state = bucketSelectionState

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if result.(model).state != bucketSelectionState {
		t.Fatal("unexpected state change")
	}
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestUpdate_ConfirmDeleteCancelRestoresPreviousState(t *testing.T) {
	m := newReadyTestModel()
	m.state = confirmDeleteState
	m.previousState = listState

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if result.(model).state != listState {
		t.Fatal("expected previous state restoration")
	}
}

func TestUpdate_BucketsLoadedWithoutSingleSESKeepsSelectionState(t *testing.T) {
	m := newReadyTestModel()
	m.state = bucketSelectionState

	result, _ := m.Update(bucketsLoadedMsg{buckets: []string{"one", "two"}})
	rm := result.(model)
	if rm.state != bucketSelectionState {
		t.Fatal("expected bucket selection state")
	}
	if rm.loading {
		t.Fatal("expected loading false")
	}
}

func TestUpdate_EmailsLoadedSetsSkippedStatus(t *testing.T) {
	m := newReadyTestModel()

	result, _ := m.Update(emailsLoadedMsg{emails: []Email{{Key: "one", Date: time.Now()}}, skipped: 2})
	if !strings.Contains(result.(model).statusMessage, "Skipped 2") {
		t.Fatal("expected skipped status")
	}
}

func TestUpdate_ClearStatusMsgClearsMessage(t *testing.T) {
	m := newReadyTestModel()
	m.statusMessage = "hello"

	result, _ := m.Update(clearStatusMsg{})
	if result.(model).statusMessage != "" {
		t.Fatal("expected cleared status")
	}
}

func TestUpdate_SpinnerTickReturnsCommand(t *testing.T) {
	m := newReadyTestModel()

	_, cmd := m.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Fatal("expected spinner command")
	}
}

func TestReplaceEmail_AppendsWhenMissingAndFindEmailHandlesMissing(t *testing.T) {
	m := newReadyTestModel()

	m.replaceEmail(Email{Key: "one", Subject: "hello"})
	if len(m.emails) != 1 {
		t.Fatalf("emails = %d", len(m.emails))
	}
	if m.findEmailByKey("missing") != nil {
		t.Fatal("expected nil for missing email")
	}
}

func TestMergeEmailData_OverlaysLoadedFields(t *testing.T) {
	current := Email{From: "old", Key: "one"}
	incoming := Email{
		From:        "new",
		To:          "to",
		Subject:     "subject",
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		S3Date:      time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		Size:        12,
		Body:        "body",
		BodyLoaded:  true,
		Raw:         []byte("raw"),
		RawLoaded:   true,
		Attachments: []Attachment{{Name: "a.txt"}},
	}

	merged := mergeEmailData(current, incoming)

	if merged.From != "new" || merged.To != "to" || merged.Subject != "subject" {
		t.Fatalf("merged headers = %#v", merged)
	}
	if !merged.BodyLoaded || !merged.RawLoaded || merged.Size != 12 || len(merged.Attachments) != 1 {
		t.Fatalf("merged payload = %#v", merged)
	}
}

func TestUpdate_FilterTypingUpdatesQuery(t *testing.T) {
	m := newReadyTestModel()
	m.filterActive = true
	m.filterInput.Focus()

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if result.(model).filterQuery != "a" {
		t.Fatalf("query = %q", result.(model).filterQuery)
	}
}

func TestUpdate_ListEscapeReturnsToBucketSelection(t *testing.T) {
	m := newReadyTestModel()
	m.emails = []Email{{Key: "one"}}
	m.visibleEmails = m.emails
	token := "next"
	m.continuation = &token
	m.filterQuery = "alice"
	m.filterInput.SetValue("alice")

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	rm := result.(model)
	if rm.state != bucketSelectionState {
		t.Fatalf("state = %v", rm.state)
	}
	if len(rm.emails) != 0 || rm.continuation != nil || rm.filterQuery != "" {
		t.Fatal("expected list state reset")
	}
	if cmd == nil {
		t.Fatal("expected reload buckets command")
	}
}

func TestUpdate_ListSlashActivatesFilter(t *testing.T) {
	m := newReadyTestModel()
	m.filterQuery = "bob"

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	rm := result.(model)
	if !rm.filterActive {
		t.Fatal("expected active filter")
	}
	if rm.filterInput.Value() != "bob" {
		t.Fatalf("value = %q", rm.filterInput.Value())
	}
}

func TestUpdate_ListDownLoadsMoreAtBottom(t *testing.T) {
	m := newReadyTestModel()
	m.emails = []Email{{Key: "one"}, {Key: "two"}}
	m.updateTableRows()
	m.table.SetCursor(0)
	m.hasMore = true
	m.loading = false

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatal("expected load-more command batch")
	}
}

func TestUpdate_ViewDefaultDelegatesToViewport(t *testing.T) {
	m := newReadyTestModel()
	m.state = viewState
	m.viewport.SetContent("line1\nline2\nline3")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if result.(model).state != viewState {
		t.Fatal("expected to remain in view state")
	}
}

func TestUpdate_WindowSizeInitialisesComponents(t *testing.T) {
	m := model{}

	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	rm := result.(model)
	if !rm.ready {
		t.Fatal("expected ready")
	}
	if rm.width != 100 || rm.height != 30 {
		t.Fatal("expected dimensions set")
	}
}

func TestUpdate_EmailLoadedMsgSetsViewportContent(t *testing.T) {
	m := newReadyTestModel()
	m.emails = []Email{{Key: "one", Subject: "a"}}

	result, _ := m.Update(emailLoadedMsg{email: Email{Key: "one", Subject: "a", Body: "loaded", BodyLoaded: true}})
	if !strings.Contains(result.(model).viewport.View(), "loaded") {
		t.Fatalf("viewport = %q", result.(model).viewport.View())
	}
}

func TestUpdate_EmailDeletedErrorSetsStatus(t *testing.T) {
	m := newReadyTestModel()

	result, _ := m.Update(emailDeletedMsg{err: context.Canceled})
	if !strings.Contains(result.(model).statusMessage, "Delete failed") {
		t.Fatal("expected delete failure status")
	}
}

func TestUpdate_EmailDeletedSuccessRemovesSelectedEmail(t *testing.T) {
	m := newReadyTestModel()
	m.state = confirmDeleteState
	m.previousState = listState
	m.emails = []Email{{Key: "one"}, {Key: "two"}}
	m.updateTableRows()
	m.selectedEmail = &Email{Key: "one"}

	result, _ := m.Update(emailDeletedMsg{})
	rm := result.(model)
	if len(rm.emails) != 1 || rm.emails[0].Key != "two" {
		t.Fatalf("emails = %#v", rm.emails)
	}
	if rm.state != listState {
		t.Fatalf("state = %v", rm.state)
	}
}

func TestUpdate_EmailAndAttachmentErrorStatuses(t *testing.T) {
	m := newReadyTestModel()

	result, _ := m.Update(emailSavedMsg{err: context.DeadlineExceeded})
	if !strings.Contains(result.(model).statusMessage, "Save failed") {
		t.Fatal("expected email save failure")
	}

	result, _ = m.Update(attachmentsSavedMsg{err: context.DeadlineExceeded})
	if !strings.Contains(result.(model).statusMessage, "Attachment save failed") {
		t.Fatal("expected attachment save failure")
	}
}

func strPtr(s string) *string { return &s }

func newReadyTestModel() model {
	s := spinner.New()
	s.Spinner = spinner.Globe
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	m := model{
		spinner: s,
		state:   listState,
		ready:   true,
		width:   120,
		height:  40,
		saveDir: "/tmp/smailer-tests",
	}
	m.initComponents()
	return m
}
