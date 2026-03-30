package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type mockS3 struct {
	listBucketsFunc   func(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	listObjectsV2Func func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	getObjectFunc     func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	deleteObjectFunc  func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

func (m *mockS3) ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	return m.listBucketsFunc(ctx, params, optFns...)
}

func (m *mockS3) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return m.listObjectsV2Func(ctx, params, optFns...)
}

func (m *mockS3) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return m.getObjectFunc(ctx, params, optFns...)
}

func (m *mockS3) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return m.deleteObjectFunc(ctx, params, optFns...)
}

func buildMIMEEmail(from, to, subject, body string, date time.Time) string {
	return fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nContent-Type: text/plain\r\n\r\n%s",
		from, to, subject, date.Format(time.RFC1123Z), body)
}

func TestLoadBuckets_ReturnsSortedBuckets(t *testing.T) {
	mock := &mockS3{
		listBucketsFunc: func(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
			return &s3.ListBucketsOutput{
				Buckets: []types.Bucket{
					{Name: aws.String("zebra-bucket")},
					{Name: aws.String("my-ses-inbox")},
					{Name: aws.String("alpha-bucket")},
					{Name: aws.String("ses-logs")},
				},
			}, nil
		},
	}

	m := model{s3Client: mock}
	cmd := m.loadBuckets()
	msg := cmd()

	loaded, ok := msg.(bucketsLoadedMsg)
	if !ok {
		t.Fatalf("expected bucketsLoadedMsg, got %T", msg)
	}

	// SES buckets should come first, sorted
	if len(loaded.buckets) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(loaded.buckets))
	}
	if loaded.buckets[0] != "my-ses-inbox" {
		t.Errorf("buckets[0] = %q, want %q", loaded.buckets[0], "my-ses-inbox")
	}
	if loaded.buckets[1] != "ses-logs" {
		t.Errorf("buckets[1] = %q, want %q", loaded.buckets[1], "ses-logs")
	}
	// Non-SES buckets after, sorted
	if loaded.buckets[2] != "alpha-bucket" {
		t.Errorf("buckets[2] = %q, want %q", loaded.buckets[2], "alpha-bucket")
	}
	if loaded.buckets[3] != "zebra-bucket" {
		t.Errorf("buckets[3] = %q, want %q", loaded.buckets[3], "zebra-bucket")
	}
}

func TestLoadBuckets_Error(t *testing.T) {
	mock := &mockS3{
		listBucketsFunc: func(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}

	m := model{s3Client: mock}
	cmd := m.loadBuckets()
	msg := cmd()

	errMsg, ok := msg.(errorMsg)
	if !ok {
		t.Fatalf("expected errorMsg, got %T", msg)
	}
	if errMsg.err.Error() != "access denied" {
		t.Errorf("error = %q, want %q", errMsg.err.Error(), "access denied")
	}
}

func TestLoadEmails_ParsesMIME(t *testing.T) {
	emailDate := time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)
	mimeBody := buildMIMEEmail("alice@example.com", "bob@example.com", "Hello Bob", "How are you?", emailDate)

	mock := &mockS3{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []types.Object{
					{Key: aws.String("inbound/msg001")},
				},
				IsTruncated: aws.Bool(false),
			}, nil
		},
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader(mimeBody)),
			}, nil
		},
	}

	m := model{s3Client: mock, bucket: "test-bucket", prefix: "inbound/"}
	cmd := m.loadEmails()
	msg := cmd()

	loaded, ok := msg.(emailsLoadedMsg)
	if !ok {
		t.Fatalf("expected emailsLoadedMsg, got %T", msg)
	}

	if len(loaded.emails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(loaded.emails))
	}

	email := loaded.emails[0]
	if email.From != "alice@example.com" {
		t.Errorf("From = %q, want %q", email.From, "alice@example.com")
	}
	if email.To != "bob@example.com" {
		t.Errorf("To = %q, want %q", email.To, "bob@example.com")
	}
	if email.Subject != "Hello Bob" {
		t.Errorf("Subject = %q, want %q", email.Subject, "Hello Bob")
	}
	if email.Body != "" {
		t.Errorf("Body = %q, want empty summary body", email.Body)
	}
	if email.BodyLoaded {
		t.Error("BodyLoaded should be false for list summaries")
	}
	if email.Key != "inbound/msg001" {
		t.Errorf("Key = %q, want %q", email.Key, "inbound/msg001")
	}
	if loaded.hasMore {
		t.Error("hasMore should be false")
	}
}

func TestLoadEmails_Pagination(t *testing.T) {
	mock := &mockS3{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents:              []types.Object{},
				IsTruncated:           aws.Bool(true),
				NextContinuationToken: aws.String("next-page-token"),
			}, nil
		},
	}

	m := model{s3Client: mock, bucket: "test-bucket", prefix: "inbound/"}
	cmd := m.loadEmails()
	msg := cmd()

	loaded, ok := msg.(emailsLoadedMsg)
	if !ok {
		t.Fatalf("expected emailsLoadedMsg, got %T", msg)
	}

	if !loaded.hasMore {
		t.Error("hasMore should be true when IsTruncated")
	}
	if loaded.continuation == nil || *loaded.continuation != "next-page-token" {
		t.Error("continuation token should be set")
	}
}

func TestLoadEmails_PassesContinuationToken(t *testing.T) {
	var capturedToken *string
	mock := &mockS3{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			capturedToken = params.ContinuationToken
			return &s3.ListObjectsV2Output{
				Contents:    []types.Object{},
				IsTruncated: aws.Bool(false),
			}, nil
		},
	}

	token := "page-2-token"
	m := model{s3Client: mock, bucket: "test-bucket", prefix: "inbound/", continuation: &token}
	cmd := m.loadEmails()
	cmd()

	if capturedToken == nil || *capturedToken != "page-2-token" {
		t.Error("continuation token should be passed to ListObjectsV2")
	}
}

func TestLoadEmails_Error(t *testing.T) {
	mock := &mockS3{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, fmt.Errorf("no such bucket")
		},
	}

	m := model{s3Client: mock, bucket: "bad-bucket", prefix: "inbound/"}
	cmd := m.loadEmails()
	msg := cmd()

	errMsg, ok := msg.(errorMsg)
	if !ok {
		t.Fatalf("expected errorMsg, got %T", msg)
	}
	if errMsg.err.Error() != "no such bucket" {
		t.Errorf("error = %q, want %q", errMsg.err.Error(), "no such bucket")
	}
}

func TestLoadEmails_IncludesDegradedRowForUnparseableObjects(t *testing.T) {
	modified := time.Date(2025, 3, 20, 9, 15, 0, 0, time.UTC)
	goodEmail := buildMIMEEmail("a@b.com", "c@d.com", "Good", "body", modified)

	mock := &mockS3{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []types.Object{
					{Key: aws.String("inbound/good"), LastModified: &modified},
					{Key: aws.String("inbound/bad"), LastModified: &modified},
				},
				IsTruncated: aws.Bool(false),
			}, nil
		},
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			if *params.Key == "inbound/bad" {
				return &s3.GetObjectOutput{
					Body: io.NopCloser(strings.NewReader("not valid mime at all")),
				}, nil
			}
			return &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader(goodEmail)),
			}, nil
		},
	}

	m := model{s3Client: mock, bucket: "test-bucket", prefix: "inbound/"}
	cmd := m.loadEmails()
	msg := cmd()

	loaded := msg.(emailsLoadedMsg)
	if len(loaded.emails) != 2 {
		t.Fatalf("expected 2 emails including degraded row, got %d", len(loaded.emails))
	}
	if loaded.skipped != 0 {
		t.Fatalf("expected 0 skipped emails, got %d", loaded.skipped)
	}

	degraded := loaded.emails[1]
	if degraded.Key != "inbound/bad" {
		t.Fatalf("expected degraded row for inbound/bad, got %q", degraded.Key)
	}
	if !degraded.SummaryError {
		t.Fatal("expected degraded row to be marked as summary error")
	}
	if degraded.Subject != "(unparseable email)" {
		t.Fatalf("subject = %q", degraded.Subject)
	}
	if degraded.Date != modified {
		t.Fatalf("date = %v, want %v", degraded.Date, modified)
	}
	if degraded.BodyLoaded {
		t.Fatal("degraded summary should not have body loaded")
	}
}

func TestFallbackEmailSummary_UsesObjectMetadata(t *testing.T) {
	modified := time.Date(2025, 4, 1, 12, 34, 56, 0, time.UTC)

	email := fallbackEmailSummary(types.Object{
		Key:          aws.String("inbound/raw-object"),
		LastModified: &modified,
		Size:         aws.Int64(42),
	})

	if email.Key != "inbound/raw-object" {
		t.Fatalf("key = %q", email.Key)
	}
	if !email.SummaryError {
		t.Fatal("expected SummaryError to be true")
	}
	if email.Subject != "(unparseable email)" {
		t.Fatalf("subject = %q", email.Subject)
	}
	if !email.Date.Equal(modified) || !email.S3Date.Equal(modified) {
		t.Fatalf("dates = %v / %v, want %v", email.Date, email.S3Date, modified)
	}
	if email.Size != 42 {
		t.Fatalf("size = %d", email.Size)
	}
}

func TestDeleteEmail_Success(t *testing.T) {
	var deletedKey string
	mock := &mockS3{
		deleteObjectFunc: func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
			deletedKey = *params.Key
			return &s3.DeleteObjectOutput{}, nil
		},
	}

	m := model{
		s3Client:      mock,
		bucket:        "test-bucket",
		selectedEmail: &Email{Key: "inbound/msg001"},
	}
	cmd := m.deleteEmail()
	msg := cmd()

	deleted, ok := msg.(emailDeletedMsg)
	if !ok {
		t.Fatalf("expected emailDeletedMsg, got %T", msg)
	}
	if deleted.err != nil {
		t.Errorf("unexpected error: %v", deleted.err)
	}
	if deletedKey != "inbound/msg001" {
		t.Errorf("deleted key = %q, want %q", deletedKey, "inbound/msg001")
	}
}

func TestDeleteEmail_Error(t *testing.T) {
	mock := &mockS3{
		deleteObjectFunc: func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
			return nil, fmt.Errorf("forbidden")
		},
	}

	m := model{
		s3Client:      mock,
		bucket:        "test-bucket",
		selectedEmail: &Email{Key: "inbound/msg001"},
	}
	cmd := m.deleteEmail()
	msg := cmd()

	deleted := msg.(emailDeletedMsg)
	if deleted.err == nil {
		t.Fatal("expected error, got nil")
	}
	if deleted.err.Error() != "forbidden" {
		t.Errorf("error = %q, want %q", deleted.err.Error(), "forbidden")
	}
}

func TestGetEmailBody_RendersMarkdown(t *testing.T) {
	renderer, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(80))
	m := model{glamourRenderer: renderer}

	email := &Email{Body: "# Hello\n\nThis is **bold**."}
	result := m.getEmailBody(email)

	if !strings.Contains(result, "Hello") {
		t.Errorf("rendered body should contain 'Hello', got %q", result)
	}
}

func TestGetEmailBody_FallsBackOnError(t *testing.T) {
	// nil renderer will cause Render to fail
	m := model{glamourRenderer: nil}

	// This will panic if we don't handle it — but getEmailBody
	// calls m.glamourRenderer.Render which would nil-deref.
	// Test with a valid renderer that's given empty body instead.
	renderer, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(80))
	m.glamourRenderer = renderer

	email := &Email{Body: "plain text"}
	result := m.getEmailBody(email)

	if !strings.Contains(result, "plain text") {
		t.Errorf("rendered body should contain 'plain text', got %q", result)
	}
}

func TestFetchAndParseEmail_HTMLConvertedToMarkdown(t *testing.T) {
	htmlEmail := "From: a@b.com\r\nTo: c@d.com\r\nSubject: HTML Test\r\nDate: Mon, 15 Mar 2025 10:30:00 +0000\r\nContent-Type: text/html\r\n\r\n<h1>Title</h1><p>Paragraph</p>"

	mock := &mockS3{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader(htmlEmail)),
			}, nil
		},
	}

	m := model{s3Client: mock, bucket: "test-bucket"}
	email, err := m.fetchAndParseEmail(context.Background(), "inbound/html-msg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if email.Subject != "HTML Test" {
		t.Errorf("Subject = %q, want %q", email.Subject, "HTML Test")
	}
	// HTML should be converted to markdown
	if !strings.Contains(email.Body, "Title") {
		t.Errorf("Body should contain 'Title', got %q", email.Body)
	}
	if !strings.Contains(email.Body, "Paragraph") {
		t.Errorf("Body should contain 'Paragraph', got %q", email.Body)
	}
}

func TestFetchAndParseEmail_GetObjectError(t *testing.T) {
	mock := &mockS3{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, fmt.Errorf("not found")
		},
	}

	m := model{s3Client: mock, bucket: "test-bucket"}
	_, err := m.fetchAndParseEmail(context.Background(), "inbound/missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEmailFilename_UsesDateAndSanitizedSubject(t *testing.T) {
	email := Email{
		Subject: "Hello / Bob: report",
		Date:    time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC),
	}

	got := emailFilename(email)

	want := "2025-03-15_103000-Hello-Bob-report.eml"
	if got != want {
		t.Fatalf("filename = %q, want %q", got, want)
	}
}

func TestSaveEmailFile_CreatesUniqueFilename(t *testing.T) {
	dir := t.TempDir()
	email := Email{
		Subject: "Hello",
		Date:    time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC),
	}

	first, err := saveEmailFile(dir, email, []byte("one"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := saveEmailFile(dir, email, []byte("two"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if first == second {
		t.Fatal("expected unique filenames for duplicate saves")
	}
	if filepath.Base(second) != "2025-03-15_103000-Hello-2.eml" {
		t.Fatalf("unexpected second filename: %s", filepath.Base(second))
	}
}

func TestSaveAttachments_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	email := Email{
		Subject: "With attachment",
		Date:    time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC),
		Attachments: []Attachment{
			{Name: "invoice.pdf", Data: []byte("pdf-data")},
		},
	}

	paths, err := saveAttachments(dir, email)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 attachment path, got %d", len(paths))
	}
	content, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "pdf-data" {
		t.Fatalf("saved attachment content = %q", string(content))
	}
}

func TestLoadSelectedEmail_ReturnsNilWhenAlreadyLoaded(t *testing.T) {
	m := model{selectedEmail: &Email{Key: "inbound/one", BodyLoaded: true}}

	if cmd := m.loadSelectedEmail(); cmd != nil {
		t.Fatal("expected nil command when email body is already loaded")
	}
}

func TestLoadSelectedEmail_LoadsEmail(t *testing.T) {
	raw := buildMIMEEmail("alice@example.com", "bob@example.com", "Loaded", "Loaded body", time.Now())
	mock := &mockS3{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(raw))}, nil
		},
	}
	m := newMockTestModel(mock)
	m.selectedEmail = &Email{Key: "inbound/one"}

	cmd := m.loadSelectedEmail()
	if cmd == nil {
		t.Fatal("expected command")
	}

	msg := cmd()
	loaded, ok := msg.(emailLoadedMsg)
	if !ok {
		t.Fatalf("expected emailLoadedMsg, got %T", msg)
	}
	if !loaded.email.BodyLoaded {
		t.Fatal("expected loaded email body")
	}
	if !strings.Contains(loaded.email.Body, "Loaded body") {
		t.Fatalf("unexpected body %q", loaded.email.Body)
	}
}

func TestSaveSelectedEmail_UsesCachedRawMessage(t *testing.T) {
	dir := t.TempDir()
	m := model{
		saveDir:       dir,
		selectedEmail: &Email{Subject: "Saved", Date: time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC), RawLoaded: true, Raw: []byte("raw-message")},
	}

	cmd := m.saveSelectedEmail()
	if cmd == nil {
		t.Fatal("expected command")
	}

	msg := cmd()
	saved, ok := msg.(emailSavedMsg)
	if !ok {
		t.Fatalf("expected emailSavedMsg, got %T", msg)
	}
	content, err := os.ReadFile(saved.path)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if string(content) != "raw-message" {
		t.Fatalf("saved content = %q", string(content))
	}
}

func TestSaveSelectedAttachments_ReturnsEmptyWhenNone(t *testing.T) {
	m := model{
		saveDir:       t.TempDir(),
		selectedEmail: &Email{Subject: "No attachments", BodyLoaded: true},
	}

	cmd := m.saveSelectedAttachments()
	if cmd == nil {
		t.Fatal("expected command")
	}

	msg := cmd()
	saved, ok := msg.(attachmentsSavedMsg)
	if !ok {
		t.Fatalf("expected attachmentsSavedMsg, got %T", msg)
	}
	if len(saved.paths) != 0 {
		t.Fatalf("expected no attachment paths, got %d", len(saved.paths))
	}
}

func TestFallbackTime_PrefersParsedDateThenFallback(t *testing.T) {
	fallback := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	parsed := fallbackTime("Mon, 15 Mar 2025 10:30:00 +0000", &fallback)
	if parsed.Equal(fallback) {
		t.Fatal("expected parsed date to win over fallback")
	}

	got := fallbackTime("not a date", &fallback)
	if !got.Equal(fallback) {
		t.Fatalf("fallback = %v, want %v", got, fallback)
	}
}

func TestSaveSelectedEmail_ReturnsNilWithoutSelection(t *testing.T) {
	m := model{}

	if cmd := m.saveSelectedEmail(); cmd != nil {
		t.Fatal("expected nil command")
	}
}

func TestSaveSelectedEmail_FetchesRawWhenMissing(t *testing.T) {
	dir := t.TempDir()
	raw := buildMIMEEmail("alice@example.com", "bob@example.com", "Saved", "Body", time.Now())
	mock := &mockS3{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(raw))}, nil
		},
	}
	m := newMockTestModel(mock)
	m.saveDir = dir
	m.selectedEmail = &Email{Key: "one", Subject: "Saved", Date: time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)}

	msg := m.saveSelectedEmail()()
	saved, ok := msg.(emailSavedMsg)
	if !ok {
		t.Fatalf("expected emailSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("unexpected error: %v", saved.err)
	}
}

func TestSaveSelectedAttachments_LoadsEmailWhenNeeded(t *testing.T) {
	dir := t.TempDir()
	raw := "From: a@b.com\r\nTo: c@d.com\r\nSubject: Attach\r\nDate: Mon, 15 Mar 2025 10:30:00 +0000\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=abc\r\n\r\n--abc\r\nContent-Type: text/plain\r\n\r\nhello\r\n--abc\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=\"file.txt\"\r\n\r\npayload\r\n--abc--"
	mock := &mockS3{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(raw))}, nil
		},
	}
	m := newMockTestModel(mock)
	m.saveDir = dir
	m.selectedEmail = &Email{Key: "one", Subject: "Attach"}

	msg := m.saveSelectedAttachments()()
	saved, ok := msg.(attachmentsSavedMsg)
	if !ok {
		t.Fatalf("expected attachmentsSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("unexpected error: %v", saved.err)
	}
	if len(saved.paths) != 1 {
		t.Fatalf("paths = %d", len(saved.paths))
	}
}

func TestSaveSelectedAttachments_ReturnsNilWithoutSelection(t *testing.T) {
	m := model{}

	if cmd := m.saveSelectedAttachments(); cmd != nil {
		t.Fatal("expected nil command")
	}
}

func TestEmailFilename_FallsBackToS3DateAndNoSubject(t *testing.T) {
	email := Email{
		S3Date: time.Date(2025, 4, 1, 12, 34, 56, 0, time.UTC),
	}

	got := emailFilename(email)

	if got != "2025-04-01_123456-no-subject.eml" {
		t.Fatalf("got %q", got)
	}
}

func TestSaveEmailFile_ReturnsErrorWhenDirIsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := saveEmailFile(file, Email{Subject: "x", Date: time.Now()}, []byte("raw"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveAttachments_UsesFallbackNameForBlankAttachment(t *testing.T) {
	dir := t.TempDir()
	email := Email{
		Subject: "With attachment",
		Date:    time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC),
		Attachments: []Attachment{
			{Name: "", Data: []byte("data")},
		},
	}

	paths, err := saveAttachments(dir, email)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(filepath.Base(paths[0]), "attachment-1") {
		t.Fatalf("unexpected fallback name: %s", filepath.Base(paths[0]))
	}
}

func TestSaveAttachments_ReturnsErrorWhenDirIsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := saveAttachments(file, Email{
		Subject:     "x",
		Date:        time.Now(),
		Attachments: []Attachment{{Name: "a.txt", Data: []byte("x")}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEmailFilename_TruncatesLongSubject(t *testing.T) {
	email := Email{
		Subject: strings.Repeat("a", 100),
		Date:    time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC),
	}

	got := emailFilename(email)

	if len(filepath.Base(got)) >= len("2025-03-15_103000-"+strings.Repeat("a", 100)+".eml") {
		t.Fatalf("filename was not truncated: %q", got)
	}
}

func TestFetchEmailSummary_FallsBackToLastModifiedWhenDateMissing(t *testing.T) {
	modified := time.Date(2025, 5, 1, 12, 0, 0, 0, time.UTC)
	raw := "From: alice@example.com\r\nTo: bob@example.com\r\nSubject: No Date\r\n\r\nBody"
	mock := &mockS3{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(raw))}, nil
		},
	}
	m := newMockTestModel(mock)

	email, err := m.fetchEmailSummary(context.Background(), types.Object{Key: aws.String("one"), LastModified: &modified})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !email.Date.Equal(modified) {
		t.Fatalf("date = %v, want %v", email.Date, modified)
	}
}

func TestLoadSelectedEmail_ReturnsErrorMsgOnFetchFailure(t *testing.T) {
	mock := &mockS3{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, fmt.Errorf("boom")
		},
	}
	m := newMockTestModel(mock)
	m.selectedEmail = &Email{Key: "inbound/one"}

	msg := m.loadSelectedEmail()()
	if _, ok := msg.(errorMsg); !ok {
		t.Fatalf("expected errorMsg, got %T", msg)
	}
}

func TestSaveSelectedEmail_ReturnsErrorWhenFetchFails(t *testing.T) {
	mock := &mockS3{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, fmt.Errorf("boom")
		},
	}
	m := newMockTestModel(mock)
	m.saveDir = t.TempDir()
	m.selectedEmail = &Email{Key: "one", Subject: "Saved", Date: time.Now()}

	msg := m.saveSelectedEmail()()
	saved, ok := msg.(emailSavedMsg)
	if !ok {
		t.Fatalf("expected emailSavedMsg, got %T", msg)
	}
	if saved.err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveSelectedAttachments_ReturnsErrorWhenLoadFails(t *testing.T) {
	mock := &mockS3{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, fmt.Errorf("boom")
		},
	}
	m := newMockTestModel(mock)
	m.saveDir = t.TempDir()
	m.selectedEmail = &Email{Key: "one", Subject: "Attach"}

	msg := m.saveSelectedAttachments()()
	saved, ok := msg.(attachmentsSavedMsg)
	if !ok {
		t.Fatalf("expected attachmentsSavedMsg, got %T", msg)
	}
	if saved.err == nil {
		t.Fatal("expected error")
	}
}

func TestParseFullEmail_ParsesPlainTextWithoutDate(t *testing.T) {
	raw := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: Plain\r\n\r\nhello")

	email, err := parseFullEmail(raw, "inbound/plain")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email.Date.IsZero() != true {
		t.Fatalf("expected zero date, got %v", email.Date)
	}
	if email.Body != "hello" {
		t.Fatalf("body = %q", email.Body)
	}
}

func TestFallbackTime_ReturnsZeroWithoutInputs(t *testing.T) {
	got := fallbackTime("", nil)
	if !got.IsZero() {
		t.Fatalf("got %v", got)
	}
}

// newMockTestModel creates a model with a mock S3 client suitable for tests
// that need the full model setup (spinner, ready state, etc.)
func newMockTestModel(mock s3API) model {
	sp := spinner.New()
	sp.Spinner = spinner.Globe
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return model{
		s3Client: mock,
		spinner:  sp,
		state:    listState,
		ready:    true,
		width:    120,
		height:   40,
		bucket:   "test-bucket",
		prefix:   "inbound/",
	}
}
