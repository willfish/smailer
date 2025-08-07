package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhillyerd/enmime"
	"github.com/joho/godotenv"
	"github.com/muesli/reflow/ansi"
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
	table            table.Model
	viewport         viewport.Model
	spinner          spinner.Model
	bucketsList      list.Model
	emails           []Email
	state            state
	selectedEmail    *Email
	selectedIndex    int
	s3Client         *s3.Client
	bucket           string
	prefix           string
	continuation     *string
	hasMore          bool
	ready            bool
	width            int
	height           int
	err              error
	loading          bool
	glamourRenderer  *glamour.TermRenderer
	isDeleteFromList bool
}

func main() {
	_ = godotenv.Load()

	bucket := os.Getenv("BUCKET")
	prefix := os.Getenv("PREFIX")
	if prefix == "" {
		prefix = "inbound/"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = os.Getenv("REGION")
	}
	if region == "" {
		region = "eu-west-2"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		fmt.Printf("Error loading AWS config: %v\n", err)
		os.Exit(1)
	}
	client := s3.NewFromConfig(cfg)

	p := tea.NewProgram(initialModel(client, bucket, prefix))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func initialModel(client *s3.Client, bucket, prefix string) model {
	s := spinner.New()
	s.Spinner = spinner.Globe
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := model{
		s3Client: client,
		bucket:   bucket,
		prefix:   prefix,
		hasMore:  true,
		spinner:  s,
		loading:  true,
	}

	if bucket == "" {
		m.state = bucketSelectionState
	} else {
		m.state = listState
	}

	return m
}

func (m model) Init() tea.Cmd {
	if m.state == bucketSelectionState {
		return m.loadBuckets()
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
				if m.isDeleteFromList {
					m.state = listState
				} else {
					m.state = viewState
				}
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
					return m, m.loadEmails()
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
			case msg.String() == "enter":
				if len(m.emails) > 0 {
					m.selectedIndex = m.table.Cursor()
					m.selectedEmail = &m.emails[m.selectedIndex]
					m.viewport.SetContent(m.getEmailBody(m.selectedEmail))
					m.state = viewState
				}
			case msg.String() == "d":
				if len(m.emails) > 0 {
					m.isDeleteFromList = true
					m.selectedIndex = m.table.Cursor()
					m.selectedEmail = &m.emails[m.selectedIndex]
					m.state = confirmDeleteState
				}
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
				m.isDeleteFromList = false
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
			m.state = listState
		}
	case errorMsg:
		m.err = msg.err
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

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
		if m.state == confirmDeleteState && !m.isDeleteFromList {
			help := helpStyle.Render("↑/↓: scroll • esc/q: back • d: delete")
			content := bodyStyle.Width(m.width).Height(m.height - 4).Render(m.viewport.View())
			header := headerStyle.Render(fmt.Sprintf("From: %s | To: %s | Subject: %s | Date: %s",
				m.selectedEmail.From, m.selectedEmail.To, m.selectedEmail.Subject, m.selectedEmail.Date.Format(time.RFC1123)))
			baseView = lipgloss.JoinVertical(lipgloss.Left, title, header, content, help)
		} else {
			help := helpStyle.Render("↑/↓: navigate • enter: read • d: delete • q: quit")
			if m.loading {
				help += " (loading more...)"
			}
			content := baseStyle.Width(m.width).Height(m.height - 4).Render(m.table.View())
			baseView = lipgloss.JoinVertical(lipgloss.Left, title, content, help)
		}
	case viewState:
		help := helpStyle.Render("↑/↓: scroll • esc/q: back • d: delete")
		content := bodyStyle.Width(m.width).Height(m.height - 4).Render(m.viewport.View())
		header := headerStyle.Render(fmt.Sprintf("From: %s | To: %s | Subject: %s | Date: %s",
			m.selectedEmail.From, m.selectedEmail.To, m.selectedEmail.Subject, m.selectedEmail.Date.Format(time.RFC1123)))
		baseView = lipgloss.JoinVertical(lipgloss.Left, title, header, content, help)
	}

	if m.state == bucketSelectionState && m.loading {
		splash := splashStyle.Render(spaceSplash)
		loading := m.spinner.View() + " Loading buckets..."
		content := lipgloss.JoinVertical(lipgloss.Center, splash, loading)
		return lipgloss.JoinVertical(lipgloss.Left, title, baseStyle.Width(m.width).Height(m.height-4).Render(content))
	}

	if (m.state == listState || (m.state == confirmDeleteState && m.isDeleteFromList)) && len(m.emails) == 0 && m.loading {
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

func (m *model) initComponents() {
	columns := []table.Column{
		{Title: "From", Width: 40},
		{Title: "To", Width: 40},
		{Title: "Subject", Width: 50},
		{Title: "Date", Width: 30},
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
}

func (m *model) updateComponents() {
	m.viewport.Width = m.width - 4
	m.viewport.Height = m.height - 6

	m.table.SetWidth(m.width - 4)
	m.table.SetHeight(m.height - 6)

	numColumns := 4
	borderWidth := numColumns + 1
	availableContent := max(0, m.width-4-borderWidth)
	proportions := []float64{0.25, 0.25, 0.35, 0.15}
	mins := []int{20, 20, 30, 16}

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
		{Title: "To", Width: colWidths[1]},
		{Title: "Subject", Width: colWidths[2]},
		{Title: "Date", Width: colWidths[3]},
	}
	m.table.SetColumns(newColumns)

	m.bucketsList.SetWidth(m.width - 4)
	m.bucketsList.SetHeight(m.height - 6)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *model) updateTableRows() {
	rows := []table.Row{}
	for _, e := range m.emails {
		rows = append(rows, table.Row{e.From, e.To, e.Subject, e.Date.Format("2006-01-02 15:04")})
	}
	m.table.SetRows(rows)
}

func (m model) loadBuckets() tea.Cmd {
	m.loading = true
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
	m.loading = true
	return func() tea.Msg {
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(m.bucket),
			Prefix:            aws.String(m.prefix),
			MaxKeys:           aws.Int32(10),
			ContinuationToken: m.continuation,
		}
		paginator := s3.NewListObjectsV2Paginator(m.s3Client, input)

		var newEmails []Email
		var nextContinuation *string
		var hasMore bool

		ctx := context.Background()
		if paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return errorMsg{err}
			}
			for _, obj := range page.Contents {
				email, err := m.fetchAndParseEmail(ctx, *obj.Key)
				if err != nil {
					continue
				}
				newEmails = append(newEmails, *email)
			}
			nextContinuation = page.NextContinuationToken
			hasMore = paginator.HasMorePages()
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

var spaceSplash = strings.TrimSpace(`
   .    '     *     .      *    .     ' 
     .      .     *    .    '   *     .  
*  .     *     .    '    .      *    .   
   '    .     *     .    *    .     '    
.     *    .      *     .     '     *    
     .    '     .     *    .      *    . 
`)

type item struct {
	title string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return "" }
func (i item) FilterValue() string { return i.title }
