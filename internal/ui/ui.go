package ui

import (
	"image"
	_ "image/jpeg"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/koki-develop/moview/internal/ascii"
	"github.com/koki-develop/moview/internal/ffmpeg"
	"github.com/koki-develop/moview/internal/resize"
)

type Option struct {
	Path string
}

func Start(opt *Option) error {
	m := newModel(opt)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return err
	}

	if m.err != nil {
		return m.err
	}

	return nil
}

var _ tea.Model = &model{}

type model struct {
	err error

	resizer   *resize.Resizer
	converter *ascii.Converter

	path    string
	current int

	state        modelState
	windowHeight int
	windowWidth  int

	tickDuration time.Duration
	images       []image.Image
	imagesDir    string
}

func newModel(opt *Option) *model {
	return &model{
		resizer:   resize.NewResizer(),
		converter: ascii.NewConverter(),

		path:    opt.Path,
		current: 0,
	}
}

func (m *model) Init() tea.Cmd {
	m.state = modelStateLoading
	return m.load()
}

func (m *model) View() string {
	switch m.state {
	case modelStateLoading:
		return m.loadingView()
	case modelStatePlaying:
		return m.playingView()
	case modelStatePaused:
		return m.pausedView()
	}

	return ""
}

func (m *model) loadingView() string {
	// TODO: show progress
	return "loading..."
}

func (m *model) pausedView() string {
	// TODO: show help
	return m.currentAscii()
}

func (m *model) playingView() string {
	// TODO: show help
	return m.currentAscii()
}

func (m *model) currentAscii() string {
	img := m.resizer.Resize(m.images[m.current], m.windowWidth, m.windowHeight-3)
	ascii, err := m.converter.ImageToASCII(img)
	if err != nil {
		return err.Error()
	}
	return strings.Join(ascii, "\n")
}

type modelState string

const (
	modelStateLoading modelState = "loading"
	modelStatePlaying modelState = "playing"
	modelStatePaused  modelState = "paused"
)

type errMsg struct{ error }
type loadMsg struct {
	tickDuration time.Duration
	images       []image.Image
	imagesDir    string
}
type forwardMsg struct{}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, m.quit()
		}

	case errMsg:
		m.err = msg.error
		return m, m.quit()

	case tea.WindowSizeMsg:
		m.windowHeight = msg.Height
		m.windowWidth = msg.Width
		return m, nil

	case loadMsg:
		m.tickDuration = msg.tickDuration
		m.images = msg.images
		m.imagesDir = msg.imagesDir
		m.state = modelStatePlaying
		return m, tea.Batch(m.play(), tea.EnterAltScreen)

	case forwardMsg:
		m.current++
		if m.current == len(m.images)-1 {
			m.state = modelStatePaused
			return m, nil
		}
		return m, m.play()
	}
	return m, nil
}

func (m *model) quit() tea.Cmd {
	if m.imagesDir != "" {
		_ = os.RemoveAll(m.imagesDir)
	}
	return tea.Quit
}

func (m *model) load() tea.Cmd {
	return func() tea.Msg {
		probe, err := ffmpeg.FFProbe(m.path)
		if err != nil {
			return errMsg{err}
		}

		dir, err := os.MkdirTemp("", "moview")
		if err != nil {
			return errMsg{err}
		}
		paths, err := ffmpeg.MovieToImages(m.path, dir)
		if err != nil {
			return errMsg{err}
		}

		imgs := make([]image.Image, 0, len(paths))
		for _, path := range paths {
			f, err := os.Open(path)
			if err != nil {
				return errMsg{err}
			}
			defer f.Close()

			img, _, err := image.Decode(f)
			if err != nil {
				return errMsg{err}
			}
			imgs = append(imgs, img)
		}

		d := time.Second / time.Duration(probe.FrameRate)

		return loadMsg{d, imgs, dir}
	}
}

func (m *model) play() tea.Cmd {
	return tea.Tick(m.tickDuration, func(t time.Time) tea.Msg {
		return forwardMsg{}
	})
}
