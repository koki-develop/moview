package ui

import (
	"image"
	_ "image/jpeg"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fatih/color"
	"github.com/koki-develop/moview/internal/ascii"
	"github.com/koki-develop/moview/internal/ffmpeg"
	"github.com/koki-develop/moview/internal/resize"
	"github.com/koki-develop/moview/internal/util"
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

	progress progress.Model

	resizer   *resize.Resizer
	converter *ascii.Converter

	path           string
	current        int
	currentPercent float64

	state        modelState
	windowHeight int
	windowWidth  int

	frameRate float64
	images    []image.Image
	imagesDir string
}

func newModel(opt *Option) *model {
	return &model{
		progress: progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage()),

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
	return m.currentAsciiView() + "\n" + m.progress.ViewAs(m.currentPercent) + "\n\n" + m.helpView()
}

func (m *model) playingView() string {
	return m.currentAsciiView() + "\n" + m.progress.ViewAs(m.currentPercent) + "\n\n" + m.helpView()
}

func (m *model) currentAsciiView() string {
	img := m.resizer.Resize(m.images[m.current], m.windowWidth-2, m.windowHeight-4)
	ascii, err := m.converter.ImageToASCII(img)
	if err != nil {
		return err.Error()
	}

	leftPad := strings.Repeat(" ", util.Max(0, (m.windowWidth-img.Bounds().Max.X)/2))
	b := new(strings.Builder)
	for _, line := range ascii {
		b.WriteString(leftPad)
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (m *model) helpView() string {
	b := new(strings.Builder)
	b.WriteString(strings.Repeat(" ", util.Max(0, (m.windowWidth-36)/2)))

	b.WriteString("← 10s | ")

	switch m.state {
	case modelStatePlaying:
		b.WriteString(color.New(color.BgRed, color.FgWhite).Sprintf(" ⏸︎ "))
	case modelStatePaused:
		b.WriteString(color.New(color.BgGreen, color.FgBlack).Sprintf(" ▶︎ "))
	}
	b.WriteString(" Space/Enter")

	b.WriteString(" | 10s →")

	return b.String()
}

type modelState string

const (
	modelStateLoading modelState = "loading"
	modelStatePlaying modelState = "playing"
	modelStatePaused  modelState = "paused"
)

type errMsg struct{ error }
type loadMsg struct {
	frameRate float64
	images    []image.Image
	imagesDir string
}
type playMsg struct{}
type pauseMsg struct{}
type nextMsg struct{}
type jumpMsg struct{ step int }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, m.quit()
		case tea.KeySpace, tea.KeyEnter:
			switch m.state {
			case modelStatePlaying:
				return m, m.pause()
			case modelStatePaused:
				return m, m.play()
			}
		case tea.KeyRight:
			return m, m.forward()
		case tea.KeyLeft:
			return m, m.back()
		}

	case errMsg:
		m.err = msg.error
		return m, m.quit()

	case tea.WindowSizeMsg:
		m.windowHeight = msg.Height
		m.windowWidth = msg.Width
		m.progress.Width = msg.Width
		return m, nil

	case loadMsg:
		m.frameRate = msg.frameRate
		m.images = msg.images
		m.imagesDir = msg.imagesDir
		m.state = modelStatePlaying
		return m, tea.Batch(m.pause(), tea.EnterAltScreen)

	case playMsg:
		m.state = modelStatePlaying
		if m.current == len(m.images)-1 {
			m.current = 0
		}
		return m, m.next()

	case nextMsg:
		if m.current < len(m.images)-1 {
			m.current++
		}
		if m.current >= len(m.images)-1 {
			m.currentPercent = 1
			return m, m.pause()
		}
		m.currentPercent = float64(m.current) / float64(len(m.images))
		return m, m.next()

	case pauseMsg:
		m.state = modelStatePaused
		return m, nil

	case jumpMsg:
		m.current = util.Min(len(m.images)-1, util.Max(0, msg.step))
		m.currentPercent = float64(m.current) / float64(len(m.images))
		return m, nil
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

		return loadMsg{probe.FrameRate, imgs, dir}
	}
}

func (m *model) play() tea.Cmd {
	return func() tea.Msg { return playMsg{} }
}

func (m *model) next() tea.Cmd {
	return tea.Tick(time.Second/time.Duration(m.frameRate), func(t time.Time) tea.Msg {
		if m.state == modelStatePlaying {
			return nextMsg{}
		}
		return nil
	})
}

func (m *model) pause() tea.Cmd {
	return func() tea.Msg { return pauseMsg{} }
}

func (m *model) forward() tea.Cmd {
	return func() tea.Msg {
		return jumpMsg{m.current + int(5*m.frameRate)}
	}
}

func (m *model) back() tea.Cmd {
	return func() tea.Msg {
		return jumpMsg{m.current - int(5*m.frameRate)}
	}
}
