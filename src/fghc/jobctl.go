// The command utility for VM job control

package main

import (
	"os"
	"fmt"
	"strings"
	"strconv"
	"io/ioutil"
	"path/filepath"
	"github.com/bu-/magic"
)

// Read all the files from the working directory whose names end with *.pid and obtain
// information about running jobs. This function is called when the -list option is provided
// on the command line, so the working directory and also desired output format should
// be specified before.

func listjobs() {
	wdir := job.wdir
	if len(job.wdir) == 0 {
		wdir = "."
	}
	pids, e := filepath.Glob(filepath.Join(wdir, "*.pid"))
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return
	}
	jis := make([]Jobinfo, len(pids))
	for i := range pids {
		pidsplt := strings.Split(filepath.Base(pids[i]), ".")
		jis[i].id = pidsplt[0]
		jis[i].step = pidsplt[1]
		jis[i].kvmpid = 0
		pidb, e := ioutil.ReadFile(pids[i])
		if e != nil {
			continue
		}
		kvmpid, e := strconv.Atoi(strings.TrimSpace(string(pidb)))
		if e != nil {
			continue
		}
		jis[i].kvmpid = kvmpid
		jis[i].volumes = listvolumes(kvmpid)
		status := filepath.Join(wdir, pidsplt[0] + "." + pidsplt[1] + ".status")
		jst, e := ioutil.ReadFile(status)
		if e != nil {
			continue
		}
		jis[i].status = string(jst)
	}
	for i := range jis {
		fmt.Println(jis[i])
	}
}

// List volumes used by a VM. Read all symlinks from /proc/KVMPID/fd directory
// and select those pointing to regular files.

func listvolumes(kvmpid int) []Volinfo {
	fddir := filepath.Join("/proc", fmt.Sprint(kvmpid), "fd")
	fds, e := filepath.Glob(fddir + "/*")
	if e != nil {
		return []Volinfo{}
	}
	vls := []Volinfo{}
	mgc, e := magic.Open(0)
	if e != nil {
		return []Volinfo{}
	}
	defer mgc.Close()
	e = mgc.Load("/usr/share/misc/magic.mgc")
	if e != nil {
		return []Volinfo{}
	}
	for i := range fds {
		sl, e := filepath.EvalSymlinks(fds[i])
		if e != nil {
			continue;
		}
		vi := Volinfo{}
		vi.path = sl
		mgs, e := mgc.File(sl)
		if e != nil {
			continue
		}
		vi.voltype = guessvol(mgs)
		if vi.voltype == NOTSET {
			continue
		}
		vls = append(vls, vi)
	}
	return vls
}

// Guess volume type from its magic file description. If nothing guessed, return NOTSET.

func guessvol(mgds string) int {
	
	mss := []struct {
		match string
		vtype int } {
		{"QCOW", QCOW2},
		{"ext2", RAW},
	}

	for i := range mss {
		if strings.Contains(mgds, mss[i].match) {
			return mss[i].vtype
		}
	}
	return NOTSET
		
}
