package main

import (
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type state int

const (
	bucketSelectionState state = iota
	listState
	viewState
	confirmDeleteState
)

type Email struct {
	From    string
	To      string
	Subject string
	Date    time.Time
	Body    string
	Key     string
}

type model struct {
	table           table.Model
	viewport        viewport.Model
	spinner         spinner.Model
	bucketsList     list.Model
	emails          []Email
	state           state
	previousState   state
	selectedEmail   *Email
	selectedIndex   int
	s3Client        *s3.Client
	bucket          string
	prefix          string
	continuation    *string
	hasMore         bool
	ready           bool
	width           int
	height          int
	err             error
	loading         bool
	glamourRenderer *glamour.TermRenderer
	statusMessage   string
}

type emailsLoadedMsg struct {
	emails       []Email
	continuation *string
	hasMore      bool
}

type bucketsLoadedMsg struct {
	buckets []string
}

type errorMsg struct {
	err error
}

type emailDeletedMsg struct {
	err error
}

type clearStatusMsg struct{}

type item struct {
	title string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return "" }
func (i item) FilterValue() string { return i.title }
