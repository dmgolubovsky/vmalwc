// The command utility for VM job control

package main

import (
	"os"
	"fmt"
	"strings"
	"strconv"
	"path/filepath"
	"github.com/cznic/mathutil"
	"launchpad.net/zabudka/go-src/src/smlib"
	"bitbucket.org/creachadair/goflags/bytesize"
)

type Job struct {
	kernel string		// which kernel to load
	kvm string		// path to KVM
	libmap []*Library	// toplevel libraries
	steps StepMap		// job steps
	mmegs int		// default memory for VM
	xdisplay int		// if positive, enable guestfwd to X server with given display
	wdir string		// working directory of the job
}

type LibMapN map [int] *Library

type Library struct {
	name string		// name of the library that can be used for step reference
	stname string		// name of the step where library was defined
	path string		// host path to the library (directory or volume)
	libtype int		// type of the library (raw, qcow, 9p, reference)
	write bool		// writable
	tag string		// tag for 9p, serial for volumes
	save string		// if nonempty, save the volume at this path (ignored for 9p)
	from string		// if nonempty, copy the volume from this path into a new volume
	snap bool		// if true, use this volume in snapshot mode
	refstep string		// for references, address a step whose library is reused, or top if blank
	reflib *Library		// reference to the library in other step
	newsize int		// if positive, the library has to be created anew even if existed - volumes only
}

type StepMap map [string] *Step

type Step struct {
	name string		// step name
	exec string		// will be passed to the kernel command line encoded
	append []string		// other appends
	libmap []*Library	// step-specific libraries
	ncons int		// how many consoles allocate (hvc0 always dumps console log)
	mmegs int		// memory for step
	sysin string		// path to redirect standard input (/dev/null)
	sysout string		// path to redirect standard output (/dev/console)
	deps []string		// step target dependencies
	after []string		// wait for these steps to complete
}

func (s *Step) add_dep(dep string) () {
	s.deps = append(s.deps, dep)
}

const (
	RAW = iota		// volume raw format
	QCOW2			// volume qcow2 format
	HTTP			// http-accessible volume
	NINEP			// 9p mounted directory
	REF			// reference to other step or top library
	NOTSET			// unknown
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

// Invoke KVM with all necessary options.
// All files that a VM instance creates will be added to the clean after list.
// All files will be created under the job's working directory prepending step name.
// The first virtconsole device (hvc0) will be used for dumping the console output to the file.
// Minimum 2 consoles (via sockets) will be allocated, more if ncons says so.

func dumpstep(s *Step, j *Job) {
	enkvm := " `kvm-ok >/dev/null && echo -cpu host -enable-kvm`"
	fmt.Println("\t" + j.kvm + enkvm + " -vga none -no-reboot \\")
	fmt.Println("\t -kernel " + j.kernel + " \\")
	msize := j.mmegs
	monpath := filepath.Join(j.wdir, s.name + ".monitor")
	consdump := filepath.Join(j.wdir, s.name + ".consdump")
	cleanafter = append(cleanafter, monpath)
	cleanafter = append(cleanafter, consdump)
	ncons := mathutil.Min(s.ncons, 2)
	kappend := "console=hvc0"
	encexec, e := smlib.EncJsonGzipB64(s.exec)
	if e != nil {
		fmt.Println(os.Stderr, e)
		os.Exit(1)
	}
	if len(s.exec) > 0 {
		kappend = kappend + " exec=" + encexec
	} else {
		kappend = kappend + " exec=none"
	}
	if len(s.sysin) > 0 {
		kappend = kappend + " sysin=" + s.sysin
	}
	if len(s.sysout) > 0 {
		kappend = kappend + " sysout=" + s.sysout
	}
	if s.mmegs > 0 {
		msize = s.mmegs
	}
	fmt.Println("\t -m " + fmt.Sprint(msize) + "M \\")
	fmt.Println("\t -display none -device virtio-balloon -device virtio-serial-pci \\")
	fmt.Println("\t -chardev socket,id=mon,path=" + monpath + ",server,nowait \\")
	fmt.Println("\t -mon chardev=mon \\")
	fmt.Println("\t -chardev file,id=consdump,path=" + consdump + " \\")
	fmt.Println("\t -device virtconsole,chardev=consdump \\")
	for i := 1 ; i <= ncons ; i++ {
		conspath := filepath.Join(j.wdir, s.name + ".vcons" + fmt.Sprint(i))
		chdid := "vcons" + fmt.Sprint(i)
		cleanafter = append(cleanafter, conspath)
		fmt.Println("\t -chardev socket,id=" + chdid + ",path=" + conspath + ",server,nowait \\")
		fmt.Println("\t -device virtconsole,chardev=" + chdid + " \\")
	}
	for i, l := range s.libmap {
		prlib(i, l)
	}
	red1 := ""
	red2 := ""
	if j.xdisplay >= 0 {
		red1 = ",guestfwd=tcp:10.0.2.100:6000-cmd:socat stdio unix-connect:/tmp/.X11-unix/X" + fmt.Sprint(j.xdisplay)
		kappend = kappend + " display=10.0.2.100:0"
	}
	paudio := os.Getenv("PULSE_SERVER")
	xaudio := os.Getenv("PULSE_EXTERNAL_SERVER")
	if len(paudio) > 0 {
		kappend = kappend + " pulse=tcp:10.0.2.200:4713"
		papts := strings.Split(paudio, "}")
		pasrv := ""
		if len(papts) == 2 {
			pasrv = papts[1]
		} else
		{
			pasrv = paudio
	
		}
		red2 = ",guestfwd=tcp:10.0.2.200:4713-cmd:socat stdio " + pasrv
	} else if len(xaudio) > 0 {
		kappend = kappend + " pulse=tcp:10.0.2.200:4713"
		red2 = ",guestfwd=tcp:10.0.2.200:4713-cmd:socat stdio " + xaudio
	}
	fmt.Println("\t -net 'user" + red1 + red2 + "' \\")
	fmt.Println("\t -net nic,model=virtio \\")
	fmt.Println("\t -append '" + kappend + "'")
	fmt.Println("\t exit `grep ^EXITCODE: " + consdump + " | tail -n 1 | cut -d: -f 2`")
}

func prlib(i int, l *Library) {
	switch(l.libtype) {
		case REF:
			if l.reflib != nil {
				prlib(i, l.reflib)
			}
		case RAW:
			rawlib(i, l)
		case HTTP:
			httplib(i, l)
		case QCOW2:
			qcwlib(i, l)
		case NINEP:
			nplib(i, l)
	}
}

func nplib(i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Println("\t -virtfs local" + ro + ",id=" + l.tag +
		    ",path=" + l.path + ",mount_tag=" + l.tag + ",security_model=mapped \\")
}

func rawlib(i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Println("\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off,x-data-plane=on \\")
	fmt.Print("\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache=none,aio=native,format=raw" + ro + ",file=" + l.path)
	commonlib(i, l)
}

func httplib(i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Println("\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off \\")
	fmt.Print("\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache=none,aio=native,format=http" + ro + ",url=" + l.path)
	commonlib(i, l)
}

func qcwlib(i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Println("\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off \\")
	fmt.Print("\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache=none,aio=native,format=qcow2" + ro + ",file=" + l.path)
	commonlib(i, l)
}

func commonlib(i int, l *Library) {
	if l.tag != "" {
		fmt.Print(",serial=" + l.tag)
	}
	if l.snap {
		fmt.Print(",snapshot=on")
	}
	fmt.Println(" \\")
}

func main () {
	pargs := append(os.Args[1:], "")
	job.libmap = []*Library{}
	job.steps = StepMap{}
	job.xdisplay = -1
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
