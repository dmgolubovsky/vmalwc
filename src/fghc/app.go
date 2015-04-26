package main

import (
	"os"
	"fmt"
	"net/url"
	"github.com/dmgolubovsky/mini"
)

/*
 * This function parses the desktop entry file if it is provided, filling in some information
 * as if it were obtained from the command line options (and the subsequent command line options
 * can override the desktop entry file settings.
 * 
 * The desktop entry parser expects group headers identifying settings for the whole job,
 * for each step, for each library within step.
 * 
 */

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
		switch(sect[s]) {
			case "Desktop Entry":
				continue
			case "Job":
				processjob()
			default:
		}
	}
}

func processjob() {
}
