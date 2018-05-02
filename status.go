package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alphagov/paas-cf-conduit/logging"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

func NewStatus(w io.Writer) *Status {
	s := &Status{
		spin: spinner.New(spinner.CharSets[14], 250*time.Millisecond),
	}
	s.spin.Writer = os.Stderr
	s.spin.Prefix = ""
	s.spin.Suffix = ""
	return s
}

type Status struct {
	spin *spinner.Spinner
}

func (s *Status) Text(args ...interface{}) {
	if s.spin.Suffix != "" {
		s.Done()
	}
	msg := fmt.Sprintln(args...)
	msg = msg[:len(msg)-1]
	if logging.Verbose || NonInteractive {
		logging.Debug(msg)
	} else {
		s.spin.Suffix = " " + msg
		s.spin.Start()
	}
}

func (s *Status) Done() {
	if s.spin.Suffix != "" {
		s.spin.FinalMSG = color.GreenString("OK") + s.spin.Suffix + "\n"
	}
	s.spin.Stop()
	s.spin.Suffix = ""

}
