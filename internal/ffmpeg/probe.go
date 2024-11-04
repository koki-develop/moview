package ffmpeg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Probe struct {
	FrameRate float64
}

type probe struct {
	Streams []struct {
		RFrameRate string `json:"r_frame_rate"`
	} `json:"streams"`
}

func (p *probe) Probe() (*Probe, error) {
	if len(p.Streams) == 0 {
		return nil, errors.New("no video streams found")
	}

	// r_frame_rate is returned in a format like "30000/1001"
	ns := strings.Split(p.Streams[0].RFrameRate, "/")
	if len(ns) != 2 {
		return nil, fmt.Errorf("invalid r_frame_rate format: %s", p.Streams[0].RFrameRate)
	}

	n, err := strconv.ParseFloat(ns[0], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse numerator: %w", err)
	}

	d, err := strconv.ParseFloat(ns[1], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse denominator: %w", err)
	}

	return &Probe{
		FrameRate: n / d,
	}, nil
}

func FFProbe(path string) (*Probe, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=r_frame_rate", "-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute ffprobe: %w", err)
	}

	var p probe
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ffprobe output: %w", err)
	}
	return p.Probe()
}
