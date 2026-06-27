package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/phillip-england/harbor/internal/docker"
)

type view int

const (
	viewStatus view = iota
	viewContainers
	viewDockerfile
	viewRun
	viewLogs
)

type statusMsg docker.Status
type containersMsg []docker.Container
type logsMsg string
type actionMsg string
type installEventMsg struct {
	line    string
	done    bool
	message string
	err     error
}
type errMsg error

type containerItem docker.Container

func (i containerItem) Title() string {
	name := docker.Container(i).Name
	if name == "" {
		name = docker.Container(i).ID
	}
	return name
}

func (i containerItem) Description() string {
	c := docker.Container(i)
	if c.Ports != "" {
		return fmt.Sprintf("%s  %s  %s", c.Image, c.Status, c.Ports)
	}
	return fmt.Sprintf("%s  %s", c.Image, c.Status)
}

func (i containerItem) FilterValue() string {
	c := docker.Container(i)
	return c.ID + " " + c.Name + " " + c.Image + " " + c.Status
}

type Model struct {
	current    view
	width      int
	height     int
	status     docker.Status
	statusSet  bool
	containers []docker.Container
	list       list.Model
	inputs     []textinput.Model
	focus      int
	message    string
	err        error
	logs       string
	installing bool
	installCh  chan installEventMsg
	installLog []string
	all        bool
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	boxStyle   = lipgloss.NewStyle().Padding(1, 2)
)

func New() Model {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Containers"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	return Model{
		current: viewStatus,
		list:    l,
		all:     true,
		inputs:  dockerfileInputs(),
	}
}

func (m Model) Init() tea.Cmd {
	return checkStatusCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	consumed := false

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(max(20, msg.Width-4), max(8, msg.Height-9))
	case statusMsg:
		m.status = docker.Status(msg)
		m.statusSet = true
	case containersMsg:
		m.containers = []docker.Container(msg)
		items := make([]list.Item, 0, len(m.containers))
		for _, c := range m.containers {
			items = append(items, containerItem(c))
		}
		cmds = append(cmds, m.list.SetItems(items))
	case logsMsg:
		m.logs = string(msg)
		m.current = viewLogs
	case actionMsg:
		m.message = string(msg)
		m.err = nil
		cmds = append(cmds, listContainersCmd(m.all))
	case installEventMsg:
		if msg.line != "" {
			m.installLog = append(m.installLog, msg.line)
			if len(m.installLog) > 200 {
				m.installLog = m.installLog[len(m.installLog)-200:]
			}
		}
		if msg.done {
			m.installing = false
			m.installCh = nil
			if msg.err != nil {
				m.err = msg.err
			} else {
				m.message = msg.message
				m.err = nil
				cmds = append(cmds, checkStatusCmd())
			}
		} else if m.installCh != nil {
			cmds = append(cmds, waitInstallEventCmd(m.installCh))
		}
	case errMsg:
		m.err = error(msg)
	case tea.KeyMsg:
		var cmd tea.Cmd
		cmd, consumed = m.handleKey(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if consumed {
		return m, tea.Batch(cmds...)
	}

	if m.current == viewContainers {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.current == viewDockerfile || m.current == viewRun {
		var cmd tea.Cmd
		for i := range m.inputs {
			m.inputs[i], cmd = m.inputs[i].Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "q":
		return tea.Quit, true
	case "1":
		m.current = viewStatus
		m.message = ""
		m.err = nil
		return checkStatusCmd(), true
	case "2":
		m.current = viewContainers
		m.message = ""
		m.err = nil
		return listContainersCmd(m.all), true
	case "3":
		m.current = viewDockerfile
		m.inputs = dockerfileInputs()
		m.focus = 0
		focusInput(m.inputs, m.focus)
		return nil, true
	case "4":
		m.current = viewRun
		m.inputs = runInputs()
		m.focus = 0
		focusInput(m.inputs, m.focus)
		return nil, true
	case "r":
		return tea.Batch(checkStatusCmd(), listContainersCmd(m.all)), true
	case "i":
		if m.current == viewStatus && m.statusSet && !m.status.Running && !m.installing {
			m.message = ""
			m.err = nil
			m.installing = true
			m.installLog = []string{"Starting Docker installation..."}
			m.installCh = make(chan installEventMsg, 100)
			return installDockerCmd(m.installCh), true
		}
	case "esc":
		m.current = viewStatus
		m.message = ""
		m.err = nil
		return nil, true
	case "tab", "shift+tab", "up", "down":
		if m.current == viewDockerfile || m.current == viewRun {
			if msg.String() == "up" || msg.String() == "shift+tab" {
				m.focus--
			} else {
				m.focus++
			}
			if m.focus < 0 {
				m.focus = len(m.inputs) - 1
			}
			if m.focus >= len(m.inputs) {
				m.focus = 0
			}
			focusInput(m.inputs, m.focus)
			return nil, true
		}
	case "enter":
		switch m.current {
		case viewDockerfile:
			return m.createDockerfile(), true
		case viewRun:
			return m.runContainer(), true
		case viewContainers:
			return m.logsForSelected(), true
		}
	case "s":
		if m.current == viewContainers {
			return m.stopSelected(), true
		}
	case "S":
		if m.current == viewContainers {
			return m.startSelected(), true
		}
	case "x":
		if m.current == viewContainers {
			return m.removeSelected(), true
		}
	case "a":
		if m.current == viewContainers {
			m.all = !m.all
			return listContainersCmd(m.all), true
		}
	}
	return nil, false
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Harbor"))
	b.WriteString(" ")
	b.WriteString(mutedStyle.Render("Docker from the terminal"))
	b.WriteString("\n")
	b.WriteString(m.nav())
	b.WriteString("\n\n")

	switch m.current {
	case viewStatus:
		b.WriteString(m.renderStatus())
	case viewContainers:
		b.WriteString(m.renderContainers())
	case viewDockerfile:
		b.WriteString(m.renderForm("Create Dockerfile", "enter create", "Project path, base image, and command", m.inputs))
	case viewRun:
		b.WriteString(m.renderForm("Run Container", "enter run", "Image, optional name, and comma-separated ports", m.inputs))
	case viewLogs:
		b.WriteString(m.renderLogs())
	}

	if m.message != "" {
		b.WriteString("\n\n")
		b.WriteString(okStyle.Render(m.message))
	}
	if m.err != nil {
		b.WriteString("\n\n")
		b.WriteString(errStyle.Render(m.err.Error()))
	}
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("q quit  esc back  r refresh"))
	return boxStyle.Render(b.String())
}

func (m Model) nav() string {
	items := []string{"1 Status", "2 Containers", "3 Dockerfile", "4 Run"}
	for i, item := range items {
		if view(i) == m.current {
			items[i] = titleStyle.Render(item)
		} else {
			items[i] = mutedStyle.Render(item)
		}
	}
	return strings.Join(items, "  ")
}

func (m Model) renderStatus() string {
	if !m.statusSet {
		return "Checking Docker..."
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Docker Status"))
	if m.status.Installed {
		lines = append(lines, okStyle.Render("CLI: installed"))
	} else {
		lines = append(lines, errStyle.Render("CLI: not installed"))
	}
	if m.status.Running {
		lines = append(lines, okStyle.Render("Daemon: running"))
	} else {
		lines = append(lines, errStyle.Render("Daemon: unavailable"))
	}
	if m.status.Version != "" {
		lines = append(lines, "Version: "+m.status.Version)
	}
	lines = append(lines, "", m.status.Message, "", docker.InstallHint())
	if !m.status.Running {
		lines = append(lines, "", mutedStyle.Render("i install Docker components"))
	}
	if len(m.installLog) > 0 {
		lines = append(lines, "", titleStyle.Render("Installation Details"))
		details := m.installLog
		if len(details) > installDetailLimit(m.height) {
			details = details[len(details)-installDetailLimit(m.height):]
		}
		for _, line := range details {
			lines = append(lines, mutedStyle.Render(line))
		}
		if m.installing {
			lines = append(lines, mutedStyle.Render("Installing..."))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderContainers() string {
	mode := "all"
	if !m.all {
		mode = "running"
	}
	header := mutedStyle.Render("enter logs  s stop  S start  x remove  a toggle all/running (" + mode + ")")
	return header + "\n\n" + m.list.View()
}

func (m Model) renderForm(title, submit, help string, inputs []textinput.Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(help))
	b.WriteString("\n\n")
	for _, input := range inputs {
		b.WriteString(input.View())
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("tab next  " + submit))
	return b.String()
}

func (m Model) renderLogs() string {
	body := strings.TrimSpace(m.logs)
	if body == "" {
		body = "(no logs)"
	}
	return titleStyle.Render("Container Logs") + "\n\n" + body
}

func (m Model) selectedContainer() (docker.Container, bool) {
	item, ok := m.list.SelectedItem().(containerItem)
	if !ok {
		return docker.Container{}, false
	}
	return docker.Container(item), true
}

func (m Model) logsForSelected() tea.Cmd {
	c, ok := m.selectedContainer()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		out, err := docker.Logs(context.Background(), c.ID)
		if err != nil {
			return errMsg(err)
		}
		return logsMsg(out)
	}
}

func (m Model) stopSelected() tea.Cmd {
	c, ok := m.selectedContainer()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		if err := docker.StopContainer(context.Background(), c.ID); err != nil {
			return errMsg(err)
		}
		return actionMsg("Stopped " + c.Name)
	}
}

func (m Model) startSelected() tea.Cmd {
	c, ok := m.selectedContainer()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		if err := docker.StartContainer(context.Background(), c.ID); err != nil {
			return errMsg(err)
		}
		return actionMsg("Started " + c.Name)
	}
}

func (m Model) removeSelected() tea.Cmd {
	c, ok := m.selectedContainer()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		if err := docker.RemoveContainer(context.Background(), c.ID); err != nil {
			return errMsg(err)
		}
		return actionMsg("Removed " + c.Name)
	}
}

func (m Model) createDockerfile() tea.Cmd {
	path := strings.TrimSpace(m.inputs[0].Value())
	base := strings.TrimSpace(m.inputs[1].Value())
	command := strings.TrimSpace(m.inputs[2].Value())
	if path == "" {
		path = "."
	}
	if base == "" {
		base = "alpine:latest"
	}
	if command == "" {
		command = `["sh"]`
	}

	return func() tea.Msg {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return errMsg(err)
		}
		content := fmt.Sprintf("FROM %s\n\nWORKDIR /app\nCOPY . .\n\nCMD %s\n", base, command)
		target := strings.TrimRight(path, "/") + "/Dockerfile"
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return errMsg(err)
		}
		return actionMsg("Created " + target)
	}
}

func (m Model) runContainer() tea.Cmd {
	opts := docker.RunOptions{
		Image:  m.inputs[0].Value(),
		Name:   m.inputs[1].Value(),
		Ports:  m.inputs[2].Value(),
		Detach: strings.ToLower(strings.TrimSpace(m.inputs[3].Value())) != "false",
	}
	return func() tea.Msg {
		out, err := docker.RunContainer(context.Background(), opts)
		if err != nil {
			return errMsg(err)
		}
		return actionMsg("Container launched: " + strings.TrimSpace(out))
	}
}

func checkStatusCmd() tea.Cmd {
	return func() tea.Msg {
		return statusMsg(docker.CheckStatus(context.Background()))
	}
}

func listContainersCmd(all bool) tea.Cmd {
	return func() tea.Msg {
		containers, err := docker.ListContainers(context.Background(), all)
		if err != nil {
			return errMsg(err)
		}
		return containersMsg(containers)
	}
}

func installDockerCmd(ch chan installEventMsg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			msg, err := docker.InstallDockerWithOutput(context.Background(), func(line string) {
				ch <- installEventMsg{line: line}
			})
			ch <- installEventMsg{done: true, message: msg, err: err}
			close(ch)
		}()
		return <-ch
	}
}

func waitInstallEventCmd(ch <-chan installEventMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return installEventMsg{done: true}
		}
		return msg
	}
}

func installDetailLimit(height int) int {
	if height <= 0 {
		return 12
	}
	limit := height - 18
	if limit < 6 {
		return 6
	}
	if limit > 18 {
		return 18
	}
	return limit
}

func dockerfileInputs() []textinput.Model {
	return []textinput.Model{
		newInput("project path", "."),
		newInput("base image", "alpine:latest"),
		newInput("cmd", "[\"sh\"]"),
	}
}

func runInputs() []textinput.Model {
	return []textinput.Model{
		newInput("image", "nginx:latest"),
		newInput("name", "harbor-nginx"),
		newInput("ports", "8080:80"),
		newInput("detach true/false", "true"),
	}
}

func newInput(placeholder, value string) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.Prompt = placeholder + ": "
	input.SetValue(value)
	input.CharLimit = 200
	return input
}

func focusInput(inputs []textinput.Model, focus int) {
	for i := range inputs {
		if i == focus {
			inputs[i].Focus()
			continue
		}
		inputs[i].Blur()
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
