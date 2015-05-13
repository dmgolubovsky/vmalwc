// The command utility for VM job control

package main

import (
	"os"
	"fmt"
	"syscall"
	"strings"
	"strconv"
	"io/ioutil"
	"path/filepath"
	"github.com/bu-/magic"
)

// Purge all files in the working directory related to a particular stalled job or all stalled jobs.
// If -id was previously specified on the command line only the specified job will be purged.
// If a job is not stalled, it cannot be purged.

func purgejobs() {
	wdir := job.wdir
	if len(job.wdir) == 0 {
		wdir = "."
	}
	jis := listjobs()
	for i := range jis {
		if len(job.id) != 0 && jis[i].id != job.id {
			continue
		}
		if !(jis[i].stalled) {
			continue
		}
		files, e := filepath.Glob (filepath.Join(wdir, jis[i].id + "." + jis[i].step + ".*"))
		if e != nil {
			continue
		}
		logger("Purging job " + jis[i].id)
		for j := range files {
			e = os.Remove(files[j])
			if e != nil {
				logger("Remove file " + files[j] + ": " + fmt.Sprint(e))
			}
		}
	}
}

// Stop a job by sending SIGINT to the KVM running it. This causes clean termination,
// and job's temporary files will be cleaned up. Job ID should have been spceified
// earlier in the command line.

func stopjob() {
	termjob(syscall.SIGINT)
}

// Kill a job by sending SIGKILL to the KVM running it. This causes dirty termination,
// and job's temporary files will be preserved in the working directory. Job ID should 
// have been spceified earlier in the command line.

func killjob() {
	termjob(syscall.SIGKILL)
}

// Common part of the stop/kill job facility.

func termjob(sig syscall.Signal) {
	if len(job.id) == 0 {
		return
	}
	jis := listjobs()
	for i := range jis {
		if jis[i].id == job.id {
			syscall.Kill(jis[i].kvmpid, sig)
			return
		}
	}
}

// Read all the files from the working directory whose names end with *.pid and obtain
// information about running jobs. This function is called when the -list option is provided
// on the command line, so the working directory and also desired output format should
// be specified before.

func listjobs() []Jobinfo {
	wdir := job.wdir
	if len(job.wdir) == 0 {
		wdir = "."
	}
	pids, e := filepath.Glob(filepath.Join(wdir, "*.pid"))
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return []Jobinfo{}
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
		kvmexe, e := os.Readlink(filepath.Join("/proc", fmt.Sprint(kvmpid), "exe"))
		jis[i].stalled = !(kvmexe == job.kvm)
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
	return jis
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
