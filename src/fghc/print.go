// The command utility for VM job control

package main

import (
	"os"
	"fmt"
	"strings"
	"text/tabwriter"
)

// Print out the list of job info structures. Depending on the print mode given,
// produce in the proper format.

func prtjobs(jis []Jobinfo, prtmode string) {
	switch(prtmode) {
		default:
			fmt.Println("Printing jobs: unknown print mode " + prtmode)
			return
		case "text":
			prtext(jis)
	}
}

// Print jobs in text mode in multiple columns

func prtext(jis []Jobinfo) {
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 8, 1, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "Id\tStep\tStatus\tRunning\t")
	for _, j := range jis {
		fmt.Fprintln(w, j.id + "\t" + j.step + "\t" + strings.TrimSpace(j.status) + "\t" + fmt.Sprint(!j.stalled) + "\t")
	}
	w.Flush()
}
