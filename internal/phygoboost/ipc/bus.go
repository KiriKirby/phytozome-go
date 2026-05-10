package ipc

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/goccy/go-json"
)

type Message struct {
	ID      string `json:"id,omitempty"`
	Task    string `json:"task,omitempty"`
	Payload []byte `json:"payload,omitempty"`
	Output  []byte `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	Type    string `json:"type,omitempty"`
	Network map[string]int `json:"network,omitempty"`
}

type Bus struct {
	mu     sync.Mutex
	reader *bufio.Reader
	writer *bufio.Writer
}

func New(reader io.Reader, writer io.Writer) *Bus {
	return &Bus{
		reader: bufio.NewReader(reader),
		writer: bufio.NewWriter(writer),
	}
}

func (b *Bus) Send(_ context.Context, msg Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.writer == nil {
		return fmt.Errorf("ipc writer is nil")
	}
	encoded, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := b.writer.Write(encoded); err != nil {
		return err
	}
	if err := b.writer.WriteByte('\n'); err != nil {
		return err
	}
	return b.writer.Flush()
}

func (b *Bus) Receive(_ context.Context) (Message, error) {
	var msg Message
	if b.reader == nil {
		return msg, fmt.Errorf("ipc reader is nil")
	}
	line, err := b.reader.ReadBytes('\n')
	if err != nil {
		return msg, err
	}
	err = json.Unmarshal(line, &msg)
	return msg, err
}
