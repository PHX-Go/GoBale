package middleware

import (
	"fmt"
)

func Recovery(onError func(err error)) func(next func()) {
	return func(next func()) {
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("recovered from panic: %v", r)
				if onError != nil {
					onError(err)
				}
			}
		}()
		next()
	}
}