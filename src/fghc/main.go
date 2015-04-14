// The command utility for VM job control

package main

import (
	"os"
	"fmt"
	"os/user"
	"strings"
	"strconv"
	"path/filepath"
	"bitbucket.org/creachadair/goflags/bytesize"
)


func vbreak () {
	fmt.Println()
	fmt.Println()
}

var job = Job {}

var curstep *Step = nil
var curlib *Library = nil

var cleanafter []string = []string{}

var alltarg []string = []string{}

var savelibs []*Library = []*Library{}

func newlib() *Library {
	lib := Library{}
	var lfptr *[]*Library
	if curstep != nil {
		lfptr = &curstep.libmap
		lib.stname = curstep.name
	} else {
		lfptr = &job.libmap
	}
	*lfptr = append(*lfptr, &lib)
	return &lib
}

func findlib(refname string) *Library {
	if refname[0] != '@' {
		return nil
	}
	stlb := strings.Split(refname[1:], ".")
	var libmap *[]*Library
	var libname string
	switch(len(stlb)) {
		case 0:
			return nil
		case 1:
			libmap = &job.libmap
			libname = stlb[0]
		default:
			step, ok := job.steps[stlb[0]]
			if !ok {
				return nil
			}
			libmap = &step.libmap
			libname = stlb[1]
	}
	for _, l := range(*libmap) {
		if l.name == libname{
			return l
		}
	}
	return nil
}


func main () {
	pargs := append(os.Args[1:], "")
	job.libmap = []*Library{}
	job.steps = StepMap{}
	job.xdisplay = -1
	job.video = false
	job.audio = false
	u, e := user.Current()
	if e != nil {
		job.user = nil
	} else {
		job.user = u
	}
	var skip = false
	for i := range pargs {
		if skip {
			skip = false
			continue
		}
		switch(pargs[i]) {
			default:
				fmt.Fprintln(os.Stderr, "Unknown option: ", pargs[i])
				os.Exit(1)
			case "":
				break
			case "-user":
				if job.user == nil {
					fmt.Fprintln(os.Stderr, "Cannot obtain current user information")
					os.Exit(1)
				}
				job.uimp = true
			case "-audio":
				job.audio = true
			case "-video":
				job.video = true
			case "-kernel":
				i++
				job.kernel = pargs[i]
				skip = true
			case "-kvm":
				i++
				job.kvm = pargs[i]
				skip = true
			case "-workdir":
				i++
				job.wdir = pargs[i]
				skip = true
			case "-mem":
				i++
				bsize, err := bytesize.Parse(pargs[i])
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				if curstep !=nil {
					curstep.mmegs = bsize / (1024 * 1024)
				} else {
					job.mmegs = bsize / (1024 * 1024)
				}
				skip = true
			case "-sysin":
				i++
				if curstep != nil {
					curstep.sysin = pargs[i]
				}
				skip = true
			case "-sysout":
				i++
				if curstep != nil {
					curstep.sysout = pargs[i]
				}
				skip = true
			case "-xdisplay":
				i++
				x, e := strconv.ParseInt(pargs[i], 10, 32)
				if e != nil {
					fmt.Println(os.Stderr, e)
					os.Exit(1)
				}
				job.xdisplay = int(x)
				skip = true
			case "-lib":
				i++
				curlib = newlib()
				curlib.name = pargs[i]
				curlib.tag = curlib.name
				curlib.id = curlib.name
				curlib.snap = false
				if curlib.name[0] == '@' {
					curlib.libtype = REF
					stlb := strings.Split(curlib.name[1:], ".")
					if len(stlb) == 2 {
						curlib.refstep = stlb[0]
					}
				} else {
					curlib.libtype = NOTSET
				}
				skip = true
			case "-tag":
				i++
				if curlib != nil {
					curlib.tag = pargs[i]
					if job.uimp && strings.HasPrefix(curlib.path, job.user.HomeDir) {
						curlib.tag = "H#" + curlib.tag
					}
				}
				skip = true
			case "-path":
				i++
				if curlib != nil {
					curlib.path = pargs[i]
				}
				skip = true
			case "-save":
				i++
				if curlib != nil {
					curlib.save = pargs[i]
				}
				skip = true
			case "-ro":
				if curlib != nil {
					curlib.write = false
				}
			case "-rw":
				if curlib != nil {
					curlib.write = true
				}
			case "-new":
				i++
				if curlib != nil {
					bsize, err := bytesize.Parse(pargs[i])
					if err == nil {
						curlib.newsize = bsize / (1024 * 1024)
					} else {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(1)
					}
				}
				skip = true
			case "-from":
				i++
				if curlib != nil {
					curlib.from = pargs[i]
				}
				skip = true
			case "-snap":
				if curlib != nil {
					curlib.snap = true
				}
			case "-type":
				i++
				if curlib != nil {
					if curlib.libtype != NOTSET {
						fmt.Fprintln(os.Stderr, "library type set already")
						os.Exit(1)
					}
					switch pargs[i] {
						default:
							fmt.Fprintln(os.Stderr, "unknown library type: ", pargs[i])
							os.Exit(1)
						case "raw":
							curlib.libtype = RAW
						case "qcow2":
							curlib.libtype = QCOW2
						case "http":
							curlib.libtype = HTTP
						case "9p":
							curlib.libtype = NINEP
					}
				}
				skip = true
			case "-step":
				i++
				curstep = &Step{}
				job.steps[pargs[i]] = curstep
				curstep.libmap = []*Library{}
				curstep.name = pargs[i]
				curstep.ncons = 2
				skip = true
			case "-exec":
				i++
				if curstep != nil {
					curstep.exec = pargs[i]
				}
				skip = true
			case "-after":
				i++
				if curstep != nil {
					 curstep.add_dep("step_" + pargs[i])
				}
				skip = true
		}
	}

	fmt.Print("all: alltarg cleanafter")
	vbreak()

// Append all steps to the toplevel target.
	
	for s := range job.steps {
		alltarg = append(alltarg, "step_" + job.steps[s].name)
	}
	vbreak()

// For each step if there are new-created libraries, step depends on their creation.
// Each new-created library is an cleanafteriate target to be deleted at the end.
// For each step if there are references to other step's libraries, step depends on
// the step referred to.
// For each library that is new, dump a file creation recipe based on its type.

	for _, s := range job.steps {
		if s.libmap != nil {
			for _, l := range s.libmap {
				if l.save != "" {
					alltarg = append(alltarg, "save_lib_" + l.stname + "_" + l.name)
					savelibs  = append(savelibs, l)
				}
				if l.newsize > 0 {
					if len(l.from) > 0 {
						fmt.Fprintln(os.Stderr, "new library declared as copy")
						os.Exit(1)
					}
					l.path = filepath.Join(job.wdir, s.name + "." + l.name)
					dep := "create_lib_" + s.name + "_" + l.name
					s.add_dep(dep)
					cleanafter = append(cleanafter, l.path)
					fmt.Println(dep + ":")
					fmt.Println("\trm -f " + l.path)
					switch(l.libtype) {
						case RAW:
							fmt.Print("\tfallocate -l " + 
								  fmt.Sprint(l.newsize) + "M " + 
								  l.path)
						case QCOW2:
							fmt.Print("\tqemu-img create -f qcow2 " + 
								  l.path + " " + 
								  fmt.Sprint(l.newsize) + "M")
						default:
							fmt.Fprintln(os.Stderr, "new library wrong format")
							os.Exit(1)
					}
					vbreak()
					continue
				}
				if len(l.from) > 0 {
					l.path = filepath.Join(job.wdir, s.name + "." + l.name)
					dep := "copy_lib_" + s.name + "_" + l.name
					s.add_dep(dep)
					cleanafter = append(cleanafter, l.path)
					fmt.Println(dep + ":")
					fmt.Println("\tcp " + l.from + " " + l.path)
					vbreak()
					continue
				}
				if l.name[0] != '@' {
					continue
				}
				if l.refstep != "" {
					s.add_dep("step_" + l.refstep)
				}
				rl := findlib(l.name)
				if rl == nil {
					fmt.Fprintln(os.Stderr, "cannot dereference " + l.name + ": library does not exist")
					os.Exit(1)
				}
				l.reflib = rl
			}
		}
	}

// Dump toplevel targets.

	fmt.Print("alltarg:")
	for _, t := range alltarg {
		fmt.Print(" " + t)
	}
	vbreak()

// Dump recipes to save each library that needs to be saved.

	for _, l := range savelibs {
		fmt.Println("save_lib_" + l.stname + "_" + l.name + ": " + "step_" + l.stname)
		pl := l
		if l.reflib != nil {
			pl = l.reflib
		}
		fmt.Print("\tcp " + pl.path + " " + l.save)
		vbreak()
	}

// Dump recipes for each step, listing dependencies first.

	for _, s := range job.steps {
		fmt.Print("step_" + s.name + ":")
		for _, d := range s.deps {
			fmt.Print(" " + d)
		}
		fmt.Println("")
		dumpstep(s, &job)
		vbreak()
	}

// Dump cleanafter targets.

	fmt.Println("cleanafter: alltarg")
	if len(cleanafter) > 0 {
		fmt.Print("\trm -f")
		for _, t := range cleanafter {
			fmt.Print(" \\\n\t " + t)
		}
	}
	vbreak()

}
