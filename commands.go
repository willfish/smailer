package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jhillyerd/enmime"
)

var invalidFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (m model) loadBuckets() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		output, err := m.s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
		if err != nil {
			return errorMsg{err}
		}
		var buckets []string
		for _, b := range output.Buckets {
			buckets = append(buckets, *b.Name)
		}
		var sesBuckets, otherBuckets []string
		for _, b := range buckets {
			if strings.Contains(strings.ToLower(b), "ses") {
				sesBuckets = append(sesBuckets, b)
			} else {
				otherBuckets = append(otherBuckets, b)
			}
		}
		sort.Strings(sesBuckets)
		sort.Strings(otherBuckets)
		buckets = append(sesBuckets, otherBuckets...)
		return bucketsLoadedMsg{buckets: buckets}
	}
}

func (m model) loadEmails() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(m.bucket),
			Prefix:            aws.String(m.prefix),
			MaxKeys:           aws.Int32(10),
			ContinuationToken: m.continuation,
		}

		page, err := m.s3Client.ListObjectsV2(ctx, input)
		if err != nil {
			return errorMsg{err}
		}

		var newEmails []Email
		skipped := 0
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			email, err := m.fetchEmailSummary(ctx, obj)
			if err != nil {
				email = fallbackEmailSummary(obj)
			}
			newEmails = append(newEmails, *email)
		}

		var nextContinuation *string
		hasMore := false
		if page.IsTruncated != nil && *page.IsTruncated {
			nextContinuation = page.NextContinuationToken
			hasMore = true
		}

		return emailsLoadedMsg{emails: newEmails, continuation: nextContinuation, hasMore: hasMore, skipped: skipped}
	}
}

func (m model) fetchEmailSummary(ctx context.Context, obj types.Object) (*Email, error) {
	body, err := m.fetchObject(ctx, *obj.Key)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	msg, err := mail.ReadMessage(bufio.NewReader(body))
	if err != nil {
		return nil, err
	}

	date, err := mailHeaderDate(msg.Header)
	if err != nil {
		date = fallbackTime(msg.Header.Get("Date"), obj.LastModified)
	}
	if date.IsZero() {
		date = fallbackTime("", obj.LastModified)
	}

	return &Email{
		From:    msg.Header.Get("From"),
		To:      msg.Header.Get("To"),
		Subject: msg.Header.Get("Subject"),
		Date:    date,
		S3Date:  aws.ToTime(obj.LastModified),
		Key:     *obj.Key,
		Size:    aws.ToInt64(obj.Size),
	}, nil
}

func fallbackEmailSummary(obj types.Object) *Email {
	date := aws.ToTime(obj.LastModified)
	return &Email{
		From:         "",
		To:           "",
		Subject:      "(unparseable email)",
		Date:         date,
		S3Date:       date,
		Key:          aws.ToString(obj.Key),
		Size:         aws.ToInt64(obj.Size),
		SummaryError: true,
	}
}

func (m model) fetchObject(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := m.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(m.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}

func (m model) fetchAndParseEmail(ctx context.Context, key string) (*Email, error) {
	raw, err := m.fetchRawEmail(ctx, key)
	if err != nil {
		return nil, err
	}
	return parseFullEmail(raw, key)
}

func (m model) fetchRawEmail(ctx context.Context, key string) ([]byte, error) {
	body, err := m.fetchObject(ctx, key)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

func parseFullEmail(raw []byte, key string) (*Email, error) {
	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}

	date, err := env.Date()
	if err != nil {
		date = time.Time{}
	}

	var emailBody string
	if env.HTML != "" {
		converter := md.NewConverter("", true, nil)
		emailBody, err = converter.ConvertString(env.HTML)
		if err != nil {
			emailBody = env.Text
		}
	} else {
		emailBody = env.Text
	}

	attachments := make([]Attachment, 0, len(env.Attachments))
	for _, part := range env.Attachments {
		name := strings.TrimSpace(part.FileName)
		if name == "" {
			name = "attachment"
		}
		attachments = append(attachments, Attachment{Name: name, Data: append([]byte(nil), part.Content...)})
	}

	return &Email{
		From:        env.GetHeader("From"),
		To:          env.GetHeader("To"),
		Subject:     env.GetHeader("Subject"),
		Date:        date,
		Body:        emailBody,
		Key:         key,
		RawLoaded:   true,
		BodyLoaded:  true,
		Raw:         append([]byte(nil), raw...),
		Attachments: attachments,
	}, nil
}

func (m model) getEmailBody(e *Email) string {
	if m.glamourRenderer == nil {
		return e.Body
	}
	rendered, err := m.glamourRenderer.Render(e.Body)
	if err != nil {
		return e.Body
	}
	return rendered
}

func (m model) deleteEmail() tea.Cmd {
	key := m.selectedEmail.Key
	return func() tea.Msg {
		ctx := context.Background()
		_, err := m.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(m.bucket),
			Key:    aws.String(key),
		})
		return emailDeletedMsg{err: err}
	}
}

func (m model) loadSelectedEmail() tea.Cmd {
	if m.selectedEmail == nil || m.selectedEmail.BodyLoaded {
		return nil
	}
	key := m.selectedEmail.Key
	return func() tea.Msg {
		email, err := m.fetchAndParseEmail(context.Background(), key)
		if err != nil {
			return errorMsg{err}
		}
		return emailLoadedMsg{email: *email}
	}
}

func (m model) saveSelectedEmail() tea.Cmd {
	if m.selectedEmail == nil {
		return nil
	}
	selected := *m.selectedEmail
	return func() tea.Msg {
		raw := selected.Raw
		if !selected.RawLoaded {
			var err error
			raw, err = m.fetchRawEmail(context.Background(), selected.Key)
			if err != nil {
				return emailSavedMsg{err: err}
			}
		}

		path, err := saveEmailFile(m.saveDir, selected, raw)
		if err != nil {
			return emailSavedMsg{err: err}
		}
		return emailSavedMsg{path: path}
	}
}

func (m model) saveSelectedAttachments() tea.Cmd {
	if m.selectedEmail == nil {
		return nil
	}
	selected := *m.selectedEmail
	return func() tea.Msg {
		if !selected.BodyLoaded {
			loaded, err := m.fetchAndParseEmail(context.Background(), selected.Key)
			if err != nil {
				return attachmentsSavedMsg{err: err}
			}
			selected = *loaded
		}
		if len(selected.Attachments) == 0 {
			return attachmentsSavedMsg{}
		}
		paths, err := saveAttachments(m.saveDir, selected)
		if err != nil {
			return attachmentsSavedMsg{err: err}
		}
		return attachmentsSavedMsg{paths: paths}
	}
}

func saveEmailFile(dir string, email Email, raw []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filename := emailFilename(email)
	path, err := uniquePath(filepath.Join(dir, filename))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func saveAttachments(dir string, email Email) ([]string, error) {
	baseDir := filepath.Join(dir, strings.TrimSuffix(emailFilename(email), ".eml")+"-attachments")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(email.Attachments))
	for i, attachment := range email.Attachments {
		name := sanitizeFilename(attachment.Name)
		if name == "" || name == "." {
			name = fmt.Sprintf("attachment-%d", i+1)
		}
		path, err := uniquePath(filepath.Join(baseDir, name))
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, attachment.Data, 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func uniquePath(path string) (string, error) {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	candidate := path
	for i := 2; ; i++ {
		_, err := os.Stat(candidate)
		if err == nil {
			candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
			continue
		}
		if os.IsNotExist(err) {
			return candidate, nil
		}
		return "", err
	}
}

func emailFilename(email Email) string {
	stamp := email.Date
	if stamp.IsZero() {
		stamp = email.S3Date
	}
	if stamp.IsZero() {
		stamp = time.Now()
	}
	subject := sanitizeFilename(email.Subject)
	if subject == "" {
		subject = "no-subject"
	}
	if len(subject) > 80 {
		subject = subject[:80]
	}
	return fmt.Sprintf("%s-%s.eml", stamp.Format("2006-01-02_150405"), subject)
}

func sanitizeFilename(input string) string {
	cleaned := strings.TrimSpace(input)
	cleaned = strings.Join(strings.Fields(cleaned), "-")
	cleaned = invalidFilenameChars.ReplaceAllString(cleaned, "-")
	cleaned = strings.Trim(regexp.MustCompile(`-+`).ReplaceAllString(cleaned, "-"), "-.")
	cleaned = strings.Trim(cleaned, "-.")
	return cleaned
}

func mailHeaderDate(header mail.Header) (time.Time, error) {
	return header.Date()
}

func fallbackTime(raw string, fallback *time.Time) time.Time {
	if raw != "" {
		if t, err := mail.ParseDate(raw); err == nil {
			return t
		}
	}
	if fallback != nil {
		return *fallback
	}
	return time.Time{}
}
