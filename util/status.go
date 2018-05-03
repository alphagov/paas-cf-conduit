package util

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alphagov/paas-cf-conduit/logging"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

type Status struct {
	spin           *spinner.Spinner
	nonInteractive bool
}

func NewStatus(w io.Writer, nonInteractive bool) *Status {
	s := &Status{
		spin:           spinner.New(spinner.CharSets[14], 250*time.Millisecond),
		nonInteractive: nonInteractive,
	}
	s.spin.Writer = os.Stderr
	s.spin.Prefix = ""
	s.spin.Suffix = ""
	return s
}

func (s *Status) Text(args ...interface{}) {
	if s.spin.Suffix != "" {
		s.Done()
	}
	msg := fmt.Sprintln(args...)
	msg = msg[:len(msg)-1]
	if logging.Verbose || s.nonInteractive {
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
