package ui

import (
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	m.program = p
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
	program *tea.Program
	err     error

	progress progress.Model
	spinner  spinner.Model

	resizer   *resize.Resizer
	converter *ascii.Converter

	totalFrameCount  int
	loadedFrameCount int

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
		spinner:  spinner.New(spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("205"))), spinner.WithSpinner(spinner.Dot)),

		resizer:   resize.NewResizer(),
		converter: ascii.NewConverter(),

		path:    opt.Path,
		current: 0,
	}
}

func (m *model) Init() tea.Cmd {
	m.state = modelStateLoadingMetadata
	return tea.Batch(m.spinner.Tick, m.loadMetadata())
}

func (m *model) View() string {
	switch m.state {
	case modelStateLoadingMetadata:
		return m.loadingMetadataView()
	case modelStateExtractingImages:
		return m.extractingImagesView()
	case modelStateLoadingImages:
		return m.loadingImagesView()
	case modelStatePlaying:
		return m.playingView()
	case modelStatePaused:
		return m.pausedView()
	}

	return ""
}

func (m *model) loadingMetadataView() string {
	return m.spinner.View() + "Loading video metadata..."
}

func (m *model) extractingImagesView() string {
	return m.spinner.View() + "Extracting frames..."
}

func (m *model) loadingImagesView() string {
	return m.spinner.View() + fmt.Sprintf("Loading frames...(%d/%d)", m.loadedFrameCount, m.totalFrameCount)
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

type modelState int

const (
	_ modelState = iota
	modelStateLoadingMetadata
	modelStateExtractingImages
	modelStateLoadingImages
	modelStatePlaying
	modelStatePaused
)

type errMsg struct{ error }
type metadataMsg struct {
	frameRate float64
}
type extractImagesMsg struct {
	imagesDir string
	paths     []string
}
type loadFrameMsg struct{}
type loadMsg struct {
	images []image.Image
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

	case metadataMsg:
		m.frameRate = msg.frameRate
		m.state = modelStateExtractingImages
		return m, m.extractImages()

	case extractImagesMsg:
		m.imagesDir = msg.imagesDir
		m.totalFrameCount = len(msg.paths)
		m.state = modelStateLoadingImages
		return m, m.load(msg.paths)

	case loadFrameMsg:
		m.loadedFrameCount++
		return m, nil

	case loadMsg:
		m.images = msg.images
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

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) quit() tea.Cmd {
	if m.imagesDir != "" {
		_ = os.RemoveAll(m.imagesDir)
	}
	return tea.Quit
}

func (m *model) loadMetadata() tea.Cmd {
	return func() tea.Msg {
		probe, err := ffmpeg.FFProbe(m.path)
		if err != nil {
			return errMsg{err}
		}
		return metadataMsg{probe.FrameRate}
	}
}

func (m *model) extractImages() tea.Cmd {
	return func() tea.Msg {
		dir, err := os.MkdirTemp("", "moview")
		if err != nil {
			return errMsg{err}
		}
		paths, err := ffmpeg.MovieToImages(m.path, dir)
		if err != nil {
			return errMsg{err}
		}
		return extractImagesMsg{dir, paths}
	}
}

func (m *model) load(paths []string) tea.Cmd {
	return func() tea.Msg {
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

			m.program.Send(loadFrameMsg{})
		}

		return loadMsg{imgs}
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
