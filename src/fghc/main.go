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

// Translate a given path using the lib prefix map provided. If path starts with an exclamation,
// it is considered absolute path in the slave VM context, and is returned as is, rw is returned
// as true.

func transpath(path string, pfxl *LibPrefix) (string, bool, error) {
	if pfxl == nil {
		return path, false, nil
	}
	if path[0] == '!' {
		return path[1:], true, nil
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
	job.kernel = ""
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
			case "-list":	//+ H: list existing jobs
				lst := listjobs()
				if len(lst) > 0 {
					prtjobs(lst, prtmode)
				}
				os.Exit(0)
			case "-find":	//+ H: find jobs by job ID glob
				findjobs()
				os.Exit(0)
			case "-purge":	//+ H: remove files from not running jobs
				purgejobs()
				os.Exit(0)
			case "-log":	//+ H: print log of the job wih given job ID
				joblog(prtmode)
				os.Exit(0)
			case "-stop":	//+ H: gently stop the job wih given job ID
				stopjob()
				os.Exit(0)
			case "-kill":	//+ H: stop hard the job wih given job ID
				killjob()
				os.Exit(0)
			case "-showlib"://+ M: list all currently defined libraries
				if pfxlib != nil {
					prtlibs(pfxlib, prtmode)
				}
				os.Exit(0)
			case "-user":	//+ J: mount user's home directory into the VM
				if job.user == nil {
					fmt.Fprintln(os.Stderr, "Cannot obtain current user information")
					os.Exit(1)
				}
				job.uimp = true
			case "-app":	//+ R: reserved for future extensions
				i++
				job.desktop = pargs[i]
				skip = true
			case "-lpfx":	//+ X: pass encoded library access rules, not to be specified by user
				i++
				e := smlib.DecJsonGzipB64(pargs[i], &pfxlib)
				if e != nil {
					fmt.Fprintln(os.Stderr, "Library prefix decode: " , e)
				}
				skip = true
			case "-audio":	//+ J: allow host audio access in this job
				job.audio = true
				job.pulseaddr = "tcp:10.0.2.200:4713"
			case "-video":	//+ J: allow host video (X11) access in this job
				job.video = true
				job.xservaddr = "tcp:10.0.2.100:6000"
				job.xservdsp = "10.0.2.100:0"
			case "-make":	//+ H: invoke make automatically rather than just create a Makefile
				job.make = true
			case "-quiet":	//+ J: suppress output frpom make (invoke with -q)
				job.quiet = true
			case "-kernel":	//+ J: path to the kernel to be loaded
				i++
				job.kernel = pargs[i]
				skip = true
			case "-id":	//+ J: specify desired job ID
				i++
				job.idraw = pargs[i]
				job.id = strings.Replace(pargs[i], ".", "", -1)
				skip = true
			case "-idprt":	//+ J: when -make and -quiet specified, print job ID after it starts
				job.idprt = true
			case "-kvm":	//+ J: path to the KVM executable
				i++
				job.kvm = pargs[i]
				skip = true
			case "-hostname"://+ J: set job VM hostname (may be overridden by the job itself)
				i++
				job.hostname = filepath.Base(pargs[i])
				skip = true
			case "-workdir"://+ J: specify the working directory
				i++
				job.wdir = pargs[i]
				skip = true
			case "-mem":	//+ S: specify memory (in K, M, G) allocated to the step or job (if used outside of a step)
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
			case "-sysin":	//+ S: specify host file to be passed to the step standard input
				i++
				if curstep != nil {
					p, _, e := transpath(pargs[i], pfxlib)
					if e != nil {
						fmt.Fprintln(os.Stderr, "Translate path: ", pargs[i], e)
						os.Exit(1)
					}
					curstep.sysin = p
				}
				skip = true
			case "-sysout":	//+ S: specify host path where standard output of the step will be written
				i++
				if curstep != nil {
					var wr bool
					p, wr, e := transpath(pargs[i], pfxlib)
					if e != nil {
						fmt.Fprintln(os.Stderr, "Translate path: ", pargs[i], e)
						os.Exit(1)
					}
					if !wr {
						fmt.Fprintln(os.Stderr, "Canot write sysout to (read-only): ", pargs[i])
						os.Exit(1)
					}
					curstep.sysout = p
				}
				skip = true
			case "-host":	//+ S: allow host control in the step
				i++
				if curstep != nil {
					curstep.host = true
				}
			case "-xkernel"://+ S: kernel path to be used in the jobs submitted from within this VM
				i++
				if curstep != nil {
					curstep.xkernel = pargs[i]
				}
				skip = true
			case "-info":	//+ S: path to the directory where host files will be copied to be 9p-mounted into the VM
				i++
				if curstep != nil {
					curstep.infopath = pargs[i]
				}
				skip = true
			case "-copy":	//+ S: host file to be copied to the directory specified by -info, use multiple times if needed
				i++
				if curstep != nil {
					curstep.copyfiles = append(curstep.copyfiles, pargs[i])
				}
				skip = true
			case "-fwd":	//+ S: host port:VM port, enable TCP forward from host to VM
				i++
				if curstep != nil {
					ps := strings.Split(pargs[i], ":")
					if len(ps) != 2 {
						fmt.Fprintln(os.Stderr, "-fwd needs two numbers, colon-separated")
						os.Exit(1)
					}
					hp, e1 := strconv.ParseInt(ps[0], 10, 16)
					gp, e2 := strconv.ParseInt(ps[1], 10, 16)
					if e1 != nil || e2 != nil {
						fmt.Fprintln(os.Stderr, "-fwd: cannot parse port number")
						os.Exit(1)
					}
					curstep.hostfwd = append(curstep.hostfwd, HostFwd{int(hp), int(gp),})
				}
				skip = true
			case "-lbrst":	//+ H: pass the library restriction information to the jobs that this job will submit on the host
				i++
				if curstep != nil {
					curstep.lbrst = true
					if curstep.libpfx == nil {
						curstep.libpfx = &LibPrefix{}
					}
				}
			case "-xdisplay"://+ J: forward guest video requests to this X display
				i++
				x, e := strconv.ParseInt(pargs[i], 10, 32)
				if e != nil {
					fmt.Println(os.Stderr, e)
					os.Exit(1)
				}
				job.xdisplay = int(x)
				skip = true
			case "-lib":	//+ S: start library definition at the step or job level
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
			case "-utag":	//+ L: for 9p libraries, specify that should be mounted under user's home directory with given tag
				i++
				if curlib != nil && job.uimp && strings.HasPrefix(curlib.path, job.user.HomeDir) {
					curlib.tag = "H#" + pargs[i]
				}
				skip = true
			case "-tag":	//+ L: for 9p libraries, specify that should be mounted under the default location with given tag
				i++
				if curlib != nil {
					curlib.tag = pargs[i]
				}
				skip = true
			case "-path":	//+ L: path to library medium file or directory on the host
				i++
				if curlib != nil {
					var wr bool
					curlib.path, wr, e = transpath(pargs[i], pfxlib)
					if e != nil {
						fmt.Fprintln(os.Stderr, "Translate path: ", pargs[i], e)
						os.Exit(1)
					}
					if pfxlib != nil {
						curlib.write = wr
						curlib.locked = true
					}
				}
				skip = true
			case "-save":	//+ L: for intermediate libraries, save at this host path
				i++
				if curlib != nil {
					var wr bool
					curlib.save, wr, e = transpath(pargs[i], pfxlib)
					if e != nil {
						fmt.Fprintln(os.Stderr, "Translate path: ", pargs[i], e)
						os.Exit(1)
					}
					if !wr {
						fmt.Fprintln(os.Stderr, "Canot save to (read-only): ", pargs[i])
						os.Exit(1)
					}
				}
				skip = true
			case "-ro":	//+ L: allow only read-only access to the library
				if curlib != nil {
					curlib.write = false
				}
			case "-rw":	//+ L: allow read-write access to the library
				if curlib != nil {
					if curlib.locked && !curlib.write {
						fmt.Fprintln(os.Stderr, "Canot override write mode: ", curlib.name)
						os.Exit(1)
					}
					curlib.write = true
				}
			case "-new":	//+ L: create new intermediate library, file name will be chosen automatically
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
			case "-from":	//+ L: create an intermediate library as copy of an existing permanent library
				i++
				if curlib != nil {
					curlib.from, _, e = transpath(pargs[i], pfxlib)
					if e != nil {
						fmt.Fprintln(os.Stderr, "Translate path: ", pargs[i], e)
						os.Exit(1)
					}
				}
				skip = true
			case "-snap":	//+ L: use the library in snapshot mode: all changes will be lost
				if curlib != nil {
					curlib.snap = true
					curlib.locked = false
				}
			case "-type":	//+ L: specify library type if it cannot be determined automatically
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
			case "-step":	//+ S: opens a step scope
				i++
				curstep = &Step{}
				job.steps[pargs[i]] = curstep
				curstep.libmap = []*Library{}
				curstep.name = strings.Replace(pargs[i], ".", "", -1)
				curstep.ncons = 2
				curstep.hje = "tcp:10.0.2.150:77"
				curstep.infopath = "/dev/null"
				skip = true
			case "-exec":	//+ S: specify program and its arguments to be executed in the current step
				i++
				if curstep != nil {
					curstep.exec = pargs[i]
				}
				skip = true
			case "-after":	//+ S: set explicit dependency of the current step upon another step
				i++
				if curstep != nil {
					 curstep.add_dep("step_" + strings.Replace(pargs[i], ".", "", -1))
				}
				skip = true
		}
	}
	
// If job ID was not specified make it PID of this fghc instance.
// Otherwise append the PID of fghc after what's specified.

	mypid := fmt.Sprint(os.Getpid())
	if len(job.id) == 0 {
		job.id = mypid
	} else {
		job.id = job.id + "-" + mypid
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
			if job.idprt {
				fmt.Println(job.id)
			}
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
