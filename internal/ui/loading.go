package ui

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
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
	fmt.Fprintf(s.out, "%s%s\n", clear, finalMessage)
}

func (s *Spinner) render(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	padding := ""
	if extra := s.lastWidth - len(message); extra > 0 {
		padding = strings.Repeat(" ", extra)
	}
	fmt.Fprintf(s.out, "\r%s%s", message, padding)
	s.lastWidth = len(message)
}

type ProgressBar struct {
	out   io.Writer
	label string
	total int

	mu        sync.Mutex
	lastWidth int
}

func NewProgressBar(out io.Writer, label string, total int) *ProgressBar {
	bar := &ProgressBar{
		out:   out,
		label: strings.TrimSpace(label),
		total: total,
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

	message := p.format(current)
	padding := ""
	if extra := p.lastWidth - len(message); extra > 0 {
		padding = strings.Repeat(" ", extra)
	}
	fmt.Fprintf(p.out, "\r%s%s", message, padding)
	p.lastWidth = len(message)
}

func (p *ProgressBar) Finish(finalMessage string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	clear := "\r" + strings.Repeat(" ", p.lastWidth) + "\r"
	if finalMessage == "" {
		fmt.Fprint(p.out, clear)
		return
	}
	fmt.Fprintf(p.out, "%s%s\n", clear, finalMessage)
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
