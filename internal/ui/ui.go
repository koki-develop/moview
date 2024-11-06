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
	Path       string
	AutoPlay   bool
	AutoRepeat bool
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
	spinner  spinner.Model

	resizer   *resize.Resizer
	converter *ascii.Converter

	autoPlay   bool
	autoRepeat bool

	path    string
	current int

	state        modelState
	windowHeight int
	windowWidth  int

	frameRate  float64
	imagesDir  string
	imagePaths []string

	asciisCache     []string
	shouldRepreload bool

	quitting bool
}

func newModel(opt *Option) *model {
	return &model{
		autoPlay:   opt.AutoPlay,
		autoRepeat: opt.AutoRepeat,

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
	case modelStatePlaying, modelStatePaused:
		return m.mainView()
	case modelStateCleanup:
		return m.cleanupView()
	}

	return ""
}

func (m *model) loadingMetadataView() string {
	return m.spinnerView("Loading video metadata...")
}

func (m *model) extractingImagesView() string {
	return m.spinnerView("Extracting frames...")
}

func (m *model) spinnerView(text string) string {
	b := new(strings.Builder)
	b.WriteString(m.spinner.View())
	b.WriteString(text)
	return b.String()
}

func (m *model) mainView() string {
	b := new(strings.Builder)
	b.WriteString(m.currentAsciiView())
	b.WriteString("\n")
	b.WriteString(m.progressView())
	b.WriteString("\n\n")
	b.WriteString(m.helpView())
	return b.String()
}

func (m *model) progressView() string {
	currentPercent := float64(m.current) / float64(len(m.imagePaths))
	if m.current == len(m.imagePaths)-1 {
		currentPercent = 1
	}

	totalSeconds := float64(len(m.imagePaths)) / m.frameRate
	currentSeconds := float64(m.current) / m.frameRate
	progressText := fmt.Sprintf(
		"%02d:%02d / %02d:%02d",
		int(currentSeconds)/60, int(currentSeconds)%60,
		int(totalSeconds)/60, int(totalSeconds)%60,
	)

	padLeft := strings.Repeat(" ", util.Max(0, (m.windowWidth-len(progressText))/2))

	return m.progress.ViewAs(currentPercent) + "\n" + padLeft + progressText
}

func (m *model) currentAsciiView() string {
	if m.asciisCache[m.current] != "" {
		return m.asciisCache[m.current]
	}
	v, err := m.asciiView(m.current)
	if err != nil {
		return err.Error()
	}
	m.asciisCache[m.current] = v
	return v
}

func (m *model) loadImage(index int) (image.Image, error) {
	f, err := os.Open(m.imagePaths[index])
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	return img, nil
}

func (m *model) asciiView(index int) (string, error) {
	img, err := m.loadImage(index)
	if err != nil {
		return "", err
	}

	img = m.resizer.Resize(img, m.windowWidth-2, m.windowHeight-5)
	ascii, err := m.converter.ImageToASCII(img)
	if err != nil {
		return "", err
	}

	leftPad := strings.Repeat(" ", util.Max(0, (m.windowWidth-img.Bounds().Max.X)/2))
	b := new(strings.Builder)
	for _, line := range ascii {
		b.WriteString(leftPad)
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String(), nil
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

func (m *model) cleanupView() string {
	return m.spinnerView("Cleaning up...")
}

type modelState int

const (
	_ modelState = iota
	modelStateLoadingMetadata
	modelStateExtractingImages
	modelStatePlaying
	modelStatePaused
	modelStateCleanup
)

type errMsg struct{ error }
type metadataMsg struct {
	frameRate float64
	imagesDir string
}
type extractImagesMsg struct {
	paths []string
}
type repreloadMsg struct{}
type playMsg struct{}
type pauseMsg struct{}
type nextMsg struct{}
type jumpMsg struct{ step int }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	if m.quitting {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			m.state = modelStateCleanup
			return m, tea.Batch(m.cleanup(), tea.ExitAltScreen)
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
		m.state = modelStateCleanup
		return m, tea.Batch(m.cleanup(), tea.ExitAltScreen)

	case tea.WindowSizeMsg:
		if m.quitting {
			return m, nil
		}

		m.windowHeight = msg.Height
		m.windowWidth = msg.Width
		m.progress.Width = msg.Width
		if len(m.imagePaths) > 0 {
			m.asciisCache = make([]string, len(m.imagePaths))
			m.shouldRepreload = true
		}
		return m, nil

	case metadataMsg:
		m.frameRate = msg.frameRate
		m.imagesDir = msg.imagesDir
		m.state = modelStateExtractingImages
		return m, m.extractImages(msg.imagesDir)

	case extractImagesMsg:
		m.imagePaths = msg.paths
		m.asciisCache = make([]string, len(msg.paths))
		m.state = modelStatePlaying
		cmds := []tea.Cmd{m.preloadAsciis(), tea.EnterAltScreen}
		if m.autoPlay {
			cmds = append(cmds, m.play())
		} else {
			cmds = append(cmds, m.pause())
		}
		return m, tea.Batch(cmds...)

	case playMsg:
		m.state = modelStatePlaying
		if m.current == len(m.imagePaths)-1 {
			m.current = 0
		}
		return m, m.next()

	case nextMsg:
		if m.current < len(m.imagePaths)-1 {
			m.current++
		}
		if m.current >= len(m.imagePaths)-1 {
			if m.autoRepeat {
				m.current = 0
			} else {
				return m, m.pause()
			}
		}
		return m, m.next()

	case pauseMsg:
		m.state = modelStatePaused
		return m, nil

	case jumpMsg:
		m.current = util.Min(len(m.imagePaths)-1, util.Max(0, msg.step))
		m.shouldRepreload = true
		return m, nil

	case repreloadMsg:
		return m, m.preloadAsciis()
	}

	return m, nil
}

func (m *model) cleanup() tea.Cmd {
	return func() tea.Msg {
		if len(m.imagePaths) > 0 {
			// NOTE: Using os.RemoveAll alone can fail sometimes, so delete files one by one
			for _, path := range m.imagePaths {
				if err := os.Remove(path); err != nil {
					return errMsg{err}
				}
			}

			if err := os.RemoveAll(m.imagesDir); err != nil {
				return errMsg{err}
			}
		}
		return tea.Quit()
	}
}

func (m *model) loadMetadata() tea.Cmd {
	return func() tea.Msg {
		probe, err := ffmpeg.FFProbe(m.path)
		if err != nil {
			return errMsg{err}
		}

		dir, err := os.MkdirTemp("", "moview")
		if err != nil {
			return errMsg{err}
		}

		return metadataMsg{probe.FrameRate, dir}
	}
}

func (m *model) extractImages(dir string) tea.Cmd {
	return func() tea.Msg {
		paths, err := ffmpeg.MovieToImages(m.path, dir)
		if err != nil {
			return errMsg{err}
		}
		return extractImagesMsg{paths}
	}
}

func (m *model) preloadAsciis() tea.Cmd {
	return func() tea.Msg {
		for i := m.current; i < len(m.imagePaths); i++ {
			if m.shouldRepreload {
				m.shouldRepreload = false
				return repreloadMsg{}
			}
			if m.quitting {
				m.quitting = false
				return nil
			}

			if m.asciisCache[i] != "" {
				continue
			}

			if v, err := m.asciiView(i); err == nil {
				m.asciisCache[i] = v
			}
		}
		return nil
	}
}

func (m *model) play() tea.Cmd {
	return func() tea.Msg { return playMsg{} }
}

func (m *model) next() tea.Cmd {
	// NOTE: Play 2% faster to account for rendering delay
	return tea.Tick(time.Duration(float64(time.Second)/m.frameRate*0.98), func(t time.Time) tea.Msg {
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
		return jumpMsg{m.current + int(10*m.frameRate)}
	}
}

func (m *model) back() tea.Cmd {
	return func() tea.Msg {
		return jumpMsg{m.current - int(10*m.frameRate)}
	}
}
