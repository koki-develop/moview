package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type Option struct {
	Path string
}

func Start(opt *Option) error {
	p := tea.NewProgram(newModel(opt))
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

var _ tea.Model = &model{}

type model struct {
	path string
}

func newModel(opt *Option) *model {
	return &model{path: opt.Path}
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *model) View() string {
	return "hello, world"
}
