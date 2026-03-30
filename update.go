package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Init() tea.Cmd {
	if m.state == bucketSelectionState {
		return tea.Batch(m.loadBuckets(), m.spinner.Tick)
	}
	return tea.Batch(m.loadEmails(), m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.state == confirmDeleteState {
			switch msg.String() {
			case "y":
				cmd = m.deleteEmail()
				cmds = append(cmds, cmd)
			case "n", "esc":
				m.state = m.previousState
			}
			return m, tea.Batch(cmds...)
		}

		switch m.state {
		case bucketSelectionState:
			switch msg.String() {
			case "enter":
				if selected, ok := m.bucketsList.SelectedItem().(item); ok {
					m.bucket = selected.title
					m.state = listState
					m.loading = true
					return m, tea.Batch(m.loadEmails(), m.spinner.Tick)
				}
			case "ctrl+c", "q":
				return m, tea.Quit
			default:
				m.bucketsList, cmd = m.bucketsList.Update(msg)
				cmds = append(cmds, cmd)
			}
		case listState:
			if m.filterActive {
				switch msg.String() {
				case "esc":
					m.filterActive = false
					m.filterInput.Blur()
				case "enter":
					m.filterQuery = strings.TrimSpace(m.filterInput.Value())
					m.filterActive = false
					m.filterInput.Blur()
					m.updateTableRows()
					m.setStatus(filterStatus(m.filterQuery, len(m.visibleEmails)))
				default:
					m.filterInput, cmd = m.filterInput.Update(msg)
					m.filterQuery = strings.TrimSpace(m.filterInput.Value())
					m.updateTableRows()
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
			switch {
			case msg.String() == "ctrl+c" || msg.String() == "q":
				return m, tea.Quit
			case msg.String() == "esc":
				m.state = bucketSelectionState
				m.emails = nil
				m.visibleEmails = nil
				m.continuation = nil
				m.hasMore = true
				m.loading = true
				m.filterQuery = ""
				m.filterInput.SetValue("")
				return m, tea.Batch(m.loadBuckets(), m.spinner.Tick)
			case msg.String() == "enter":
				if len(m.visibleEmails) > 0 {
					m.selectedIndex = m.table.Cursor()
					selected := m.visibleEmails[m.selectedIndex]
					m.selectedEmail = &selected
					m.state = viewState
					m.viewport.SetContent("Loading email...")
					if selected.BodyLoaded {
						m.viewport.SetContent(m.getEmailBody(m.selectedEmail))
						return m, nil
					}
					return m, tea.Batch(m.loadSelectedEmail(), m.spinner.Tick)
				}
			case msg.String() == "d":
				if len(m.visibleEmails) > 0 {
					m.previousState = listState
					m.selectedIndex = m.table.Cursor()
					selected := m.visibleEmails[m.selectedIndex]
					m.selectedEmail = &selected
					m.state = confirmDeleteState
				}
			case msg.String() == "r":
				m.emails = nil
				m.visibleEmails = nil
				m.continuation = nil
				m.hasMore = true
				m.loading = true
				m.updateTableRows()
				return m, tea.Batch(m.loadEmails(), m.spinner.Tick)
			case msg.String() == "/":
				m.filterActive = true
				m.filterInput.SetValue(m.filterQuery)
				m.filterInput.Focus()
			case msg.String() == "s":
				if len(m.visibleEmails) > 0 {
					m.selectedIndex = m.table.Cursor()
					selected := m.visibleEmails[m.selectedIndex]
					m.selectedEmail = &selected
					return m, m.saveSelectedEmail()
				}
			case msg.String() == "down" || msg.String() == "j":
				m.table, cmd = m.table.Update(msg)
				cmds = append(cmds, cmd)
				if m.table.Cursor() == len(m.visibleEmails)-1 && m.hasMore && !m.loading {
					m.loading = true
					cmds = append(cmds, m.loadEmails())
				}
			default:
				m.table, cmd = m.table.Update(msg)
				cmds = append(cmds, cmd)
			}
		case viewState:
			switch msg.String() {
			case "esc", "q":
				m.state = listState
			case "d":
				m.previousState = viewState
				m.state = confirmDeleteState
			case "s":
				return m, m.saveSelectedEmail()
			case "a":
				return m, m.saveSelectedAttachments()
			default:
				m.viewport, cmd = m.viewport.Update(msg)
				cmds = append(cmds, cmd)
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.initComponents()
			m.ready = true
		}
		m.updateComponents()
	case bucketsLoadedMsg:
		m.loading = false
		var sesBuckets []string
		for _, b := range msg.buckets {
			if strings.Contains(strings.ToLower(b), "ses") {
				sesBuckets = append(sesBuckets, b)
			}
		}
		if len(sesBuckets) == 1 {
			m.bucket = sesBuckets[0]
			m.state = listState
			m.loading = true
			return m, tea.Batch(m.loadEmails(), m.spinner.Tick)
		}
		items := []list.Item{}
		for _, b := range msg.buckets {
			items = append(items, item{title: b})
		}
		m.bucketsList = list.New(items, list.NewDefaultDelegate(), m.width-4, m.height-6)
		m.bucketsList.Title = "Select a Bucket"
		m.bucketsList.SetShowHelp(false)
	case emailsLoadedMsg:
		m.loading = false
		m.emails = mergeEmailsByKey(m.emails, msg.emails)
		sort.SliceStable(m.emails, func(i, j int) bool {
			return m.emails[i].Date.After(m.emails[j].Date)
		})
		m.hasMore = msg.hasMore
		m.continuation = msg.continuation
		m.updateTableRows()
		if msg.skipped > 0 {
			m.setStatus(fmt.Sprintf("Skipped %d unparseable email(s)", msg.skipped))
		}
	case emailLoadedMsg:
		m.replaceEmail(msg.email)
		m.selectedEmail = m.findEmailByKey(msg.email.Key)
		if m.selectedEmail != nil {
			m.viewport.SetContent(m.getEmailBody(m.selectedEmail))
		}
	case emailDeletedMsg:
		if msg.err != nil {
			m.setStatus("Delete failed: " + msg.err.Error())
		} else {
			m.deleteEmailByKey(m.selectedEmail.Key)
			m.updateTableRows()
			if len(m.visibleEmails) > 0 {
				if m.selectedIndex >= len(m.visibleEmails) {
					m.selectedIndex = len(m.visibleEmails) - 1
				}
				m.table.SetCursor(m.selectedIndex)
			}
			m.state = m.previousState
			m.setStatus("Email deleted")
		}
	case emailSavedMsg:
		if msg.err != nil {
			m.setStatus("Save failed: " + msg.err.Error())
		} else {
			m.setStatus("Saved .eml to " + msg.path)
		}
	case attachmentsSavedMsg:
		if msg.err != nil {
			m.setStatus("Attachment save failed: " + msg.err.Error())
		} else if len(msg.paths) == 0 {
			m.setStatus("No attachments to save")
		} else {
			m.setStatus(fmt.Sprintf("Saved %d attachment(s)", len(msg.paths)))
		}
	case clearStatusMsg:
		m.statusMessage = ""
	case errorMsg:
		m.loading = false
		m.setStatus("Error: " + msg.err.Error())
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func mergeEmailsByKey(existing []Email, incoming []Email) []Email {
	merged := append([]Email(nil), existing...)
	seen := make(map[string]int, len(merged))
	for i, email := range merged {
		seen[email.Key] = i
	}
	for _, email := range incoming {
		if idx, ok := seen[email.Key]; ok {
			merged[idx] = mergeEmailData(merged[idx], email)
			continue
		}
		seen[email.Key] = len(merged)
		merged = append(merged, email)
	}
	return merged
}

func mergeEmailData(current, incoming Email) Email {
	if incoming.From != "" {
		current.From = incoming.From
	}
	if incoming.To != "" {
		current.To = incoming.To
	}
	if incoming.Subject != "" {
		current.Subject = incoming.Subject
	}
	if !incoming.Date.IsZero() {
		current.Date = incoming.Date
	}
	if !incoming.S3Date.IsZero() {
		current.S3Date = incoming.S3Date
	}
	if incoming.Size != 0 {
		current.Size = incoming.Size
	}
	if incoming.BodyLoaded {
		current.Body = incoming.Body
		current.BodyLoaded = true
		current.Attachments = incoming.Attachments
	}
	if incoming.RawLoaded {
		current.Raw = incoming.Raw
		current.RawLoaded = true
	}
	return current
}

func (m *model) replaceEmail(incoming Email) {
	for i, email := range m.emails {
		if email.Key == incoming.Key {
			m.emails[i] = mergeEmailData(email, incoming)
			m.updateTableRows()
			return
		}
	}
	m.emails = append(m.emails, incoming)
	m.updateTableRows()
}

func (m *model) findEmailByKey(key string) *Email {
	for i := range m.emails {
		if m.emails[i].Key == key {
			return &m.emails[i]
		}
	}
	return nil
}

func (m *model) deleteEmailByKey(key string) {
	filtered := m.emails[:0]
	for _, email := range m.emails {
		if email.Key != key {
			filtered = append(filtered, email)
		}
	}
	m.emails = filtered
	m.selectedEmail = nil
}

func (m *model) setStatus(message string) {
	m.statusMessage = message
}

func filterStatus(query string, count int) string {
	if query == "" {
		return "Filter cleared"
	}
	return fmt.Sprintf("Filter '%s' matched %d email(s)", query, count)
}
