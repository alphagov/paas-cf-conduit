package logging

import (
	"fmt"
	"os"
)

var (
	Verbose bool
)

func Debug(args ...interface{}) {
	if Verbose {
		fmt.Fprintln(os.Stderr, args...)
	}
}

func Error(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
}
