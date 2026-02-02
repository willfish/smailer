package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/ansi"
)

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}
	if !m.ready {
		return "Initializing...\n"
	}

	title := titleStyle.Width(m.width).Render("🌌 Smailer: S3 Inbox Reader 🚀")

	var baseView string

	switch m.state {
	case bucketSelectionState:
		help := helpStyle.Render("↑/↓: navigate • enter: select • q: quit")
		content := baseStyle.Width(m.width).Height(m.height - 4).Render(m.bucketsList.View())
		baseView = lipgloss.JoinVertical(lipgloss.Left, title, content, help)
	case listState, confirmDeleteState:
		if m.state == confirmDeleteState && m.previousState == viewState {
			baseView = m.renderEmailView()
		} else {
			help := m.renderListHelp()
			if len(m.emails) == 0 && !m.loading {
				emptyMsg := lipgloss.NewStyle().
					Foreground(lipgloss.Color("240")).
					Align(lipgloss.Center).
					Render("No emails found")
				content := baseStyle.Width(m.width).Height(m.height - 4).Render(
					lipgloss.Place(m.width-4, m.height-6, lipgloss.Center, lipgloss.Center, emptyMsg),
				)
				baseView = lipgloss.JoinVertical(lipgloss.Left, title, content, help)
			} else {
				content := baseStyle.Width(m.width).Height(m.height - 4).Render(m.table.View())
				baseView = lipgloss.JoinVertical(lipgloss.Left, title, content, help)
			}
		}
	case viewState:
		baseView = m.renderEmailView()
	}

	if m.state == bucketSelectionState && m.loading {
		splash := splashStyle.Render(spaceSplash)
		loading := m.spinner.View() + " Loading buckets..."
		content := lipgloss.JoinVertical(lipgloss.Center, splash, loading)
		return lipgloss.JoinVertical(lipgloss.Left, title, baseStyle.Width(m.width).Height(m.height-4).Render(content))
	}

	if (m.state == listState || (m.state == confirmDeleteState && m.previousState == listState)) && len(m.emails) == 0 && m.loading {
		splash := splashStyle.Render(spaceSplash)
		loading := m.spinner.View() + " Loading emails..."
		content := lipgloss.JoinVertical(lipgloss.Center, splash, loading)
		return lipgloss.JoinVertical(lipgloss.Left, title, baseStyle.Width(m.width).Height(m.height-4).Render(content))
	}

	if m.state == confirmDeleteState {
		modalContent := modalStyle.Render("Delete this email?\n\nPress y to confirm, n or esc to cancel.")
		modalWidth := lipgloss.Width(modalContent)
		modalHeight := lipgloss.Height(modalContent)
		modalX := (m.width - modalWidth) / 2
		modalY := (m.height - modalHeight) / 2
		baseView = placeOverlay(modalX, modalY, modalContent, baseView)
	}

	return baseView
}

func (m model) renderEmailView() string {
	title := titleStyle.Width(m.width).Render("🌌 Smailer: S3 Inbox Reader 🚀")
	help := helpStyle.Render("↑/↓: scroll • esc/q: back • d: delete")
	content := bodyStyle.Width(m.width).Height(m.height - 4).Render(m.viewport.View())
	header := headerStyle.Render(fmt.Sprintf(
		"From:    %s\nTo:      %s\nSubject: %s\nDate:    %s",
		m.selectedEmail.From,
		m.selectedEmail.To,
		m.selectedEmail.Subject,
		m.selectedEmail.Date.Format("2006-01-02 15:04"),
	))
	return lipgloss.JoinVertical(lipgloss.Left, title, header, content, help)
}

func (m model) renderListHelp() string {
	parts := []string{"↑/↓: navigate • enter: read • d: delete • r: refresh • esc: back • q: quit"}

	countStr := fmt.Sprintf("%d emails", len(m.emails))
	if m.hasMore {
		countStr += " (more available)"
	}
	parts = append(parts, countStr)

	help := helpStyle.Render(strings.Join(parts, " • "))

	if m.loading {
		help += " (loading more...)"
	}

	if m.statusMessage != "" {
		help += "  " + statusStyle.Render(m.statusMessage)
	}

	return help
}

func cutToWidth(s string, w int) (prefix, remainder string) {
	var buf strings.Builder
	var currentWidth int
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			start := i
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && s[i] != 'm' {
					i++
				}
				if i < len(s) {
					i++
				}
			}
			buf.WriteString(s[start:i])
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := ansi.PrintableRuneWidth(string(r))
		if currentWidth+rw > w {
			break
		}
		buf.WriteRune(r)
		currentWidth += rw
		i += size
	}
	return buf.String(), s[i:]
}

func placeOverlay(x, y int, overlay, base string) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	for i, oLine := range overlayLines {
		oy := y + i
		if oy >= len(baseLines) {
			break
		}
		bLine := baseLines[oy]
		oWidth := lipgloss.Width(oLine)
		left, rest := cutToWidth(bLine, x)
		_, right := cutToWidth(rest, oWidth)
		baseLines[oy] = left + oLine + right
	}
	return strings.Join(baseLines, "\n")
}

var spaceSplash = strings.TrimSpace(`
   .    '     *     .      *    .     '
     .      .     *    .    '   *     .
*  .     *     .    '    .      *    .
   '    .     *     .    *    .     '
.     *    .      *     .     '     *
     .    '     .     *    .      *    .
`)
