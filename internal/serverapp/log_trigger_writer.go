package serverapp

import (
	"io"
	"log"
	"sync"
)

type triggerWriter struct {
	target  io.Writer
	trigger func()
}

func (w *triggerWriter) Write(p []byte) (int, error) {
	n, err := w.target.Write(p)
	if n > 0 && w.trigger != nil {
		w.trigger()
	}
	return n, err
}

func installLogTrigger(trigger func()) func() {
	original := log.Writer()
	if original == nil {
		return func() {}
	}

	log.SetOutput(&triggerWriter{
		target:  original,
		trigger: trigger,
	})

	var once sync.Once
	return func() {
		once.Do(func() {
			log.SetOutput(original)
		})
	}
}
