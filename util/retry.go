package util

import "time"

func Retry(fn func() error) error {
	delayBetweenRetries := 500 * time.Millisecond
	maxRetries := 10
	try := 0
	for {
		try++
		err := fn()
		if err == nil {
			return nil
		}
		if try > maxRetries {
			return err
		}
		time.Sleep(delayBetweenRetries)
	}
}
