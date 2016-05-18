package main

import (
	"io"
	"os"
)

func init() {
	topics = append(topics, Topics{
		{
			Name:   "debug",
			Hidden: true,
			Commands: Commands{
				{
					Topic:   "debug",
					Command: "errlog",
					Run: func(ctx *Context) {
						f, err := os.Open(ErrLogPath)
						must(err)
						io.CopyBuffer(Stdout, f, make([]byte, 1024))
					},
				},
			},
		},
	}...)
}
