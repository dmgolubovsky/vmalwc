// The command utility for VM job control

package main

import (
	"io"
	"os"
	"fmt"
	"errors"
	"os/user"
	"os/exec"
	"strings"
	"strconv"
	"path/filepath"
	"launchpad.net/zabudka/go-src/src/smlib"
	"bitbucket.org/creachadair/goflags/bytesize"
)

func logger(s string) {
		exec.Command("logger", s).Run()
}

func vbreak (w io.WriteCloser) {
	fmt.Fprintln(w)
	fmt.Fprintln(w)
}

var pfxlib *LibPrefix = nil

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

// Translate a given path using the lib prefix map provided.

func transpath(path string, pfxl *LibPrefix) (string, bool, error) {
	if pfxl == nil {
		return path, false, nil
	}
	for k, p := range *pfxl {
		if len(k) > len(path) {
			continue
		}
		if strings.HasPrefix(path, k) {
			npath := p.Hostpath + path[len(k):]
			return npath, p.Write, nil
		}
	}
	return path, false, errors.New("no library prefix found for " + path)
}

func main () {
	pargs := append(os.Args[1:], "")
	job.libmap = []*Library{}
	job.steps = StepMap{}
	job.xdisplay = -1
	job.video = false
	job.audio = false
	job.hostname = "VM-" + fmt.Sprint(os.Getpid())
	prtmode := "text"
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
			case "-list":
				lst := listjobs()
				if len(lst) > 0 {
					prtjobs(lst, prtmode)
				}
				os.Exit(0)
			case "-find":
				findjobs()
				os.Exit(0)
			case "-purge":
				purgejobs()
				os.Exit(0)
			case "-log":
				joblog(prtmode)
				os.Exit(0)
			case "-stop":
				stopjob()
				os.Exit(0)
			case "-kill":
				killjob()
				os.Exit(0)
			case "-showlib":
				if pfxlib != nil {
					prtlibs(pfxlib, prtmode)
				}
				os.Exit(0)
			case "-user":
				if job.user == nil {
					fmt.Fprintln(os.Stderr, "Cannot obtain current user information")
					os.Exit(1)
				}
				job.uimp = true
			case "-app":
				i++
				job.desktop = pargs[i]
				skip = true
			case "-lpfx":
				i++
				e := smlib.DecJsonGzipB64(pargs[i], &pfxlib)
				if e != nil {
					fmt.Fprintln(os.Stderr, "Library prefix decode: " , e)
				}
				skip = true
			case "-audio":
				job.audio = true
			case "-video":
				job.video = true
			case "-make":
				job.make = true
			case "-quiet":
				job.quiet = true
			case "-kernel":
				i++
				job.kernel = pargs[i]
				skip = true
			case "-id":
				i++
				job.idraw = pargs[i]
				job.id = strings.Replace(pargs[i], ".", "", -1)
				skip = true
			case "-kvm":
				i++
				job.kvm = pargs[i]
				skip = true
			case "-hostname":
				i++
				job.hostname = filepath.Base(pargs[i])
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
			case "-host":
				i++
				if curstep != nil {
					curstep.host = true
				}
			case "-lbrst":
				i++
				if curstep != nil {
					curstep.lbrst = true
					if curstep.libpfx == nil {
						curstep.libpfx = &LibPrefix{}
					}
				}
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
						curlib.refstep = strings.Replace(stlb[0], ".", "", -1)
					}
				} else {
					curlib.libtype = NOTSET
				}
				skip = true
			case "-utag":
				i++
				if curlib != nil && job.uimp && strings.HasPrefix(curlib.path, job.user.HomeDir) {
					curlib.tag = "H#" + pargs[i]
				}
				skip = true
			case "-tag":
				i++
				if curlib != nil {
					curlib.tag = pargs[i]
				}
				skip = true
			case "-path":
				i++
				if curlib != nil {
					var wr bool
					curlib.path, wr, e = transpath(pargs[i], pfxlib)
					if e != nil {
						fmt.Fprintln(os.Stderr, "Translate path: ", pargs[i], e)
						os.Exit(1)
					}
					curlib.write = wr
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
						case "auto":
							curlib.libtype = NOTSET
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
				curstep.name = strings.Replace(pargs[i], ".", "", -1)
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
					 curstep.add_dep("step_" + strings.Replace(pargs[i], ".", "", -1))
				}
				skip = true
		}
	}
	
// If job ID was not specified make it PID of the current shell's parent (that is make)
// Otherwise append the PID of make after what's specified.

	if len(job.id) == 0 {
		job.id = "$$PPID"
	} else {
		job.id = job.id + "-$$PPID"
	}

// If app config was specified, process it (stub for now).

	appconfig()

// If -make was given, redirect stdout to make -f -

	var p io.WriteCloser
	var mkpr *exec.Cmd
	
	p = os.Stdout
	mkpr = nil

	if job.make {
		mkpr = exec.Command("make", "-f", "-")
		if mkpr == nil {
			fmt.Fprintln(os.Stderr, "cannot pipe to make")
			os.Exit(1)
		}
		if job.quiet {
			mkpr.Args = append(mkpr.Args, "--quiet")
		}
		p, e = mkpr.StdinPipe()
		if e != nil {
			fmt.Fprintln(os.Stderr, "cannot pipe to make: ", e)
			os.Exit(1)
		}
		mkpr.Stdout = os.Stdout
		logger("Job " + job.id + " started")
		mkpr.Start()
		if e != nil {
			fmt.Fprintln(os.Stderr, "cannot start make: ", e)
			os.Exit(1)
		}
	}

	fmt.Fprint(p, "all: alltarg cleanafter")
	vbreak(p)

// Append all steps to the toplevel target.
	
	for s := range job.steps {
		alltarg = append(alltarg, "step_" + job.steps[s].name)
	}
	vbreak(p)

// For each step if there are new-created libraries, step depends on their creation.
// Each new-created library is an cleanafteriate target to be deleted at the end.
// For each step if there are references to other step's libraries, step depends on
// the step referred to.
// For each library that is new, dump a file creation recipe based on its type.

	for _, s := range job.steps {
		rstep := job.id + "." + s.name
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
					l.path = filepath.Join(job.wdir, rstep + "." + l.name)
					dep := "create_lib_" + s.name + "_" + l.name
					s.add_dep(dep)
					cleanafter = append(cleanafter, l.path)
					fmt.Fprintln(p, dep + ":")
					fmt.Fprintln(p, "\trm -f " + l.path)
					switch(l.libtype) {
						case RAW:
							fmt.Fprint(p, "\tfallocate -l " + 
								  fmt.Sprint(l.newsize) + "M " + 
								  l.path)
						case QCOW2:
							fmt.Fprint(p, "\tqemu-img create -f qcow2 " + 
								  l.path + " " + 
								  fmt.Sprint(l.newsize) + "M")
						default:
							fmt.Fprintln(os.Stderr, "new library wrong format")
							os.Exit(1)
					}
					vbreak(p)
					continue
				}
				if len(l.from) > 0 {
					l.path = filepath.Join(job.wdir, rstep + "." + l.name)
					dep := "copy_lib_" + s.name + "_" + l.name
					s.add_dep(dep)
					cleanafter = append(cleanafter, l.path)
					fmt.Fprintln(p, dep + ":")
					fmt.Fprintln(p, "\tcp " + l.from + " " + l.path)
					vbreak(p)
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

	fmt.Fprint(p, "alltarg:")
	for _, t := range alltarg {
		fmt.Fprint(p, " " + t)
	}
	vbreak(p)

// Dump recipes to save each library that needs to be saved.

	for _, l := range savelibs {
		fmt.Fprintln(p, "save_lib_" + l.stname + "_" + l.name + ": " + "step_" + l.stname)
		pl := l
		if l.reflib != nil {
			pl = l.reflib
		}
		fmt.Fprint(p, "\tcp " + pl.path + " " + l.save)
		vbreak(p)
	}

// Dump recipes for each step, listing dependencies first.

	for _, s := range job.steps {
		fmt.Fprint(p, "step_" + s.name + ":")
		for _, d := range s.deps {
			fmt.Fprint(p, " " + d)
		}
		fmt.Fprintln(p, "")
		dumpstep(p, s, &job)
		vbreak(p)
	}

// Dump cleanafter targets.

	fmt.Fprintln(p, "cleanafter: alltarg")
	if len(cleanafter) > 0 {
		fmt.Fprint(p, "\trm -f")
		for _, t := range cleanafter {
			fmt.Fprint(p, " \\\n\t " + t)
		}
	}
	vbreak(p)
	
	p.Close()

	if mkpr != nil {
		e = mkpr.Wait()
		etxt := ""
		if e != nil {
			etxt = fmt.Sprint(": ", e)
		}
		logger("Job " + job.id + " finished" + etxt)
	}

}
