package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []string{"|", "/", "-", "\\"}

type Spinner struct {
	out      io.Writer
	label    string
	interval time.Duration
	done     chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once

	mu        sync.Mutex
	lastWidth int
}

func NewSpinner(out io.Writer, label string) *Spinner {
	return &Spinner{
		out:      out,
		label:    strings.TrimSpace(label),
		interval: 120 * time.Millisecond,
		done:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

func (s *Spinner) Start() {
	go func() {
		defer close(s.stopped)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		frame := 0
		for {
			s.render(fmt.Sprintf("%s %s", spinnerFrames[frame%len(spinnerFrames)], s.label))
			frame++

			select {
			case <-s.done:
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Spinner) Stop(finalMessage string) {
	s.stopOnce.Do(func() {
		close(s.done)
	})
	<-s.stopped

	s.mu.Lock()
	defer s.mu.Unlock()

	clear := "\r" + strings.Repeat(" ", s.lastWidth) + "\r"
	if finalMessage == "" {
		fmt.Fprint(s.out, clear)
		return
	}
	fmt.Fprintf(s.out, "%s%s\n", clear, fitToWidth(finalMessage, s.maxLineWidth()))
}

func (s *Spinner) render(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	message = fitToWidth(message, s.maxLineWidth())
	clear := "\r" + strings.Repeat(" ", s.lastWidth) + "\r"
	fmt.Fprintf(s.out, "%s%s", clear, message)
	s.lastWidth = len(message)
}

func (s *Spinner) maxLineWidth() int {
	return consoleWidth(s.out)
}

type ProgressBar struct {
	out   io.Writer
	label string
	total int

	mu        sync.Mutex
	lastWidth int
	lastDraw  time.Time
	minDraw   time.Duration
	pending   string
}

func NewProgressBar(out io.Writer, label string, total int) *ProgressBar {
	bar := &ProgressBar{
		out:     out,
		label:   strings.TrimSpace(label),
		total:   total,
		minDraw: 120 * time.Millisecond,
	}
	bar.Set(0)
	return bar
}

func (p *ProgressBar) Set(current int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if current < 0 {
		current = 0
	}
	if p.total > 0 && current > p.total {
		current = p.total
	}

	message := fitToWidth(p.format(current), p.maxLineWidth())
	p.pending = message
	if current < p.total && !p.lastDraw.IsZero() && time.Since(p.lastDraw) < p.minDraw {
		return
	}
	p.drawLocked()
}

func (p *ProgressBar) Finish(finalMessage string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	clear := "\r" + strings.Repeat(" ", p.lastWidth) + "\r"
	if finalMessage == "" {
		fmt.Fprint(p.out, clear)
		return
	}
	fmt.Fprintf(p.out, "%s%s\n", clear, fitToWidth(finalMessage, p.maxLineWidth()))
	p.lastWidth = 0
	p.pending = ""
	p.lastDraw = time.Now()
}

func (p *ProgressBar) format(current int) string {
	if p.total <= 0 {
		return p.label
	}

	const width = 24
	ratio := float64(current) / float64(p.total)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * width)
	if filled > width {
		filled = width
	}

	return fmt.Sprintf(
		"%s [%s%s] %d/%d (%3.0f%%)",
		p.label,
		strings.Repeat("#", filled),
		strings.Repeat("-", width-filled),
		current,
		p.total,
		ratio*100,
	)
}

func (p *ProgressBar) maxLineWidth() int {
	return consoleWidth(p.out)
}

func (p *ProgressBar) drawLocked() {
	message := p.pending
	if message == "" {
		message = p.format(0)
	}
	clear := "\r" + strings.Repeat(" ", p.lastWidth) + "\r"
	fmt.Fprintf(p.out, "%s%s", clear, message)
	p.lastWidth = len(message)
	p.lastDraw = time.Now()
}

func consoleWidth(out io.Writer) int {
	file, ok := out.(*os.File)
	if !ok {
		return 80
	}
	if width, _, err := term.GetSize(int(file.Fd())); err == nil && width > 0 {
		return width
	}
	return 80
}

func fitToWidth(message string, width int) string {
	if width <= 0 {
		return message
	}
	runes := []rune(message)
	if len(runes) <= width {
		return message
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}
