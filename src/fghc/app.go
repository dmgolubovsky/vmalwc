package main

import (
	"os"
	"fmt"
	"net/url"
	"github.com/dmgolubovsky/mini"
)

func appconfig() {
	if len(job.desktop) == 0 {
		return
	}
	u, e := url.Parse(job.desktop)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	if u.Scheme != "file" && u.Scheme != "" {
		fmt.Fprintln(os.Stderr, "app: cannot handle scheme " + u.Scheme)
		os.Exit(1)
	}
	c, e := mini.LoadConfiguration(u.Path)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	sect := c.SectionNames()
	for s := range sect {
		fmt.Fprintln(os.Stderr, "["+sect[s]+"]")
	}
}
