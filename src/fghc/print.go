// The command utility for VM job control

package main

import (
	"io"
	"os"
	"fmt"
	"strings"
	"text/tabwriter"
)

// Print out the log of the job. Depending on the print mode given,
// produce in the proper format.

func prtlog(r io.Reader, prtmode string) {
	switch(prtmode) {
		default:
			fmt.Println("Printing jobs: unknown print mode " + prtmode)
			return
		case "text":
			io.Copy(os.Stdout, r)
	}
}

// Print out the list of job info structures. Depending on the print mode given,
// produce in the proper format.

func prtjobs(jis []Jobinfo, prtmode string) {
	switch(prtmode) {
		default:
			fmt.Println("Printing jobs: unknown print mode " + prtmode)
			return
		case "text":
			prjtext(jis)
	}
}

// Print jobs in text mode in multiple columns.

func prjtext(jis []Jobinfo) {
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 8, 1, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "Id\tStep\tStatus\tRunning\t")
	for _, j := range jis {
		fmt.Fprintln(w, j.id + "\t" + j.step + "\t" + 
				strings.TrimSpace(j.status) + "\t" + 
				fmt.Sprint(!j.stalled) + "\t")
	}
	w.Flush()
}

// Print out the list of library mappings if any. Depending on the print mode given,
// produce in the proper format.

func prtlibs(lpx *LibPrefix, prtmode string) {
	switch(prtmode) {
		default:
			fmt.Println("Printing libraries: unknown print mode " + prtmode)
			return
		case "text":
			prltext(lpx)
	}
}

// Print libs in text mode in multiple columns.

func prltext(lpx *LibPrefix) {
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 8, 1, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "VM Path\tHost path\tWritable\t")
	for k, v := range *lpx {
		fmt.Fprintln(w, k + "\t" + v.Hostpath + "\t" + 
				fmt.Sprint(v.Write) + "\t")
	}
	w.Flush()
}
