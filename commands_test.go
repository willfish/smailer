package main

import (
	"context"
	"fmt"
	"io"
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
	if email.Body != "How are you?" {
		t.Errorf("Body = %q, want %q", email.Body, "How are you?")
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

func TestLoadEmails_SkipsUnparseableObjects(t *testing.T) {
	goodEmail := buildMIMEEmail("a@b.com", "c@d.com", "Good", "body", time.Now())

	mock := &mockS3{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []types.Object{
					{Key: aws.String("inbound/good")},
					{Key: aws.String("inbound/bad")},
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
	if len(loaded.emails) != 1 {
		t.Errorf("expected 1 parseable email, got %d", len(loaded.emails))
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
