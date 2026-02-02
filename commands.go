package main

import (
	"bytes"
	"context"
	"io"
	"sort"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jhillyerd/enmime"
)

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
		for _, obj := range page.Contents {
			email, err := m.fetchAndParseEmail(ctx, *obj.Key)
			if err != nil {
				continue
			}
			newEmails = append(newEmails, *email)
		}

		var nextContinuation *string
		hasMore := false
		if page.IsTruncated != nil && *page.IsTruncated {
			nextContinuation = page.NextContinuationToken
			hasMore = true
		}

		return emailsLoadedMsg{emails: newEmails, continuation: nextContinuation, hasMore: hasMore}
	}
}

func (m model) fetchAndParseEmail(ctx context.Context, key string) (*Email, error) {
	getInput := &s3.GetObjectInput{
		Bucket: aws.String(m.bucket),
		Key:    aws.String(key),
	}
	result, err := m.s3Client.GetObject(ctx, getInput)
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()

	body, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}

	env, err := enmime.ReadEnvelope(io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return nil, err
	}

	from := env.GetHeader("From")
	to := env.GetHeader("To")
	subject := env.GetHeader("Subject")
	date, err := env.Date()
	if err != nil {
		date = time.Now()
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

	return &Email{
		From:    from,
		To:      to,
		Subject: subject,
		Date:    date,
		Body:    emailBody,
		Key:     key,
	}, nil
}

func (m model) getEmailBody(e *Email) string {
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
