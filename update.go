package main

import (
	"sort"
	"strings"
	"time"

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
			switch {
			case msg.String() == "ctrl+c" || msg.String() == "q":
				return m, tea.Quit
			case msg.String() == "esc":
				m.state = bucketSelectionState
				m.emails = nil
				m.continuation = nil
				m.hasMore = true
				m.loading = true
				return m, tea.Batch(m.loadBuckets(), m.spinner.Tick)
			case msg.String() == "enter":
				if len(m.emails) > 0 {
					m.selectedIndex = m.table.Cursor()
					m.selectedEmail = &m.emails[m.selectedIndex]
					m.viewport.SetContent(m.getEmailBody(m.selectedEmail))
					m.state = viewState
				}
			case msg.String() == "d":
				if len(m.emails) > 0 {
					m.previousState = listState
					m.selectedIndex = m.table.Cursor()
					m.selectedEmail = &m.emails[m.selectedIndex]
					m.state = confirmDeleteState
				}
			case msg.String() == "r":
				m.emails = nil
				m.continuation = nil
				m.hasMore = true
				m.loading = true
				m.updateTableRows()
				return m, tea.Batch(m.loadEmails(), m.spinner.Tick)
			case msg.String() == "down" || msg.String() == "j":
				m.table, cmd = m.table.Update(msg)
				cmds = append(cmds, cmd)
				if m.table.Cursor() == len(m.emails)-1 && m.hasMore && !m.loading {
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
		// Filter for SES-matching buckets
		var sesBuckets []string
		for _, b := range msg.buckets {
			if strings.Contains(strings.ToLower(b), "ses") {
				sesBuckets = append(sesBuckets, b)
			}
		}
		// Auto-select if exactly one SES bucket
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
		m.emails = append(m.emails, msg.emails...)
		sort.SliceStable(m.emails, func(i, j int) bool {
			return m.emails[i].Date.After(m.emails[j].Date)
		})
		m.hasMore = msg.hasMore
		m.continuation = msg.continuation
		m.updateTableRows()
	case emailDeletedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.emails = append(m.emails[:m.selectedIndex], m.emails[m.selectedIndex+1:]...)
			m.updateTableRows()
			if len(m.emails) > 0 {
				if m.selectedIndex >= len(m.emails) {
					m.selectedIndex = len(m.emails) - 1
				}
				m.table.SetCursor(m.selectedIndex)
			}
			m.state = m.previousState
			m.statusMessage = "Email deleted"
			cmds = append(cmds, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			}))
		}
	case clearStatusMsg:
		m.statusMessage = ""
	case errorMsg:
		m.err = msg.err
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}
