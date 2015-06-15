
// The command utility for VM job control

package main

import (
	"io"
	"os"
	"fmt"
	"strings"
	"path/filepath"
	"github.com/bu-/magic"
	"github.com/cznic/mathutil"
	"launchpad.net/zabudka/go-src/src/smlib"
)

// Invoke KVM with all necessary options.
// All files that a VM instance creates will be added to the clean after list.
// All files will be created under the job's working directory prepending step name.
// The first virtconsole device (hvc0) will be used for dumping the console output to the file.
// Minimum 2 consoles (via sockets) will be allocated, more if ncons says so.

func dumpstep(p io.WriteCloser, s *Step, j *Job) {
	havekernel := (len(job.kernel) != 0)
	rstep := j.id + "." + s.name
	status := filepath.Join(j.wdir, rstep + ".status")
	fmt.Fprintln(p, "\t echo 'in progress' >" + status)
	enkvm := " `kvm-ok >/dev/null && echo -cpu host -enable-kvm`"
	fmt.Fprintln(p, "\t(" + j.kvm + enkvm + " -vga none -no-reboot -name " + s.name + " \\")
	if havekernel {
		fmt.Fprintln(p, "\t -kernel " + j.kernel + " \\")
	}
	msize := j.mmegs
	pidfile := filepath.Join(j.wdir, rstep + ".pid")
	monpath := filepath.Join(j.wdir, rstep + ".monitor")
	consdump := filepath.Join(j.wdir, rstep + ".consdump")
	cleanafter = append(cleanafter, monpath)
	cleanafter = append(cleanafter, consdump)
	cleanafter = append(cleanafter, pidfile)
	cleanafter = append(cleanafter, status)
	ncons := mathutil.Min(s.ncons, 2)
	info, e := os.Create(s.infopath)
	if e != nil {
		fmt.Fprintln(os.Stderr, "cannot open/create the info file: ", e)
		os.Exit(1)
	}
	defer info.Close()
	kappend := "console=hvc0 step=" + rstep + " jobid=" + j.id
	if len(j.hostname) > 0 {
		kappend = kappend + " hostname=" + j.hostname
	}
	encexec, e := smlib.EncJsonGzipB64(s.exec)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
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
	fmt.Fprintln(p, "\t -m " + fmt.Sprint(msize) + "M \\")
	fmt.Fprintln(p, "\t -pidfile " + pidfile + " \\")
	fmt.Fprintln(p, "\t -display none -device virtio-balloon -device virtio-serial-pci \\")
	fmt.Fprintln(p, "\t -chardev socket,id=mon,path=" + monpath + ",server,nowait \\")
	fmt.Fprintln(p, "\t -mon chardev=mon \\")
	fmt.Fprintln(p, "\t -chardev file,id=consdump,path=" + consdump + " \\")
	fmt.Fprintln(p, "\t -device virtconsole,chardev=consdump \\")
	for i := 1 ; i <= ncons ; i++ {
		conspath := filepath.Join(j.wdir, rstep + ".vcons" + fmt.Sprint(i))
		chdid := "vcons" + fmt.Sprint(i)
		cleanafter = append(cleanafter, conspath)
		fmt.Fprintln(p, "\t -chardev socket,id=" + chdid + ",path=" + conspath + ",server,nowait \\")
		fmt.Fprintln(p, "\t -device virtconsole,chardev=" + chdid + " \\")
	}
	for i, l := range s.libmap {
		prlib(p, i, l)
	}
	if j.uimp {
		var usrname = ""
		if len(j.user.Name) > 0 {
			usrname = j.user.Name
		} else {
			usrname = "host_user"
		}
		kappend = kappend + " user=" + usrname
		fmt.Fprintln(info, "user=" + usrname)
		kappend = kappend + " uid=" + j.user.Uid
		fmt.Fprintln(info, "uid=" + j.user.Uid)
		kappend = kappend + " gid=" + j.user.Gid
		fmt.Fprintln(info, "gid=" + j.user.Gid)
		kappend = kappend + " homebase=" + j.user.HomeDir
		fmt.Fprintln(info, "homebase=" + j.user.HomeDir)
	}
	red1 := ""
	red2 := ""
	red3 := ""
	if j.video && (j.xdisplay >= 0) {
		red1 = ",guestfwd=" + j.xservaddr + "-cmd:socat stdio unix-connect:/tmp/.X11-unix/X" + fmt.Sprint(j.xdisplay)
		kappend = kappend + " display=" + j.xservdsp
		fmt.Fprintln(info, "DISPLAY=" + j.xservdsp)
	}
	paudio := os.Getenv("PULSE_SERVER")
	xaudio := os.Getenv("PULSE_EXTERNAL_SERVER")
	if j.audio && (len(paudio) > 0) {
		kappend = kappend + " pulse=" + j.pulseaddr
		fmt.Fprintln(info, "PULSE_SERVER=" + j.pulseaddr)
		papts := strings.Split(paudio, "}")
		pasrv := ""
		if len(papts) == 2 {
			pasrv = papts[1]
		} else
		{
			pasrv = paudio
	
		}
		red2 = ",guestfwd=" + j.pulseaddr + "-cmd:socat stdio " + pasrv
	} else if j.audio && (len(xaudio) > 0) {
		kappend = kappend + " pulse=" + j.pulseaddr
		fmt.Fprintln(info, "pulse=" + j.pulseaddr)
		red2 = ",guestfwd=" + j.pulseaddr + "-cmd:socat stdio " + xaudio
	}
	if s.host {
		fenv := "env PULSE_SERVER=" + paudio + " PULSE_EXTERNAL_SERVER=" + xaudio + " "
		krnl := ""
		if havekernel {
			krnl = " -kernel " + j.kernel
		} else if len(s.xkernel) != 0 {
			krnl = " -kernel " + s.xkernel
		}
		fghc := os.Args[0] +  krnl + " -kvm " + j.kvm + 
			" -mem " + fmt.Sprint(j.mmegs) + "M" + " -workdir " + j.wdir
		if s.lbrst && s.libpfx != nil {
			for _, l := range(s.libmap) {
				lhpath := ""
				lcpath := ""
				switch(l.libtype) {
					default:
						continue
					case NINEP:
						lhpath = l.path
					case REF:
						if l.reflib == nil {
							continue
						}
						lhpath = l.reflib.path
				}
				if l.tag[0:1] == "H#" {
					lcpath = j.user.HomeDir + l.tag[2:]
				} else {
					lcpath = filepath.Join("/host", l.tag)
				}
				(*s.libpfx)[lcpath] = struct {
					Hostpath string
					Write bool
				} {
					lhpath,
					l.write,
				}
			}
			encl, e := smlib.EncJsonGzipB64(*s.libpfx)
			if e == nil {
				fghc = fghc + " -lpfx " + encl
			}
		}
		red3 = ",guestfwd=" + s.hje + "-cmd:xargs " + fenv + fghc
		fmt.Fprintln(info, "HOST_JOB_ENTRY=" + s.hje)
		kappend = kappend + " hostdisplay=" + fmt.Sprint(j.xdisplay)
		fmt.Fprintln(info, "hostdisplay=" + fmt.Sprint(j.xdisplay))
	}
	red4 := ""
	for _, p := range s.hostfwd {
		red4 = red4 + ",hostfwd=tcp:127.0.0.1:" + fmt.Sprint(p.hport) + "-:" + fmt.Sprint(p.gport)
	}
	red0 := ""
	if len(j.hostname) > 0 {
		red0 = ",hostname=" + j.hostname
	}
	fmt.Fprintln(p, "\t -net 'user" + red0 + red1 + red2 + red3 + red4 + "' \\")
	fmt.Fprintln(p, "\t -net nic,model=virtio \\")
	apnd := ""
	if havekernel {
		apnd = "\t -append \"" + kappend + "\""
	} else {
		apnd = "\t -boot c "
	}
	fmt.Fprintln(p, apnd + " ; exit `echo $$? | tee " + status + "` ) && \\")
	fmt.Fprintln(p, "\t\t exit `grep ^EXITCODE: " + consdump + " | tail -n 1 | cut -d: -f 2 | tee " + status + "`")
}

func prlib(p io.WriteCloser, i int, l *Library) {
	switch(l.libtype) {
		case NOTSET:
			mgc, e := magic.Open(0)
			if e != nil {
				return
			}
			defer mgc.Close()
			e = mgc.Load("/usr/share/misc/magic.mgc")
			if e != nil {
				return
			}
			mgs, e := mgc.File(l.path)
			if e != nil {
				return
			}
			vt := guessvol(mgs)
			if vt == NOTSET {
				return
			}
			l.libtype = vt
			prlib(p, i, l)
		case REF:
			if l.reflib != nil {
				prlib(p, i, l.reflib)
			}
		case RAW:
			rawlib(p, i, l)
		case HTTP:
			httplib(p, i, l)
		case QCOW2:
			qcwlib(p, i, l)
		case NINEP:
			nplib(p, i, l)
	}
}

func nplib(p io.WriteCloser, i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Fprintln(p, "\t -fsdev local" + ro + ",id=" + l.id + ",path=" + l.path + ",security_model=mapped \\")
	fmt.Fprintln(p, "\t -device virtio-9p-pci,fsdev=" + l.id + ",mount_tag=" + l.tag + " \\")
}

func rawlib(p io.WriteCloser, i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Fprintln(p, "\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off,x-data-plane=on \\")
	fmt.Fprint(p, "\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache.direct=on,aio=native,format=raw" + ro + ",file=" + l.path)
	commonlib(p, i, l)
}

func httplib(p io.WriteCloser, i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Fprintln(p, "\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off \\")
	fmt.Fprint(p, "\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache.direct=on,aio=native,format=http" + ro + ",url=" + l.path)
	commonlib(p, i, l)
}

func qcwlib(p io.WriteCloser, i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Fprintln(p, "\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off \\")
	fmt.Fprint(p, "\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache.direct=on,aio=native,format=qcow2" + ro + ",file=" + l.path)
	commonlib(p, i, l)
}

func commonlib(p io.WriteCloser, i int, l *Library) {
	if l.tag != "" {
		fmt.Fprint(p, ",serial=" + l.tag)
	}
	if l.snap {
		fmt.Fprint(p, ",snapshot=on")
	}
	fmt.Fprintln(p, " \\")
}
