# Go CLI Library

Package `tsingmuhe/cli` is a zero-dependency Go library for implementing command-line interfaces.

## Install

```
go get github.com/tsingmuhe/cli
```

## Example

```
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tsingmuhe/cli"
)

func main() {
	cmd := &cli.Command{
		UsageLine: "foo",
		Long:      "the long description of foo",
		Commands: []*cli.Command{{
			UsageLine: "foo bar [arguments]",
			Short:     "the short description of bar",
			Long:      "the long description of bar",
			Run: func(ctx context.Context, cmd *cli.Command, args []string) error {
				fmt.Println("bar!")
				return nil
			},
		}},
	}

	code := cli.Run(cmd, os.Args[1:])
	os.Exit(code)
}
```