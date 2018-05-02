package logging

import (
	"fmt"
	"os"
	"time"
)

var (
	Verbose bool
)

func Fatal(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	time.Sleep(10 * time.Second)
	os.Exit(1)
}

func Debug(args ...interface{}) {
	if Verbose {
		fmt.Fprintln(os.Stderr, args...)
	}
}
