package logging

import (
	"fmt"
	"os"
	"time"
)

var (
	Verbose bool
)

func Fatal(c chan struct{}, args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	close(c)
	time.Sleep(10 * time.Second)
	os.Exit(1)
}

func Debug(args ...interface{}) {
	if Verbose {
		fmt.Fprintln(os.Stderr, args...)
	}
}
