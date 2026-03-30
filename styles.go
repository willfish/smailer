package main

import (
	"math"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	baseStyle     = lipgloss.NewStyle().Padding(1).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("99"))         // Purple border
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51")).Background(lipgloss.Color("0")).Padding(0, 1) // Cyan on black
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("201"))                                                         // Magenta
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45")).Align(lipgloss.Center).Padding(1)
	bodyStyle     = lipgloss.NewStyle().Padding(1)
	splashStyle   = lipgloss.NewStyle().Align(lipgloss.Center).Foreground(lipgloss.Color("45"))
	modalStyle    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(1, 2).
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("255")).
			Width(40)
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	filterStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("45")).
			Padding(0, 1).
			Background(lipgloss.Color("235"))
)

func (m *model) initComponents() {
	columns := []table.Column{
		{Title: "From", Width: 40},
		{Title: "Subject", Width: 50},
		{Title: "Date", Width: 20},
		{Title: "Key", Width: 18},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(m.height-6),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.Bold(true).Foreground(lipgloss.Color("51"))
	s.Selected = s.Selected.Foreground(lipgloss.Color("201")).Bold(true)
	t.SetStyles(s)

	m.table = t

	vp := viewport.New(m.width-2, m.height-6)
	vp.KeyMap = viewport.DefaultKeyMap()
	m.viewport = vp

	renderer, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(m.width-4))
	m.glamourRenderer = renderer

	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	m.bucketsList = list.New([]list.Item{}, delegate, m.width-4, m.height-6)
	m.bucketsList.Title = "Select a Bucket"
	m.bucketsList.SetShowHelp(false)

	ti := textinput.New()
	ti.Placeholder = "Filter by from, to, or subject"
	ti.CharLimit = 256
	ti.Width = max(20, m.width-20)
	m.filterInput = ti
}

func (m *model) updateComponents() {
	m.viewport.Width = m.width - 4
	m.viewport.Height = m.height - 6

	m.table.SetWidth(m.width - 4)
	m.table.SetHeight(m.height - 6)

	numColumns := 4
	borderWidth := numColumns + 1
	availableContent := max(0, m.width-4-borderWidth)
	proportions := []float64{0.28, 0.38, 0.18, 0.16}
	mins := []int{22, 28, 16, 14}

	var colWidths []int
	sumWidth := 0
	for i, p := range proportions {
		w := max(mins[i], int(math.Round(p*float64(availableContent))))
		colWidths = append(colWidths, w)
		sumWidth += w
	}

	if sumWidth > availableContent {
		scale := float64(availableContent) / float64(sumWidth)
		for i := range colWidths {
			colWidths[i] = max(1, int(math.Round(float64(colWidths[i])*scale)))
		}
	}

	newColumns := []table.Column{
		{Title: "From", Width: colWidths[0]},
		{Title: "Subject", Width: colWidths[1]},
		{Title: "Date", Width: colWidths[2]},
		{Title: "Key", Width: colWidths[3]},
	}
	m.table.SetColumns(newColumns)

	m.bucketsList.SetWidth(m.width - 4)
	m.bucketsList.SetHeight(m.height - 6)
	m.filterInput.Width = max(20, m.width-20)
}

func (m *model) updateTableRows() {
	rows := []table.Row{}
	m.visibleEmails = m.filteredEmails()
	for _, e := range m.visibleEmails {
		rows = append(rows, table.Row{
			e.From,
			e.Subject,
			e.Date.Format("2006-01-02 15:04"),
			shortKey(e.Key),
		})
	}
	m.table.SetRows(rows)
	if len(rows) == 0 {
		m.table.SetCursor(0)
		return
	}
	if m.table.Cursor() >= len(rows) {
		m.table.SetCursor(len(rows) - 1)
	}
}
